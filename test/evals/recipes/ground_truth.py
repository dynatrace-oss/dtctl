#!/usr/bin/env python3
"""Measure ground truth for every task, independently of the recipes.

    ground_truth.py <batch-dir> <phase> [task ...]     phase = start | end

Writes <batch-dir>/gt-<phase>.json. Run it right before a batch (start) and
right after it (end): volatile answers (open problems, last hour's logs) are
judged against both measurements.

The DQL here is written for the eval, not copied from a recipe: it is the
cross-check. Where a recipe and this file disagree, read both — that is a
finding. Queries use the main-branch binary against a read-only config.
"""
import json
import sys
import time
import traceback
from pathlib import Path

import lib

GIB = 1073741824


def s(v):
    """DQL string literal."""
    return json.dumps(v)


# ------------------------------------------------------------------ tenant 2

def t01(q, env):
    rows = q('fetch dt.davis.problems, from:now()-30d '
             '| dedup display_id, sort:{timestamp desc} '
             '| filter event.status=="ACTIVE" and not(dt.davis.is_duplicate) '
             '| summarize n=count(), by:{event.category} | sort n desc')
    by = {r["event.category"]: lib.num(r["n"]) for r in rows}
    # cross-check: distinct ids without the dedup
    x = q('fetch dt.davis.problems, from:now()-30d '
          '| filter event.status=="ACTIVE" and not(dt.davis.is_duplicate) '
          '| summarize n=countDistinct(display_id)')
    return dict(open_count=sum(by.values()), by_category=by,
                crosscheck_distinct_ids=lib.num(x[0]["n"]) if x else None)


def t02(q, env):
    ns = s(env["EVAL_T2_NAMESPACE"])
    base = 'fetch dt.davis.problems, from:now()-7d '
    rows = q(base + f'| filter not(dt.davis.is_duplicate) | filter in({ns}, k8s.namespace.name) '
             '| summarize n=count(), ids=countDistinct(display_id), by:{event.name} | sort n desc')
    eq = q(base + f'| filter not(dt.davis.is_duplicate) | filter k8s.namespace.name == {ns} '
           '| summarize n=count()')
    dup = q(base + f'| filter in({ns}, k8s.namespace.name) | summarize n=count()')
    count = sum(lib.num(r["n"]) for r in rows)
    return dict(count=count, distinct_ids=sum(lib.num(r["ids"]) for r in rows),
                top_title=rows[0]["event.name"] if rows else None,
                top_title_count=lib.num(rows[0]["n"]) if rows else 0,
                titles={r["event.name"]: lib.num(r["n"]) for r in rows[:6]},
                eq_count=lib.num(eq[0]["n"]) if eq else 0,
                count_with_duplicates=lib.num(dup[0]["n"]) if dup else 0)


def t03(q, env):
    cl = s(env["EVAL_T2_CLUSTER"])
    ev = q('fetch events, from:now()-1h '
           f'| filter event.provider=="KUBERNETES_EVENT" and k8s.cluster.name=={cl} '
           '| summarize n=count(), msg=takeLast(substring(dt.kubernetes.event.message, to:200)), '
           'by:{k8s.namespace.name, w=coalesce(k8s.workload.name, dt.kubernetes.event.involved_object.name), '
           'dt.kubernetes.event.reason} | sort n desc | limit 15')
    unhealthy = [dict(namespace=r.get("k8s.namespace.name"), workload=r.get("w"),
                      reason=r.get("dt.kubernetes.event.reason"), events_1h=lib.num(r["n"]),
                      message=r.get("msg")) for r in ev]
    notready = q('smartscapeNodes K8S_DEPLOYMENT, K8S_STATEFULSET, K8S_DAEMONSET '
                 f'| filter k8s.cluster.name=={cl} | parse k8s.object, "JSON:c" '
                 '| fieldsAdd desired=coalesce(c[spec][replicas], c[status][desiredNumberScheduled]), '
                 'ready=coalesce(c[status][readyReplicas], c[status][numberReady], 0) '
                 '| filter ready < desired | fields type, k8s.namespace.name, k8s.workload.name, desired, ready')
    cause = []
    if ev:
        # the worst workload's own logs: the evidence a good answer cites
        w = s(ev[0]["w"])
        cause = q('fetch logs, from:now()-1h '
                  f'| filter k8s.cluster.name=={cl} and (k8s.workload.name=={w} or contains(k8s.pod.name, {w})) '
                  '| filter in(status, {"ERROR", "WARN"}) or contains(content, "rror") or contains(content, "imeout") '
                  '| fieldsAdd p=replacePattern(substring(content, to:200), "DIGIT+", "*") '
                  '| summarize n=count(), by:{status, p} | sort n desc | limit 10')
    return dict(unhealthy=unhealthy, not_ready_controllers=notready,
                cause_log_patterns_of_worst=[dict(status=r.get("status"), n=lib.num(r["n"]), pattern=r.get("p")) for r in cause],
                note="Judge the cause against the log lines of the worst workload above.")


