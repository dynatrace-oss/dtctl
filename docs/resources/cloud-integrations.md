<!-- Migrated from the standalone docs site; SME to verify against the current dtctl binary. -->

# Cloud Integrations

<!-- NOTE: NOT a CRUD resource in the CLI - no verb exposes aws/azure/gcp; managed via the Settings API (scopes: settings:objects, extensions:configurations). Author prose + examples. -->

## Overview

dtctl supports configuring cloud monitoring integrations for AWS, Azure, and GCP (GCP support is currently in Preview). Each integration follows a connection-then-configuration pattern: first establish a connection with credentials, then create a monitoring configuration that defines what to monitor. Monitoring configurations are always created in a **disabled** state; a separate `dtctl enable` step activates them.

## Supported operations

There is no dedicated `aws`/`azure`/`gcp` resource type in the command catalog. These operations are exposed as their own verb + noun-phrase pairs (`create aws connection`, `create azure monitoring-config`, `enable gcp monitoring`, ...) layered over the Settings API, rather than generic `get`/`create`/`delete` on a single resource name.

| Operation | Resource | Command syntax | Description | Mutating | Access |
| --- | --- | --- | --- | --- | --- |
| create | aws connection | `dtctl create aws connection` | Create an AWS connection (credentials) | yes | write |
| update | aws connection | `dtctl update aws connection` | Patch an AWS connection (e.g. role ARN) | yes | write |
| delete | aws connection | `dtctl delete aws connection` | Delete an AWS connection | yes | delete |
| create | aws monitoring | `dtctl create aws monitoring` | Create an AWS monitoring configuration (disabled) | yes | write |
| update | aws monitoring | `dtctl update aws monitoring` | Update an AWS monitoring configuration's scope | yes | write |
| delete | aws monitoring | `dtctl delete aws monitoring` | Delete an AWS monitoring configuration | yes | delete |
| enable | aws monitoring | `dtctl enable aws monitoring` | Enable an AWS monitoring configuration | yes | write |
| get | aws monitoring-regions | `dtctl get aws monitoring-regions` | List available AWS regions for monitoring | no | read |
| get | aws monitoring-feature-sets | `dtctl get aws monitoring-feature-sets` | List available AWS feature sets | no | read |
| create | azure connection | `dtctl create azure connection` | Create an Azure connection (federated identity or client secret) | yes | write |
| update | azure connection | `dtctl update azure connection` | Patch an Azure connection (e.g. directory/application ID, rotate secret) | yes | write |
| delete | azure connection | `dtctl delete azure connection` | Delete an Azure connection | yes | delete |
| create | azure monitoring-config | `dtctl create azure monitoring-config` | Create an Azure monitoring configuration (disabled) | yes | write |
| update | azure monitoring-config | `dtctl update azure monitoring-config` | Update location filtering / feature sets | yes | write |
| delete | azure monitoring-config | `dtctl delete azure monitoring-config` | Delete an Azure monitoring configuration | yes | delete |
| enable | azure monitoring | `dtctl enable azure monitoring` | Enable an Azure monitoring configuration | yes | write |
| create | gcp connection | `dtctl create gcp connection` | Create a GCP connection (Preview) | yes | write |
| update | gcp connection | `dtctl update gcp connection` | Patch a GCP connection (project ID, service account) | yes | write |
| delete | gcp connection | `dtctl delete gcp connection` | Delete a GCP connection | yes | delete |
| create | gcp monitoring-config | `dtctl create gcp monitoring-config` | Create a GCP monitoring configuration (disabled) | yes | write |
| update | gcp monitoring-config | `dtctl update gcp monitoring-config` | Update a GCP monitoring configuration's scope | yes | write |
| delete | gcp monitoring-config | `dtctl delete gcp monitoring-config` | Delete a GCP monitoring configuration | yes | delete |
| enable | gcp monitoring | `dtctl enable gcp monitoring` | Enable a GCP monitoring configuration | yes | write |
| get | gcp locations | `dtctl get gcp locations` | List available GCP regions for monitoring | no | read |
| get | gcp feature-sets | `dtctl get gcp feature-sets` | List available GCP feature sets | no | read |
| create | edgeconnect | `dtctl create edgeconnect` | Create a Dynatrace EdgeConnect instance | yes | write |
| get | edgeconnects | `dtctl get edgeconnects` | List EdgeConnect instances | no | read |
| delete | edgeconnect | `dtctl delete edgeconnect` | Delete an EdgeConnect instance | yes | delete |

