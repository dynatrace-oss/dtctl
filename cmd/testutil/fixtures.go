package testutil

// WorkflowYAML returns a sample workflow YAML for testing
const WorkflowYAML = `title: Test Workflow
description: A test workflow for unit tests
tasks:
  - name: task1
    action: dynatrace.automations:run-javascript
    input:
      script: |
        export default async function() {
          return { result: 'success' };
        }
`

// DashboardYAML returns a sample dashboard YAML for testing
const DashboardYAML = `dashboardMetadata:
  name: Test Dashboard
  shared: true
tiles:
  - name: tile1
    tileType: DATA_EXPLORER
    configured: true
    bounds:
      top: 0
      left: 0
      width: 400
      height: 200
`

// NotebookYAML returns a sample notebook YAML for testing
const NotebookYAML = `name: Test Notebook
content:
  sections:
    - title: Section 1
      type: markdown
      content: |
        # Test Section
        This is a test notebook section.
    - title: Section 2
      type: dql
      content: |
        fetch logs
        | limit 10
`

// SettingsYAML returns a sample settings object YAML for testing
const SettingsYAML = `schemaId: builtin:alerting.profile
scope: environment
value:
  name: Test Alerting Profile
  mzId: null
  rules: []
`

// SLODefinitionYAML returns a sample SLO YAML for testing
const SLODefinitionYAML = `name: Test SLO
description: A test SLO definition
metricExpression: (100)*(builtin:service.errors.total.rate:splitBy())/(builtin:service.requestCount.total:splitBy())
evaluationType: AGGREGATE
filter: type("SERVICE")
target: 95
warning: 97
timeframe: -1w
`

// NotificationYAML returns a sample notification configuration YAML for testing
const NotificationYAML = `name: Test Notification
type: email
enabled: true
config:
  recipients:
    - test@example.com
  subject: Test Alert
  body: Test notification body
`

// WorkflowJSON returns a sample workflow JSON for testing
const WorkflowJSON = `{
  "title": "Test Workflow",
  "description": "A test workflow",
  "tasks": [
    {
      "name": "task1",
      "action": "dynatrace.automations:run-javascript",
      "input": {
        "script": "export default async function() { return { result: 'success' }; }"
      }
    }
  ]
}`

// DashboardJSON returns a sample dashboard JSON for testing
const DashboardJSON = `{
  "dashboardMetadata": {
    "name": "Test Dashboard",
    "shared": true
  },
  "tiles": [
    {
      "name": "tile1",
      "tileType": "DATA_EXPLORER"
    }
  ]
}`

// InvalidYAML returns invalid YAML for testing error cases
const InvalidYAML = `
this is not: [valid: yaml
  - because
    it has: {incorrect: indentation
`

// InvalidJSON returns invalid JSON for testing error cases
const InvalidJSON = `{
  "invalid": "json",
  "missing": "closing brace"
`

// DQLQuery returns a sample DQL query for testing
const DQLQuery = `fetch logs
| filter status == "ERROR"
| limit 100
`

// DQLQueryWithTemplate returns a sample DQL query with template variables
const DQLQueryWithTemplate = `fetch logs
| filter host == "{{.host}}"
| filter loglevel == "{{.level}}"
| limit {{.limit}}
`