def t04(q, env):
    rows = q('timeseries {r=sum(dt.service.request.count, scalar:true), '
             'f=sum(dt.service.request.failure_count, scalar:true)}, by:{dt.service.name}, from:now()-2h '
             '| filter r>=1000 | fieldsAdd rate=100.0*f/r | sort rate desc | limit 6')
    top = [dict(service=r["dt.service.name"], requests=lib.num(r["r"]), failures=lib.num(r["f"]),
                rate_pct=round(lib.num(r["rate"]), 2)) for r in rows]
    reasons = {}
    for t in top[:2]:
        sv = s(t["service"])
        rr = q('fetch spans, from:now()-2h '
               f'| filter request.is_root_span and request.is_failed and dt.service.name=={sv} '
               '| fieldsAdd reason=arrayFirst(iCollectArray(dt.failure_detection.results[][reason])) '
               '| summarize n=count(), by:{endpoint.name, http.response.status_code, reason} '
               '| sort n desc | limit 5')
        reasons[t["service"]] = rr
    return dict(top=top, reason=reasons)


def t05(q, env):
    rows = q('timeseries {u=avg(dt.host.disk.used.percent), a=avg(dt.host.disk.avail)}, '
             'by:{host.name, dt.smartscape.disk}, from:now()-30m '
             '| fieldsAdd used_pct=round(arrayLast(u), decimals:1), free_gib=round(arrayLast(a)/1073741824, decimals:2), '
             'mount=getNodeName(dt.smartscape.disk) '
             '| filterOut contains(mount, "/kubelet/pods/") '
             '| sort used_pct desc | fields host.name, mount, used_pct, free_gib | limit 6')
    return dict(top=[dict(host=r["host.name"], mount=r["mount"], used_pct=lib.num(r["used_pct"]),
                          free_gib=lib.num(r["free_gib"])) for r in rows])


def t06(q, env):
    rows = q('fetch user.events, from:now()-2h '
             '| filter characteristics.has_page_summary and dt.rum.user_type=="real_user" and isNotNull(frontend.name) '
             '| summarize loads=count(), with_interaction=countIf(web_vitals.interaction_to_next_paint>0ms), '
             'inp=percentile(if(web_vitals.interaction_to_next_paint>0ms, web_vitals.interaction_to_next_paint), 75), '
             'inp_all=percentile(web_vitals.interaction_to_next_paint, 75), by:{frontend.name} '
             '| fieldsAdd p75_ms=toDouble(inp)/1000000, p75_ms_incl_zero=toDouble(inp_all)/1000000 '
             '| sort p75_ms desc | fields frontend.name, loads, with_interaction, p75_ms, p75_ms_incl_zero')
    return dict(ranking=[dict(frontend=r["frontend.name"], loads=lib.num(r["loads"]),
                              with_interaction=lib.num(r["with_interaction"]),
                              p75_ms=lib.num(r["p75_ms"]), p75_ms_incl_zero=lib.num(r["p75_ms_incl_zero"]))
                         for r in rows])