## Flags

Resource commands take dtctl's **global flags** (`-o/--output`, `--dry-run`, `--context`, `--jq`, `-v`, ...). Cloud-integration commands add their own noun-specific flags, such as `--name`, `--regions`, `--featureSets`, `--credentials`, `--roleArn` (AWS), `--type`, `--directoryId`, `--applicationId`, `--clientSecret`, `--locationFiltering` (Azure), and `--projectId`, `--serviceAccountEmail`, `--connection` (GCP). See the examples below for their usage in context.

## Required token scopes

Cloud integration connections and monitoring configurations are managed through the Settings API and, for the underlying extension configuration, the Extensions API:

| Safety level | Scopes |
| --- | --- |
| read | `settings:objects:read`, `extensions:configurations:read` |
| write | `settings:objects:write`, `extensions:configurations:write` |

EdgeConnect management uses its own scopes: `app-engine:edge-connects:read` (read), `app-engine:edge-connects:write` (write), `app-engine:edge-connects:delete` (delete).

## Output

`create`/`update` commands print the created or updated object's identifying fields; `create aws connection` and `create azure connection` additionally print copy-paste setup material (a CloudFormation snippet for AWS; Issuer/Subject/Audiences values for Azure federated identity). `get aws monitoring-regions`, `get aws monitoring-feature-sets`, `get gcp locations`, and `get gcp feature-sets` print the discoverable values needed for `--regions`/`--featureSets`/`--locations`/`--feature-sets` flags elsewhere in the workflow.

## Examples

### AWS monitoring

AWS monitoring uses role-based authentication: the connection's `objectId` becomes `sts:ExternalId` in the IAM role's trust policy, provisioned via a Dynatrace-maintained CloudFormation template.

```bash
# Step 1: create the connection first — the role ARN is patched in later
dtctl create aws connection --name "my-aws-connection"
```

The command prints a copy-paste `aws cloudformation deploy` snippet that creates the IAM role using the connection's `objectId` as `sts:ExternalId`. Run it in AWS CloudShell, then patch the resulting role ARN into the connection:

```bash
dtctl update aws connection --name "my-aws-connection" --roleArn "$ROLE_ARN"
```

