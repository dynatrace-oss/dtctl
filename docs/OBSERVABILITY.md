# Observability

## Distributed Tracing (W3C Trace Context)

dtctl uses the [OpenTelemetry](https://opentelemetry.io/) SDK to create and export traces. dtctl exports spans directly via OTLP before the process exits. W3C Trace Context headers (`traceparent` / `tracestate`) are propagated on every HTTP request to Dynatrace APIs, so CLI invocations can participate in existing distributed traces.

### How it works

| Scenario                                  | Behaviour                                                                                                |
| ----------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `TRACEPARENT` is **not set**              | dtctl creates a new root span with a random trace-id                                                     |
| `TRACEPARENT` **is set**                  | dtctl inherits the trace-id and starts a child span — this invocation becomes part of the caller's trace |
| `TRACESTATE` **is set**                   | forwarded as-is alongside `traceparent`                                                                  |
| `TRACEPARENT` is **malformed**            | silently ignored per W3C spec; a new root span is created                                                |
| `OTEL_EXPORTER_OTLP_ENDPOINT` **is set**  | spans are exported to that OTLP endpoint via HTTP/protobuf                                               |
| `OTEL_EXPORTER_OTLP_ENDPOINT` **not set** | trace context is still generated and forwarded (no network export)                                       |

### Export spans to Dynatrace via OTLP

Set the standard OpenTelemetry environment variables. The Dynatrace OTLP ingest endpoint accepts spans directly:

```sh
export OTEL_EXPORTER_OTLP_ENDPOINT="https://{your-env-id}.live.dynatrace.com/api/v2/otlp"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Api-Token dt0c01.YOUR_TOKEN"
export OTEL_SERVICE_NAME="my-pipeline"   # optional, defaults to "dtctl"
dtctl get workflows
```

> **Token scopes required:** `openTelemetryTrace.ingest`

### Propagate from a CI/CD pipeline

Set `TRACEPARENT` from the pipeline's own trace context so dtctl calls appear as child spans in your build trace:

```sh
export TRACEPARENT="00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
dtctl get workflows   # carries the same trace-id, new child span-id
```

GitHub Actions example:

```yaml
- name: Query workflows
  env:
    TRACEPARENT: ${{ env.PIPELINE_TRACEPARENT }}
    OTEL_EXPORTER_OTLP_ENDPOINT: https://${{ vars.DT_ENV_ID }}.live.dynatrace.com/api/v2/otlp
    OTEL_EXPORTER_OTLP_HEADERS: Authorization=Api-Token ${{ secrets.DT_OTLP_TOKEN }}
  run: dtctl get workflows
```

### Environment variables

| Variable                      | Description                                                                                          |
| ----------------------------- | ---------------------------------------------------------------------------------------------------- |
| `TRACEPARENT`                 | W3C traceparent to inherit (`version-traceId-parentId-flags`)                                        |
| `TRACESTATE`                  | W3C tracestate to forward alongside traceparent                                                      |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP HTTP endpoint to export spans to (e.g. Dynatrace ingest URL)                                    |
| `OTEL_EXPORTER_OTLP_HEADERS`  | HTTP headers for the OTLP exporter, comma-separated `key=value` (e.g. `Authorization=Api-Token ...`) |
| `OTEL_SERVICE_NAME`           | Service name in the trace (default: `dtctl`)                                                         |

All other standard `OTEL_*` environment variables (e.g. `OTEL_RESOURCE_ATTRIBUTES`) are also passed through to the OTel SDK automatically.
