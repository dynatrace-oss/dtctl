# Worked recipe examples

These files illustrate [RECIPES_DESIGN.md](../../RECIPES_DESIGN.md). They are
**not** shipped recipes:

- The DQL is adapted from the `dynatrace-for-ai` skills (Apache-2.0). Each file
  names its source.
- None of it has been executed against an environment. The shipped, verified
  built-in recipes live in [`recipes/`](../../../../recipes/), and their
  authoring guide is [`recipes/README.md`](../../../../recipes/README.md).
  Where an example and a built-in differ, the built-in is right.

| File | Exercises |
|---|---|
| `_domains.yaml` | the domain registry that recipe names are prefixed with |
| `_scopes.yaml` | scope dimensions over primary Grail fields and tags |
| `_fragments/security.tmpl` | a shared template fragment (`{{template}}`) |
| `k8s-pod-restarts.yaml` | scope dimensions in a `timeseries` `filter:` argument; an int param |
| `services-red.yaml` | a `list` param |
| `traces-slow-endpoints.yaml` | an optional filter; `next` bound from a result row (phase 2) |
| `logs-error-patterns.yaml` | optional filters plus scope dimensions as a pipeline stage |
| `problems-active.yaml` | an `enum` param |
| `problems-get.yaml` | resolving a `P-` display ID: positional param, `pattern`, a lookback window |
| `hosts-disk-saturation.yaml` | a threshold param; host-group scope on metrics |
| `cloud-inventory.yaml` | `timeframe: none`; an enum that selects a field set |
| `frontends-web-vitals.yaml` | unit traps encoded once |
| `genai-token-usage.yaml` | `next` with `when: empty` (the empty-state protocol) |
| `security-vulns-critical-exploitable.yaml` | a `fixed` snapshot window; a fragment; segments off |
| `costs-dps-by-capability.yaml` | an inline window (`.window`) aligned to UTC days; segments off |
| `capacity-cpu-saturation.yaml` | a minimum window |
| `network-top-talkers.yaml` | a bucket-scoped fetch |
| `bundle-genai.yaml` | an app-shipped bundle (§13): several recipes, capability definitions, the empty-state protocol |
