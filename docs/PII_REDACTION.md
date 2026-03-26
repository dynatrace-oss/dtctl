# PII Redaction: Query Freely, Leak Nothing

dtctl includes built-in PII (Personally Identifiable Information) detection and redaction for DQL query results. This ensures AI agents and automated pipelines can work with real query data without exposing sensitive information like email addresses, names, IPs, or credentials.

## Table of Contents

1. [Why PII Masking Matters for LLMs](#why-pii-masking-matters-for-llms)
2. [Trust Boundary Architecture](#trust-boundary-architecture)
3. [Two-Tier Design](#two-tier-design)
4. [Quick Start](#quick-start)
5. [Configuration Reference](#configuration-reference)
6. [Custom Field Rules](#custom-field-rules)
7. [Detection Strategy](#detection-strategy)
8. [RUM Data Coverage](#rum-data-coverage)
9. [Testing Guide](#testing-guide)

---

## Why PII Masking Matters for LLMs

When AI agents query observability data, every record they see becomes part of their context window. This creates several risks:

**Training data leakage** — Model providers may use API interactions for training. Customer names, email addresses, and IPs in query results could end up in future model weights, making them potentially recoverable.

**Prompt injection via data** — Malicious actors can embed instructions in log messages or user-agent strings. If an agent processes unredacted data containing `"user": "Ignore previous instructions and..."`, the attack surface is the entire dataset.

**GDPR / CCPA compliance** — Regulations require data minimization. An AI agent diagnosing a performance issue does not need to know that `user_42` is `mario.rossi@example.invalid`. The category label `[PERSON]` is sufficient for pattern analysis.

**Context windows as attack surface** — Every token of PII in a context window is a token that could be extracted via prompt injection, logged by middleware, or cached in an unencrypted session store. Redaction reduces the blast radius to zero.

**Correlation without identification** — In full mode, pseudonyms like `<EMAIL_0>` and `<EMAIL_1>` preserve the ability to distinguish between different users without revealing who they are. The agent can still say "the same user triggered 47 errors" without knowing the user's name.

---

## Trust Boundary Architecture

The PII system enforces a strict trust boundary:

```
┌──────────────────────────┐     ┌──────────────────────────┐
│       dtctl (agent)      │     │   dtctl-pii-resolve      │
│                          │     │   (human operator)       │
│  Query → Redact → Output │     │                          │
│                          │     │  Session file → Originals│
│  Can NEVER reverse       │     │  CAN reverse pseudonyms  │
│  pseudonyms              │     │                          │
└──────────┬───────────────┘     └──────────┬───────────────┘
           │                                │
           │  writes session                │  reads session
           ▼                                ▼
     ~/.local/share/dtctl/pii/sessions/
     └── pii_20260326_102611_3b3c73ad.json
```

**dtctl** (used by agents) can only write pseudonym sessions. It never reads them back. The reverse lookup (`pseudonym → original`) is in a **separate binary** (`dtctl-pii-resolve`) that only human operators should have access to.

This means even if an agent is compromised, it cannot access the pseudonym database to reverse the redaction.

---

## Two-Tier Design

### Lite Mode (`--pii` or `--pii=lite`)

- Replaces PII with category placeholders: `[EMAIL]`, `[PERSON]`, `[IP_ADDRESS]`
- No state, no files written, no external dependencies
- All occurrences of the same value get the same placeholder (no correlation)
- Best for: quick queries, CI pipelines, environments where you don't need correlation

```bash
dtctl query "fetch logs | limit 5" --pii
```

```
email: [EMAIL]   firstName: [PERSON]   ip: [IP_ADDRESS]
```

### Full Mode (`--pii=full`)

- Replaces PII with stable, session-scoped pseudonyms: `<EMAIL_0>`, `<PERSON_1>`
- Same value → same pseudonym within a session (enables correlation)
- Session persisted to `~/.local/share/dtctl/pii/sessions/`
- Optionally integrates with [Microsoft Presidio](https://microsoft.github.io/presidio/) for NER-based detection of PII in free-text fields
- Best for: investigation sessions, debugging where you need to track "the same user" across records

```bash
dtctl query "fetch logs | limit 5" --pii=full
```

```
email: <EMAIL_0>   firstName: <PERSON_0>   ip: <IP_ADDRESS_0>
email: <EMAIL_1>   firstName: <PERSON_0>   ip: <IP_ADDRESS_1>
# ^ Same person, different email/IP — pseudonyms let you see the pattern
```

After the query, resolve pseudonyms (human operator only):

```bash
dtctl-pii-resolve resolve pii_20260326_102611_3b3c73ad --all
```

---

## Quick Start

```bash
# Lite mode — simple placeholders, zero setup
dtctl query "fetch logs | limit 10" --pii

# Full mode — stable pseudonyms with session persistence
dtctl query "fetch logs | limit 10" --pii=full

# Disable PII even if enabled in config/env
dtctl query "fetch logs | limit 10" --no-pii

# List saved sessions
dtctl-pii-resolve sessions

# Resolve all pseudonyms in a session
dtctl-pii-resolve resolve <session-id> --all

# Purge sessions older than 7 days
dtctl-pii-resolve purge --max-age 168h
```

---

## Configuration Reference

PII mode is resolved with layered precedence: **flag > environment variable > config file**.

### Flag

```bash
dtctl query "..." --pii          # lite mode (default when bare --pii)
dtctl query "..." --pii=lite     # explicit lite
dtctl query "..." --pii=full     # full mode with pseudonyms
dtctl query "..." --no-pii       # disable (overrides everything)
```

### Environment Variable

```bash
export DTCTL_PII=lite    # or "full"
dtctl query "..."        # PII redaction active without --pii flag
```

### Config File (`~/.config/dtctl/config`)

```yaml
preferences:
  pii:
    mode: lite            # or "full"
    presidio-url: http://localhost:5002   # optional, full mode only
    custom-fields:        # optional per-field rules
      - field: "data.customerRef"
        category: PERSON
      - field: "data.action"
        skip: true
```

### Presidio Integration (Full Mode Only)

If you have a [Microsoft Presidio Analyzer](https://microsoft.github.io/presidio/) instance running, dtctl can use it for NER-based detection of PII in free-text fields (log messages, descriptions, etc.) that regex patterns might miss.

Configure via config or environment:

```yaml
# In ~/.config/dtctl/config
preferences:
  pii:
    mode: full
    presidio-url: http://localhost:5002
```

```bash
# Or via environment variable
export PRESIDIO_URL=http://localhost:5002
```

Presidio is **optional and non-fatal** — if it's unavailable, dtctl falls back to regex-only detection. The score threshold is 0.85 (high confidence).

---

## Custom Field Rules

Custom rules let you extend or override built-in PII detection. They are useful for:

- Redacting domain-specific fields that built-in patterns don't cover
- Skipping fields that built-in patterns would incorrectly flag
- Adding categories like `DEVICE_INFO` for fields you consider sensitive in your context

### Rule Format

```yaml
version: v1
rules:
  - field: "data.customerRef"     # exact field path
    category: PERSON
  - field: "usr.*"                # glob suffix — matches usr.name, usr.id, etc.
    category: PERSON
  - field: "data.internalNote"    # category defaults to REDACTED
  - field: "data.action"
    skip: true                    # override built-in match — field is preserved
```

### Rule Behavior

| Property | Description | Default |
|----------|-------------|---------|
| `field` | Field path. Exact match (`data.customerRef`) or glob suffix (`usr.*`). | Required |
| `category` | PII category label for the replacement (`[CATEGORY]` or `<CATEGORY_N>`). | `REDACTED` |
| `skip` | When `true`, excludes the field from all redaction (overrides built-in patterns). | `false` |

**Matching precedence**: Custom rules are checked **before** built-in patterns. Exact matches take priority over glob matches.

### Two Sources (Merged)

1. **Project file `.dtctl-pii.yaml`** — committed to your repo, shared with team. Searched upward from the current working directory (same as `.dtctl.yaml`).

2. **Config `preferences.pii.custom-fields`** — personal overrides in `~/.config/dtctl/config`.

When both exist, config rules **override** project rules for the same field path. Config-only rules are appended.

### Examples

**Redact a custom business field:**

```yaml
# .dtctl-pii.yaml
version: v1
rules:
  - field: "data.customerRef"
    category: PERSON
```

**Skip a field that's incorrectly flagged:**

```yaml
version: v1
rules:
  - field: "workflow.name"
    skip: true
```

**Add device fingerprinting fields (not redacted by default):**

```yaml
version: v1
rules:
  - field: "device"
    category: DEVICE_INFO
  - field: "browser"
    category: DEVICE_INFO
  - field: "os"
    category: DEVICE_INFO
  - field: "referrer"
    category: URL
```

---

## Detection Strategy

PII detection works in three phases, applied to every field in every record:

### Phase 0: Custom Rules (Highest Priority)

User-defined rules from `.dtctl-pii.yaml` and config `preferences.pii.custom-fields`. Checked against the full dotted field path. `skip: true` rules stop all further detection for that field.

### Phase 1: Field Name Heuristics

The field name (last dot-segment for dotted keys, or the full key for prefix rules like `usr.*`) is matched against regex patterns. This catches fields like `email`, `firstName`, `password` regardless of their value.

### Phase 2: Value Regex

The string value is matched against anchored regex patterns for emails and IP addresses. This catches PII in generically-named fields like `"generic_field": "john@example.com"`.

### Phase 3: Presidio NER (Full Mode Only)

Free-text fields (long strings with spaces, not UUIDs or entity IDs) are sent to Presidio for Named Entity Recognition. This catches PII embedded in prose like `"message": "Login failed for user John Doe from 10.0.0.1"`.

### Built-in Detection Coverage

| Category | Field Name Patterns | Value Patterns | Examples |
|----------|-------------------|----------------|----------|
| `EMAIL` | `email`, `e-mail`, `recipients`, `sendToOthers`, `otherEmails` | `user@domain.tld` regex | `"email": "j.doe@example.com"` |
| `PHONE` | `phone`, `mobile`, `telephone`, `fax` | — | `"phone": "+1-555-0123"` |
| `PERSON` | `firstName`, `lastName`, `fullName`, `displayName`, `userName`, `customerName`, `personName`, `givenName`, `surname`, `familyName` | — | `"firstName": "Jane"` |
| `PERSON` | `usr.*` (full-key prefix) | — | `"usr.name": "Jane"`, `"usr.id": "j42"` |
| `CREDENTIAL` | `password`, `secret`, `apiKey`, `accessToken`, `refreshToken`, `authToken`, `bearer`, `jwt` | — | `"password": "s3cr3t"` |
| `IP_ADDRESS` | `ip`, `ipAddress`, `ipv4`, `ipv6`, `remoteAddr`, `clientIp`, `sourceIp`, `destIp` | IPv4 and IPv6 regex | `"ip": "10.0.0.1"` |
| `ADDRESS` | `street`, `postalCode`, `zipCode`, `city`, `state`, `country`, `region`, `mailingAddress`, `streetAddress` | — | `"city": "Vienna"` |
| `ID_NUMBER` | `ssn`, `socialSecurity`, `taxId`, `nationalId`, `passport`, `license`, `creditCard`, `cardNumber`, `pan`, `cvv`, `cvc` | — | `"ssn": "123-45-6789"` |
| `ORGANIZATION` | `accountName`, `companyName`, `orgName`, `organizationName` | — | `"companyName": "Acme"` |

### Intentionally Excluded (Add via Custom Rules)

These fields are **not** redacted by default. They are borderline PII (useful for debugging, low identification risk alone) but can be added via [custom field rules](#custom-field-rules) if your compliance requirements are stricter.

| Field | Reason for Exclusion | Custom Rule to Add |
|-------|---------------------|--------------------|
| `device` | Device type ("iPhone 15") — low PII risk alone | `{field: "device", category: "DEVICE_INFO"}` |
| `browser` | Browser name ("Chrome 120") — technical metadata | `{field: "browser", category: "DEVICE_INFO"}` |
| `os` | OS name ("iOS 17") — technical metadata | `{field: "os", category: "DEVICE_INFO"}` |
| `referrer` | Referrer URL — may contain PII in query params | `{field: "referrer", category: "URL"}` |

---

## RUM Data Coverage

Real User Monitoring (RUM) data contains user-scoped fields under the `usr.*` prefix. These are automatically detected and redacted:

| RUM Field | Category | Detection |
|-----------|----------|-----------|
| `usr.name` | `PERSON` | `usr.*` prefix match |
| `usr.id` | `PERSON` | `usr.*` prefix match |
| `usr.email` | `PERSON` | `usr.*` prefix match |
| `usr.company` | `PERSON` | `usr.*` prefix match |
| `usr.*` (any custom tag) | `PERSON` | `usr.*` prefix match |
| `ip` | `IP_ADDRESS` | Field name regex |
| `region` | `ADDRESS` | Field name regex |
| `city` | `ADDRESS` | Field name regex |
| `country` | `ADDRESS` | Field name regex |

Note: RUM also includes `device`, `browser`, `os`, and `referrer` fields. These are not redacted by default — see [Intentionally Excluded](#intentionally-excluded-add-via-custom-rules) above.

---

## Testing Guide

This section walks through testing PII redaction end-to-end using fictitious data.

### Prerequisites

```bash
# Ensure dtctl is built
go build -o dtctl .

# Build the resolve tool
go build -o dtctl-pii-resolve ./cmd/pii-resolve/
```

### Step 1: Verify Lite Mode

Run a query with `--pii` against any environment with log data:

```bash
dtctl query "fetch logs
| filter matchesPhrase(content, \"error\")
| fields timestamp, content, dt.entity.host
| limit 5" --pii
```

Expected: string fields that match PII patterns are replaced with `[CATEGORY]` placeholders. Non-PII fields (timestamp, technical IDs) are preserved.

### Step 2: Verify Full Mode with Pseudonyms

```bash
dtctl query "fetch logs
| fields timestamp, content
| limit 10" --pii=full
```

Expected output on stderr:

```
PII session pii_20260326_143022_a1b2c3d4: redacted 12 fields across 10 records
```

Verify pseudonym stability — the same PII value in different records should get the same pseudonym (e.g., `<EMAIL_0>` appears in rows 1, 3, and 7 if it's the same email).

### Step 3: Resolve Pseudonyms

```bash
# List sessions
dtctl-pii-resolve sessions

# Resolve all mappings from the session
dtctl-pii-resolve resolve <session-id> --all
```

Expected output:

```
PSEUDONYM       ORIGINAL
<EMAIL_0>       elena.martinez@example.invalid
<EMAIL_1>       kai.tanaka@example.invalid
<PERSON_0>      Elena Martinez
<IP_ADDRESS_0>  192.168.1.42
```

### Step 4: Test Custom Rules

Create a `.dtctl-pii.yaml` file in your working directory:

```yaml
version: v1
rules:
  - field: "device"
    category: DEVICE_INFO
  - field: "content"
    skip: true
```

Run a query:

```bash
dtctl query "fetch logs | limit 5" --pii
```

Expected: `device` fields show `[DEVICE_INFO]`, `content` fields are preserved (even if they contain PII-like patterns).

### Step 5: Verify No-PII Override

```bash
# Even with DTCTL_PII=full in env, --no-pii disables redaction
DTCTL_PII=full dtctl query "fetch logs | limit 5" --no-pii
```

Expected: raw, unredacted output.

### Step 6: Test Value-Based Detection

Create a DQL query that returns email addresses in non-email-named fields:

```bash
dtctl query "fetch logs
| filter isNotNull(content)
| parse content, \"LD 'user=' LD:user_field ','\"
| fields user_field
| limit 5" --pii
```

If `user_field` contains values like `elena.martinez@example.invalid`, they should be detected by the email value regex and replaced with `[EMAIL]`.

### Step 7: Run Unit Tests

```bash
# All PII tests
go test ./pkg/pii/ -v

# Full test suite (includes golden tests)
go test ./...
```