def t07(q, env):
    m = s(env["EVAL_T2_GENAI_MODEL_RE"].lower())
    base = ('fetch spans, from:now()-24h | filter isNotNull(gen_ai.usage.input_tokens) '
            f'| filter contains(gen_ai.request.model, {m}, caseSensitive: false) '
            f'or (isNull(gen_ai.request.model) and contains(gen_ai.response.model, {m}, caseSensitive: false)) ')
    agg = '| summarize calls=count(), inp=sum(gen_ai.usage.input_tokens), out=sum(gen_ai.usage.output_tokens)'
    naive = q(base + agg)
    spell = q(base + agg + ', by:{model=coalesce(gen_ai.request.model, gen_ai.response.model)}')
    dd = q(base + '| dedup {trace.id, gen_ai.usage.input_tokens, gen_ai.usage.output_tokens} ' + agg)
    return dict(input_tokens=lib.num(dd[0]["inp"]), calls=lib.num(dd[0]["calls"]),
                output_tokens=lib.num(dd[0]["out"]),
                naive_input_tokens=lib.num(naive[0]["inp"]), naive_calls=lib.num(naive[0]["calls"]),
                by_spelling=[{k: lib.num(v) for k, v in r.items()} for r in spell])


def _vulns(q, window):
    return q(f'fetch security.events, from:now()-{window} '
             '| filter event.provider=="Dynatrace" and event.level=="ENTITY" and in(event.type, '
             '{"VULNERABILITY_STATE_REPORT_EVENT","VULNERABILITY_STATUS_CHANGE_EVENT","VULNERABILITY_TRACKING_LINK_CHANGE_EVENT"}) '
             '| dedup {vulnerability.display_id, affected_entity.id}, sort:{timestamp desc} '
             '| filter vulnerability.resolution.status=="OPEN" and vulnerability.mute.status=="NOT_MUTED" '
             '| summarize risk=max(vulnerability.risk.score), by:{vulnerability.display_id} '
             '| summarize open=count(), high_or_critical=countIf(risk>=7), critical=countIf(risk>=9)')


def t08(q, env):
    out = {}
    for w in ("30m", "2h"):
        r = _vulns(q, w)
        out[w] = {k: lib.num(v) for k, v in r[0].items()} if r else {}
    best = out["2h"] or out["30m"]
    return dict(high_or_critical=best.get("high_or_critical"), critical=best.get("critical"),
                open_any_score=best.get("open"), by_window=out, capped=50,
                note="latest state per (vulnerability, entity); open + not muted on at least one entity")


def t09(q, env):
    rows = q('fetch security.events, from:now()-24h '
             '| filter event.type=="DETECTION_FINDING" or (event.type=="SECURITY_EVENT" and product.name=="Runtime Application Protection") '
             '| summarize n=count(), by:{product.name, event.type, finding.type, finding.action} | sort n desc | limit 12')
    return dict(by_type=[{k: lib.num(v) for k, v in r.items()} for r in rows])


def t10(q, env):
    rows = q('fetch bizevents, from:now()-24h | summarize n=count(), by:{event.type} | sort n desc | limit 5')
    two = q('fetch bizevents | summarize n=count(), by:{event.type} | sort n desc | limit 3')
    return dict(top=[dict(type=r["event.type"], count_24h=lib.num(r["n"])) for r in rows],
                default_2h_window=[dict(type=r["event.type"], count=lib.num(r["n"])) for r in two])


def t11(q, env):
    rows = q('fetch dt.synthetic.events, from:now()-24h '
             '| filter in(event.type, {"http_monitor_execution","browser_monitor_execution"}) '
             '| summarize total=count(), failed=countIf(result.state=="FAIL"), '
             'codes=collectDistinct(if(result.state=="FAIL", result.status.code)), '
             'msg=takeLast(if(result.state=="FAIL", result.status.message)), '
             'by:{dt.synthetic.monitor.name, event.type} | sort failed desc | limit 6')
    return dict(by_monitor=[{k: (lib.num(v) if k in ("total", "failed") else v) for k, v in r.items()}
                            for r in rows])


