---
layout: docs
title: CoPilot
---

CoPilot is Dynatrace's conversational AI assistant. dtctl lets you interact with
it from the terminal — chat, translate natural language to DQL (and back), and
search across your notebooks and dashboards.

## List CoPilot Skills

```bash
# List all available CoPilot skills
dtctl get copilot-skills

# Structured output
dtctl get copilot-skills -o json
```

## Chat

```bash
# Ask a question
dtctl exec copilot "What caused the CPU spike on host-123?"

# Stream the response in real time
dtctl exec copilot "Explain the recent errors" --stream

# Read the message from a file
dtctl exec copilot -f question.txt

# Provide additional context for the conversation
dtctl exec copilot "Why is my service slow?" \
  --context "Service: payment-api, Environment: production"

# Add formatting instructions to guide the response
dtctl exec copilot "List top errors" \
  --instruction "Answer in bullet points"

# Disable Dynatrace documentation retrieval for faster responses
dtctl exec copilot "What is DQL?" --no-docs
```

| Flag | Description |
|------|-------------|
| `-f`, `--file` | Read the message from a file |
| `--stream` | Stream the response in real time |
| `--context` | Additional context for the conversation |
| `--instruction` | Formatting instructions (e.g. "Answer in bullet points") |
| `--no-docs` | Disable Dynatrace documentation retrieval |

> `copilot` also accepts the `cp` and `chat` aliases.

## Natural Language to DQL

```bash
# Convert a natural language question to a DQL query
dtctl exec copilot nl2dql "show me error logs from the last hour"

# Read the prompt from a file
dtctl exec copilot nl2dql -f prompt.txt

# Structured output
dtctl exec copilot nl2dql "find hosts with high CPU" -o json
```

## DQL to Natural Language

```bash
# Explain a DQL query in plain English
dtctl exec copilot dql2nl "fetch logs | filter status='ERROR' | limit 10"

# Read the query from a file
dtctl exec copilot dql2nl -f query.dql

# Structured output
dtctl exec copilot dql2nl "fetch logs | limit 10" -o json
```

## Document Search

Search across your notebooks and dashboards using natural language.

```bash
# Search within specific collections
dtctl exec copilot document-search "CPU performance" \
  --collections notebooks

# Search multiple collections
dtctl exec copilot document-search "error monitoring" \
  --collections dashboards,notebooks

# Exclude specific document IDs from results
dtctl exec copilot document-search "performance" \
  --exclude doc-123,doc-456

# Structured output
dtctl exec copilot document-search "kubernetes" \
  --collections notebooks -o json
```

| Flag | Description |
|------|-------------|
| `--collections` | Document collections to search (e.g. `notebooks,dashboards`) |
| `--exclude` | Document IDs to exclude from results |

> `document-search` also accepts the `doc-search` and `ds` aliases.

## Required Scopes

| Scope | Used By |
|-------|---------|
| `davis-copilot:conversations:execute` | Chat and listing CoPilot skills |
| `davis-copilot:nl2dql:execute` | Natural language to DQL |
| `davis-copilot:dql2nl:execute` | DQL to natural language |
| `davis-copilot:document-search:execute` | Document search |
