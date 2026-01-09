# Project Manifesto: dtctl Design Guidelines

## Core Philosophy

**"Kubectl for Dynatrace"**: Leverage existing DevOps muscle memory. If a user knows kubectl, they should intuitively know how to use dtctl.

**Developer Experience (DX) First**: Prioritize speed, interactivity, and "human-readable" inputs over 1:1 API purity.

**AI-Native**: The tool must function as an efficient backend for AI Agents, providing structured discovery and error recovery.

## 1. Command Structure (The Grammar)

Strictly follow the **verb + noun** pattern.

**Management (CRUD)**: `dtctl get | describe | edit | delete | apply [resource]`

- Resources: dashboard, notebook, alert, slo.

**Execution (Data)**: `dtctl query [dql-string]`

**Separation of Concerns**: Never mix configuration flags with data query logic.

## 2. The "No Leaky Abstractions" Rule (Data Querying)

Do not invent a new query language via CLI flags.

**Passthrough**: The query command is a "dumb pipe" for DQL.

- ✅ `dtctl query "fetch logs | filter ..."`
- ❌ `dtctl get logs --filter-status=ERROR` (Anti-pattern)

**File Support**: Always support reading queries from files (`-f query.dql`) to handle complex, multi-line logic.

**Templating**: Allow basic variable substitution in DQL files (`--set host=h-123`) to make them reusable.

## 3. Configuration Management (The "Monaco Bridge")

dtctl is the "Runtime" companion to Monaco's "Build time."

**Unit Testing**: Enable rapid testing of Monaco-style JSON templates without running a full pipeline.

**Apply Logic**:

- **Strict Mode**: Validate payloads against the API schema before sending.
- **Template Injection**: Support `dtctl apply -f template.json --set name="Dev"` to render variables client-side.
- **Idempotency**: `apply` must handle the logic of "Create (POST) if new, Update (PUT) if exists."

## 4. Input/Output Standards (Human vs. Machine)

**Input (Write)**: Support YAML with Comments.

- **Principle**: "Humans write YAML, Machines speak JSON."
- **Implementation**: Automatically convert user YAML to API-compliant JSON on the fly.

**Output (Read)**:

- **Default**: Human-readable TUI tables (ASCII).
- **JSON** (`-o json`): Raw API response for piping (jq).
- **YAML** (`-o yaml`): Reconstructed YAML for copy-pasting.

## 5. Handling Identity (Naming)

**Resolution Loop**: Never assume names are unique.

- If a user asks for `dtctl get db "Production"` and multiple exist, stop and ask (Interactive Disambiguation).
- Display NAME and ID side-by-side in lists.

**Safety**: Destructive actions (delete, apply) on ambiguous names must require confirmation or an exact UUID.

## 6. AI Enablement Strategy

Design features specifically to help LLMs drive the tool.

**Self-Discovery**: Ensure `--help` provides exhaustive, structured context.

**Machine Output**: Implement a `--plain` or `--machine` flag that strips ANSI colors, spinners, and interactive prompts, returning strict JSONL/YAML for agents.

**Error Messages**: Errors should be descriptive and suggest the fix (e.g., "Unknown flag --ownr, did you mean --owner?") to allow Agentic auto-correction.