def t12(q, env):
    r = q('fetch logs, from:now()-24h, to:now()-12h | filter status=="ERROR" | summarize n=count()')
    x = q('fetch logs, from:now()-24h, to:now()-12h | summarize n=countIf(loglevel=="ERROR")')
    return dict(count=lib.num(r[0]["n"]), crosscheck_loglevel_error=lib.num(x[0]["n"]),
                measured_at=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()))


def t13(q, env):
    rows = q('smartscapeNodes "AWS_LAMBDA_FUNCTION" | parse aws.object, "JSON:j" '
             '| fieldsAdd rt=coalesce(toString(j[configuration][configuration][runtime]), '
             'if(toString(j[configuration][configuration][packageType])=="Image", "container image"), "(none)") '
             '| summarize n=count(), by:{rt} | sort n desc')
    by = {r["rt"]: lib.num(r["n"]) for r in rows}
    return dict(lambda_count=sum(by.values()), by_runtime=by)


# ------------------------------------------------------------------ tenant 1

def t14(q, env):
    det = q('fetch security.events, from:now()-7d '
            '| filter in(event.type, {"DETECTION_FINDING","SECURITY_EVENT"}) | summarize n=count()')
    types = q('fetch security.events, from:now()-7d | summarize n=count(), by:{event.type} | sort n desc | limit 10')
    return dict(detections_7d=lib.num(det[0]["n"]) if det else 0,
                present_types={r["event.type"]: lib.num(r["n"]) for r in types})


def t15(q, env):
    out = {}
    for p in ("aws", "azure", "gcp"):
        r = q(f'smartscapeNodes "{p.upper()}_*" | summarize n=count()')
        out[p] = lib.num(r[0]["n"]) if r else 0
    top = q('smartscapeNodes "AWS_*" | summarize n=count(), by:{type} | sort n desc | limit 8')
    out["aws_by_type"] = {r["type"]: lib.num(r["n"]) for r in top}
    return out


def t16(q, env):
    rows = q('fetch dt.system.events, from: now()@d - 7d, to: now()@d + 2h '
             '| filter event.kind=="BILLING_USAGE_EVENT" and contains(event.type, "Log") '
             '| fieldsAdd es=coalesce(usage.start, timestamp) | filter es >= now()@d - 7d and es < now()@d '
             '| dedup {event.id, event.type} '
             '| summarize gib=sum(toDouble(billed_bytes))/1073741824, n=count(), by:{event.type}')
    by = {r["event.type"]: round(lib.num(r["gib"]), 2) for r in rows}
    ingest = next((v for k, v in by.items() if "Ingest" in k), None)
    return dict(ingest_gib=ingest, by_type_gib=by)


def t17(q, env):
    rows = q('timeseries {a=sum(dt.kubernetes.node.cpu_allocatable), r=sum(dt.kubernetes.container.requests_cpu)}, '
             'by:{k8s.node.name}, from:now()-30m '
             '| fieldsAdd pct=round(100*arrayAvg(r)/arrayAvg(a), decimals:1) | sort pct desc '
             '| fields k8s.node.name, pct | limit 6')
    return dict(ranking=[dict(node=r["k8s.node.name"], requests_pct=lib.num(r["pct"])) for r in rows])


def t18(q, env):
    base = 'fetch spans, from:now()-24h | filter isNotNull(gen_ai.usage.input_tokens) '
    agg = ('| summarize calls=count(), inp=sum(gen_ai.usage.input_tokens), '
           'cache=sum(gen_ai.usage.cache_read.input_tokens), out=sum(gen_ai.usage.output_tokens)')
    raw = q(base + agg)[0]
    dd = q(base + '| dedup {trace.id, gen_ai.usage.input_tokens, gen_ai.usage.output_tokens} ' + agg)[0]
    inp, cache = lib.num(dd["inp"]), lib.num(dd["cache"]) or 0
    return dict(calls=lib.num(dd["calls"]), input_tokens=inp, cache_read_tokens=cache,
                cache_share_pct=round(100.0 * cache / inp, 1) if inp else None,
                output_tokens=lib.num(dd["out"]),
                raw_calls=lib.num(raw["calls"]), raw_input_tokens=lib.num(raw["inp"]))