Create a monitoring configuration (`--regions` is required; `--featureSets` is optional and defaults to the extension's default set). Monitoring configurations are created disabled:

```bash
dtctl create aws monitoring --name "my-aws-monitoring" \
  --credentials "my-aws-connection" \
  --regions us-east-1,eu-central-1
```

Discover regions and feature sets, then enable:

```bash
dtctl get aws monitoring-regions
dtctl get aws monitoring-feature-sets

dtctl enable aws monitoring --name "my-aws-monitoring"

# Or patch the role ARN and enable in one step:
dtctl enable aws monitoring --name "my-aws-monitoring" \
  --roleArn arn:aws:iam::123456789012:role/DynatraceMonitoringRole
```

Update and delete:

```bash
dtctl update aws monitoring --name "my-aws-monitoring" \
  --regions us-east-1,eu-central-1,ap-southeast-2 \
  --featureSets EC2_essential,RDS_essential

dtctl delete aws monitoring my-aws-monitoring
dtctl delete aws connection my-aws-connection
```

### Azure monitoring

dtctl supports two Azure authentication types: `federatedIdentityCredential` (recommended, no long-lived secrets) and `clientSecret` (service principal with a password).

```bash
# Create the service principal and assign the Reader role
CLIENT_ID=$(az ad sp create-for-rbac --name "$CONNECTION_NAME" --create-password false --query appId -o tsv)
az role assignment create --assignee "$CLIENT_ID" --role Reader --scope "/subscriptions/$SUBSCRIPTION_ID"

# Create the connection — its ID becomes the federated credential subject
dtctl create azure connection --name "$CONNECTION_NAME" --type federatedIdentityCredential
```

`dtctl create azure connection` prints the Issuer, Subject, and Audiences values (and a ready-to-run `az` command) to add as a federated credential in Entra ID. Then finalize:

```bash
dtctl update azure connection \
  --name "$CONNECTION_NAME" \
  --directoryId "$TENANT_ID" \
  --applicationId "$CLIENT_ID"
```

For the `clientSecret` type, create the connection with the secret directly:

```bash
dtctl create azure connection \
  --name "$CONNECTION_NAME" \
  --type clientSecret \
  --directoryId "$TENANT_ID" \
  --applicationId "$CLIENT_ID" \
  --clientSecret "$CLIENT_SECRET"
```

Create a monitoring configuration (created disabled), optionally scoped to regions/feature sets, then enable:

```bash
dtctl create azure monitoring-config \
  --name "$CONNECTION_NAME" \
  --credentials "$CONNECTION_NAME" \
  --locationFiltering westeurope,northeurope \
  --featureSets microsoft_compute.virtualmachines_essential

dtctl enable azure monitoring --name "$CONNECTION_NAME"
```

Rotate an expired client secret with `--append`, so the old secret stays valid until you cut over:

```bash
NEW_SECRET=$(az ad app credential reset --id "$CLIENT_ID" --append \
  --display-name "dtctl-$(date +%Y-%m-%d)" --query password -o tsv)

dtctl update azure connection --name "$CONNECTION_NAME" --clientSecret "$NEW_SECRET"
```

Update and delete:

```bash
dtctl update azure monitoring-config "$CONNECTION_NAME" \
  --locationFiltering westeurope,northeurope \
  --featureSets microsoft_compute.virtualmachines_essential

dtctl delete azure monitoring-config "$CONNECTION_NAME"
dtctl delete azure connection "$CONNECTION_NAME"
```

### GCP monitoring (Preview)

```bash
# Create the connection, then set up a GCP service account with gcloud
dtctl create gcp connection --name "my-gcp-connection"

gcloud iam service-accounts create dynatrace-monitoring --display-name "Dynatrace Monitoring"
gcloud projects add-iam-policy-binding <project-id> \
  --member "serviceAccount:dynatrace-monitoring@<project-id>.iam.gserviceaccount.com" \
  --role "roles/monitoring.viewer"

# Update the connection with the service account
dtctl update gcp connection \
  --name "my-gcp-connection" \
  --projectId <project-id> \
  --serviceAccountEmail "dynatrace-monitoring@<project-id>.iam.gserviceaccount.com"

# Create a monitoring config linked to the connection (created disabled)
dtctl create gcp monitoring-config --connection "my-gcp-connection"

# Discover locations and feature sets
dtctl get gcp locations --connection "my-gcp-connection"
dtctl get gcp feature-sets --connection "my-gcp-connection"

# Enable
dtctl enable gcp monitoring --name "my-gcp-monitoring"
```

Update and delete:

```bash
dtctl update gcp monitoring-config <config-id> \
  --locations us-central1,europe-west1 \
  --feature-sets compute,gke

dtctl delete gcp monitoring-config <config-id>
dtctl delete gcp connection --name "my-gcp-connection"
```

### EdgeConnect

dtctl also provides basic management commands for Dynatrace EdgeConnect instances:

```bash
dtctl get edgeconnects
dtctl create edgeconnect --name "my-edge" --hostPatterns "*.internal.example.com"
dtctl delete edgeconnect edge-123
```

See [EdgeConnect](edgeconnect) for the dedicated resource reference.
