# dtctl MCP Usage Guide

Use `list_dtctl_commands` or `get_dtctl_command_help` to discover available commands.
Prefer dynamic tools named `dtctl_<command_path>` for direct execution.

## Natural Language To CLI Flags or DQL

- `format`, `in <format>` -> `-o <format>`
- `limit`, `maximum`, `max hits`, `top N`, `limite N`, `avec une limite de N`, `limite de N` -> `| limit N` in the DQL query
- `with filter`, `with jq`, `filter`, `filtre` -> `--jq <filter>`

## Import rules

### `dtctl logs` is NOT for querying log data

`dtctl logs` only streams execution logs for **workflow executions** (e.g. `dtctl logs workflow-execution <id>`).
It has no `--tail`, `--last`, or `--limit` flag.

To query Dynatrace log records, always use `dtctl query` with DQL:

```
dtctl query "fetch logs | limit 15"          # last 15 log records
dtctl query "fetch logs | sort timestamp desc | limit 1"  # latest log
```

Never call `dtctl logs --tail`, `dtctl logs --limit`, or `dtctl logs --last`.

## Examples

- `fetch logs` -> `dtctl query "fetch logs"`
- `last 10 logs` -> `dtctl query "fetch logs | limit 10"`
- `last 10 logs in json format` -> `dtctl query "fetch logs | limit 10" -o json`
- `last 25 logs in json format` -> `dtctl query "fetch logs | limit 25" -o json`
- `search all logs limited to 10` -> `dtctl query "fetch logs | limit 10"`
- `Last log in yaml format` -> `dtctl query "fetch logs | limit 1" -o json`
- `Donne moi le dernier log en json` -> `dtctl query "fetch logs | limit 1" -o json`
- `Donne moi les 15 derniers logs` -> `dtctl query "fetch logs | limit 15"`