def t19(q, env):
    sv, ep = s(env["EVAL_T1_AGENT_SERVICE"]), s(env["EVAL_T1_AGENT_ENDPOINT"])
    root = (f'fetch spans, from:now()-2h | filter request.is_root_span and endpoint.name=={ep} '
            f'and contains(dt.service.name, {sv}) ')
    lat = q(root + '| summarize n=count(), p50=percentile(duration,50), p90=percentile(duration,90), '
            'by:{dt.service.name} | fieldsAdd p50_s=toDouble(p50)/1e9, p90_s=toDouble(p90)/1e9 '
            '| fields dt.service.name, n, p50_s, p90_s | sort n desc')
    lat = [dict(service=r["dt.service.name"], requests=lib.num(r["n"]),
                p50_s=round(lib.num(r["p50_s"]), 1), p90_s=round(lib.num(r["p90_s"]), 1)) for r in lat]
    prod = next((x for x in lat if "prod" in x["service"].lower()), lat[0] if lat else None)
    breakdown = []
    if prod:
        breakdown = q(f'fetch spans, from:now()-2h | filter request.is_root_span and endpoint.name=={ep} '
                      f'and dt.service.name=={s(prod["service"])} | fields trace.id | limit 200 '
                      '| join [fetch spans, from:now()-2h | fields trace.id, sn=span.name, d=duration, '
                      'op=gen_ai.operation.name, k=span.kind], on:{trace.id}, fields:{sn, d, op, k} '
                      '| summarize spans=count(), total_s=sum(d)/1000000000.0, avg_s=avg(d)/1000000000.0, '
                      'by:{op, k, sn} | sort total_s desc | limit 20')
        breakdown = [{k: (round(lib.num(v), 2) if k in ("total_s", "avg_s") else lib.num(v))
                      for k, v in r.items()} for r in breakdown]
    root_s = sum(r["total_s"] for r in breakdown if r.get("k") == "server")
    by_op = {}
    for r in breakdown:
        if r.get("op"):
            by_op[r["op"]] = round(by_op.get(r["op"], 0) + r["total_s"], 1)
    share = {op: round(100 * v / root_s, 1) for op, v in by_op.items()} if root_s else {}
    return dict(latency=lat, prod=prod, root_total_s=root_s, genai_op_total_s=by_op,
                genai_op_share_of_root_pct=share, breakdown=breakdown,
                note="root spans include their children: compare chat spans' summed time to the "
                     "root total, not to the sum of all rows")


def t20(q, env):
    rows = q('fetch dt.davis.problems, from:now()-7d | filter not(dt.davis.is_duplicate) '
             '| fieldsAdd d=if(event.status=="CLOSED", toDouble(event.end-event.start)/60000000000) '
             '| summarize n=count(), closed=countIf(event.status=="CLOSED"), median_min=median(d), mean_min=avg(d), '
             'by:{event.category} | sort n desc')
    cat = [dict(category=r["event.category"], count=lib.num(r["n"]), closed=lib.num(r["closed"]),
                median_min=round(lib.num(r["median_min"]) or 0, 1),
                mean_min=round(lib.num(r["mean_min"]) or 0, 1)) for r in rows]
    started = q('fetch dt.davis.problems, from:now()-7d | filter not(dt.davis.is_duplicate) and event.start >= now()-7d '
                '| summarize n=count(), by:{event.category} | sort n desc')
    top = cat[0] if cat else {}
    return dict(top_category=top.get("category"), count=top.get("count"), median_min=top.get("median_min"),
                by_category=cat,
                started_in_7d={r["event.category"]: lib.num(r["n"]) for r in started})


def t21(q, env):
    n = q('smartscapeNodes K8S_NODE | summarize n=count()')
    h = q('smartscapeNodes HOST | summarize hosts=count(), k8s=countIf(isNotNull(k8s.node.name))')
    return dict(k8s_nodes=lib.num(n[0]["n"]), hosts=lib.num(h[0]["hosts"]),
                nodes_as_hosts=lib.num(h[0]["k8s"]))


