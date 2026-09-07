# Server Mode (Experimental)

<!-- Migrated from the standalone docs site; SME to verify. -->

> **Experimental and unsupported.** `dtctl serve` is not part of the officially
> supported command set, is not registered unless you opt in, and its
> request/response contract may change without notice. Do not depend on it in
> production.

`dtctl serve` runs dtctl as a server instead of a one-shot CLI: one HTTP request
carries one dtctl command line plus the tenant to run it against, and the
response carries what the CLI would have printed (stdout, stderr, exit code).
It exists mainly as an experiment for driving dtctl from AI agents and automation
that cannot ship the binary itself.

## Opt in

Without the environment variable below, `dtctl serve` is an unknown command: it
does not appear in `--help` or in the `dtctl commands` catalog.

```bash
export DTCTL_EXPERIMENTAL_SERVE=1

dtctl serve                                      # List the protocols this build can speak
dtctl serve http                                 # JSON over HTTP on 127.0.0.1:7211
dtctl serve http --addr 0.0.0.0:8080             # Custom listen address
dtctl serve http --max-request-bytes 33554432    # Body limit (default 10485760 = 10 MiB)
```

## Endpoints

| Endpoint | Purpose |
|----------|---------|
| `POST /v1/execute` | Run one dtctl command line for one tenant: `{"command": ..., "environmentUrl": ..., "token": ..., "files": ...}` -> `{"exitCode": ..., "stdout": ..., "stderr": ..., "files": ..., "durationMs": ...}` |
| `GET /healthz` | Liveness probe |

Each request brings its own environment URL and token; the local config, keyring,
and credential environment variables are never read. A failed command still
answers HTTP 200 - the failure is in `exitCode`/`stderr`.

## No authentication of its own

The server has **no authentication of its own** and binds to localhost by
default. If you expose it beyond localhost, put your own authentication, TLS,
and rate limiting in front of it. Treat this feature as a proof of concept, not
a hardened service.

If you were looking for the fuller design write-up that used to live at this
address on the standalone docs site (request/response contract details,
concurrency model, embedding `pkg/engine` directly), ask in the project's
issue tracker or check `docs/dev/SERVICE_ENGINE_DESIGN.md` in the repository -
this page is intentionally kept minimal because the feature itself is
experimental.
