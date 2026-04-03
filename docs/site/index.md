---
layout: default
title: Home
---

<div class="hero">
  <h1><span class="accent">dtctl</span></h1>
  <p class="tagline">Your Dynatrace platform, one command away.<br>A kubectl-inspired CLI for workflows, dashboards, queries, and more.</p>

  <div class="badges">
    <a href="https://github.com/dynatrace-oss/dtctl/releases/latest"><img src="https://img.shields.io/github/v/release/dynatrace-oss/dtctl?style=flat-square" alt="Release"></a>
    <a href="https://github.com/dynatrace-oss/dtctl/actions"><img src="https://img.shields.io/github/actions/workflow/status/dynatrace-oss/dtctl/build.yml?branch=main&style=flat-square" alt="Build"></a>
    <a href="https://goreportcard.com/report/github.com/dynatrace-oss/dtctl"><img src="https://goreportcard.com/badge/github.com/dynatrace-oss/dtctl?style=flat-square" alt="Go Report"></a>
    <a href="https://github.com/dynatrace-oss/dtctl/blob/main/LICENSE"><img src="https://img.shields.io/github/license/dynatrace-oss/dtctl?style=flat-square" alt="License"></a>
  </div>

  <div class="install-boxes">
    <div class="install-box">
      <span class="install-label">macOS / Linux</span>
      <code><span class="prompt">$</span> brew install dynatrace-oss/tap/dtctl</code>
    </div>
    <div class="install-box">
      <span class="install-label">Windows</span>
      <code><span class="prompt">&gt;</span> irm https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.ps1 | iex</code>
    </div>
  </div>

  <div class="hero-buttons">
    <a href="{{ '/docs/installation/' | relative_url }}" class="btn-primary">Installation</a>
    <a href="{{ '/docs/quick-start/' | relative_url }}" class="btn-primary">Get Started</a>
    <a href="https://github.com/dynatrace-oss/dtctl" class="btn-secondary">View on GitHub</a>
  </div>
</div>

<div class="demo-container">
  <img src="https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/docs/assets/dtctl-1.gif" alt="dtctl demo showing workflow and dashboard management">
</div>

<h2 class="section-title">Why dtctl?</h2>

<div class="features">
  <div class="feature-card">
    <h3>Familiar CLI Conventions</h3>
    <p><code>get</code>, <code>describe</code>, <code>edit</code>, <code>apply</code>, <code>delete</code> &mdash; if you know kubectl, you already know dtctl. Predictable verb-noun syntax for both humans and AI agents.</p>
  </div>
  <div class="feature-card">
    <h3>Built for AI Agents</h3>
    <p>Structured JSON output (<code>--agent</code>), machine-readable command catalog (<code>dtctl commands</code>), and a bundled <a href="https://agentskills.io">Agent Skill</a> that teaches AI assistants how to operate Dynatrace.</p>
  </div>
  <div class="feature-card">
    <h3>Multi-Environment</h3>
    <p>Switch between dev, staging, and production with a single command. Safety levels prevent accidental destructive operations in the wrong environment.</p>
  </div>
  <div class="feature-card">
    <h3>DQL Passthrough</h3>
    <p>Execute DQL queries directly: <code>dtctl query "fetch logs | limit 10"</code>. File-based queries, template variables, and CSV/JSON/YAML export built in.</p>
  </div>
  <div class="feature-card">
    <h3>OAuth & Token Auth</h3>
    <p>Browser-based SSO login with automatic token refresh, or classic API tokens. Credentials stored securely in your OS keyring.</p>
  </div>
  <div class="feature-card">
    <h3>Pre-Apply Hooks</h3>
    <p>Run external validators &mdash; OPA policies, JSON Schema checks, custom linters &mdash; before <code>apply</code> sends resources to the API. Configure globally or per-context. See <a href="{{ '/docs/configuration/#pre-apply-hooks' | relative_url }}">Configuration</a>.</p>
  </div>
  <div class="feature-card">
    <h3>Watch Mode</h3>
    <p>Real-time monitoring with <code>--watch</code> for all resources. See additions, modifications, and deletions highlighted as they happen.</p>
  </div>
</div>

<h2 class="section-title">Quick Example</h2>

```bash
# List all workflows
dtctl get workflows

# Run a DQL query
dtctl query "fetch logs | filter status='ERROR' | limit 10"

# Apply configuration from a YAML file with template variables
dtctl apply -f workflow.yaml --set env=prod

# Get dashboards as JSON for automation
dtctl get dashboards -o json

# Execute a workflow and wait for completion
dtctl exec workflow "Daily Health Check" --wait --show-results

# Ask Davis CoPilot a question
dtctl exec copilot nl2dql "error logs from last hour"
```

<h2 class="section-title">Supported Resources</h2>

<table class="resource-table">
  <thead>
    <tr>
      <th>Resource</th>
      <th>Operations</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td>Workflows</td>
      <td>get, describe, create, edit, delete, execute, history, diff</td>
    </tr>
    <tr>
      <td>Dashboards & Notebooks</td>
      <td>get, describe, create, edit, delete, share, diff, history, restore</td>
    </tr>
    <tr>
      <td>DQL Queries</td>
      <td>execute, verify, template variables, live mode</td>
    </tr>
    <tr>
      <td>SLOs</td>
      <td>get, create, delete, apply, evaluate</td>
    </tr>
    <tr>
      <td>Settings</td>
      <td>get schemas, get/create/update/delete objects</td>
    </tr>
    <tr>
      <td>Buckets</td>
      <td>get, describe, create, delete</td>
    </tr>
    <tr>
      <td>Lookup Tables</td>
      <td>get, describe, create, delete (CSV auto-detection)</td>
    </tr>
    <tr>
      <td>Extensions 2.0</td>
      <td>get, describe, apply monitoring configs</td>
    </tr>
    <tr>
      <td>App Functions</td>
      <td>get, describe, execute</td>
    </tr>
    <tr>
      <td>App Intents</td>
      <td>get, describe, find, open (deep linking)</td>
    </tr>
    <tr>
      <td>Davis AI</td>
      <td>analyzers, CoPilot chat, NL-to-DQL, document search</td>
    </tr>
    <tr>
      <td>Live Debugger</td>
      <td>breakpoints, workspace filters, snapshot decoding</td>
    </tr>
    <tr>
      <td>Azure / GCP</td>
      <td>connections, monitoring configs</td>
    </tr>
  </tbody>
</table>

<h2 class="section-title">Install</h2>

```bash
# Homebrew (macOS/Linux)
brew install dynatrace-oss/tap/dtctl
```

```powershell
# PowerShell (Windows)
irm https://raw.githubusercontent.com/dynatrace-oss/dtctl/main/install.ps1 | iex
```

```bash
# Binary download
# https://github.com/dynatrace-oss/dtctl/releases/latest

# Build from source
git clone https://github.com/dynatrace-oss/dtctl.git && cd dtctl && make install
```

Then authenticate and go:

```bash
# OAuth login (recommended)
dtctl auth login --context my-env --environment "https://abc12345.apps.dynatrace.com"

# Verify
dtctl doctor

# Start using dtctl
dtctl get workflows
```

<div style="text-align: center; margin-top: 2rem;">
  <a href="{{ '/docs/quick-start/' | relative_url }}" class="btn-primary" style="display: inline-block; padding: 0.7rem 2rem; border-radius: 6px; font-weight: 600; text-decoration: none;">Read the Full Documentation</a>
</div>