def t22(q, env):
    base = ('fetch dt.system.events, from:now()-24h | filter event.kind=="WORKFLOW_EVENT" '
            'and event.type=="WORKFLOW_EXECUTION" and dt.automation_engine.state.is_final==true ')
    states = q(base + '| summarize n=count(), by:{dt.automation_engine.state}')
    failed = q(base + '| filter dt.automation_engine.state!="SUCCESS" '
               '| summarize n=count(), first=min(timestamp), last=max(timestamp), '
               'by:{dt.automation_engine.workflow.title, dt.automation_engine.state} | sort n desc')
    return dict(executions_24h=sum(lib.num(r["n"]) for r in states),
                by_state={r["dt.automation_engine.state"]: lib.num(r["n"]) for r in states},
                failed=[dict(title=r["dt.automation_engine.workflow.title"], state=r["dt.automation_engine.state"],
                             count=lib.num(r["n"]), first=r["first"], last=r["last"]) for r in failed])


def t23(q, env):
    rows = q('fetch logs, from:now()-1h | filter status=="ERROR" '
             '| fieldsAdd p=replacePattern(substring(content, to:120), "DIGIT+", "*") '
             '| summarize n=count(), src=collectDistinct(coalesce(k8s.workload.name, dt.process_group.detected_name, '
             'service.name, log.source), maxLength:4), by:{p} | sort n desc | limit 6')
    return dict(patterns=[dict(pattern=r["p"], count=lib.num(r["n"]), sources=r["src"]) for r in rows])


def t24(q, env):
    sv = s(env["EVAL_T1_LOG_SERVICE"])
    r = q(f'fetch logs, from:now()-1h | filter service.name=={sv} '
          '| summarize n=count(), by:{k8s.namespace.name} | sort n desc')
    w = q(f'fetch logs, from:now()-1h | filter service.name=={sv} '
          '| summarize n=count(), by:{has_workload=isNotNull(k8s.workload.name)}')
    hw = {str(x["has_workload"]).lower(): lib.num(x["n"]) for x in w}
    by = {str(x.get("k8s.namespace.name")): lib.num(x["n"]) for x in r}
    return dict(count=sum(by.values()), by_namespace=by,
                workload_only=hw.get("true", 0), without_workload=hw.get("false", 0),
                note="workload_only = the records a k8s.workload.name filter can reach")


TASKS = {f.__name__: f for f in (t01, t02, t03, t04, t05, t06, t07, t08, t09, t10, t11, t12, t13,
                                 t14, t15, t16, t17, t18, t19, t20, t21, t22, t23, t24)}


def main():
    if len(sys.argv) < 3 or sys.argv[2] not in ("start", "end"):
        sys.exit(__doc__)
    batch, phase, only = Path(sys.argv[1]), sys.argv[2], set(sys.argv[3:])
    env = lib.load_env()
    batch.mkdir(parents=True, exist_ok=True)
    tasks = {t["id"]: t for t in lib.load_tasks()}
    cfgs = {}
    out = {"_measured_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}
    for tid, fn in TASKS.items():
        if only and tid not in only:
            continue
        tenant = tasks[tid]["meta"]["tenant"]
        ctx = lib.tenant_context(env, tenant)
        if ctx not in cfgs:
            cfgs[ctx] = lib.make_readonly_config(env, ctx, batch / "gt-cfg" / f"{tenant}.yaml")
        iso = batch / "gt-iso" / tenant
        t0 = time.time()
        try:
            out[tid] = fn(lambda dql: lib.query(cfgs[ctx], iso, dql), env)
        except Exception as e:  # keep going: one broken query must not lose the rest
            out[tid] = {"_error": f"{type(e).__name__}: {e}"}
            traceback.print_exc()
        print(f"{tid} {time.time() - t0:5.1f}s", file=sys.stderr)
    dest = batch / f"gt-{phase}.json"
    if only and dest.exists():
        prev = json.loads(dest.read_text())
        prev.update(out)
        out = prev
    dest.write_text(json.dumps(out, indent=2, default=str))
    print(dest)


if __name__ == "__main__":
    main()
