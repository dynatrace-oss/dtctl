---
layout: docs
title: Cloud Integrations
---

dtctl supports configuring cloud monitoring integrations for **AWS**, **Azure**, and **GCP**. Each integration follows a connection-then-configuration pattern: first establish a connection with credentials, then create a monitoring configuration that defines what to monitor.

## AWS Monitoring

AWS monitoring uses role-based authentication. The connection's `objectId` is used as `sts:ExternalId` in the IAM role's trust policy, which is provisioned via a Dynatrace-maintained CloudFormation template.

### Step 1: Create an AWS Connection

```bash
# Create the connection first — the role ARN is patched in later
dtctl create aws connection --name "my-aws-connection"
```

The command prints a copy-paste `aws cloudformation deploy` snippet that creates the IAM role using the connection's `objectId` as `sts:ExternalId`.

### Step 2: Create the IAM Role (AWS CloudShell)

Run the printed snippet in AWS CloudShell. It downloads Dynatrace's least-privilege role template and deploys a CloudFormation stack:

```bash
STACK="dynatrace-monitoring-my-aws-connection"
curl -fsSLo da-role.yaml https://dynatrace-data-acquisition.s3.amazonaws.com/aws/deployment/cfn/latest/da-aws-nested-monitoring-role.yaml
aws cloudformation deploy \
  --stack-name "$STACK" \
  --template-file da-role.yaml \
  --parameter-overrides pDynatraceUrl=<your-tenant-url> pRoleExternalId=<connection-object-id> \
  --capabilities CAPABILITY_NAMED_IAM

ROLE_ARN=$(aws cloudformation describe-stacks --stack-name "$STACK" \
  --query "Stacks[0].Outputs[?OutputKey=='DynatraceMonitoringRoleArn'].OutputValue" --output text)
```

### Step 3: Patch the Role ARN into the Connection

```bash
dtctl update aws connection --name "my-aws-connection" --roleArn "$ROLE_ARN"
```

### Step 4: Create a Monitoring Configuration

`--regions` is required; `--featureSets` is optional and defaults to the extension's default set.

```bash
dtctl create aws monitoring --name "my-aws-monitoring" \
  --credentials "my-aws-connection" \
  --regions us-east-1,eu-central-1
```

> **Note:** Monitoring configurations are created in a **disabled** state. Use `dtctl enable aws monitoring` to activate them.

### Step 5: Discover Regions and Feature Sets

```bash
dtctl get aws monitoring-regions
dtctl get aws monitoring-feature-sets
```

### Step 6: Enable the Monitoring Configuration

```bash
dtctl enable aws monitoring --name "my-aws-monitoring"

# Or patch the role ARN and enable in one step:
dtctl enable aws monitoring --name "my-aws-monitoring" \
  --roleArn arn:aws:iam::123456789012:role/DynatraceMonitoringRole
```

### Step 7: Update and Delete

```bash
# Update monitoring scope
dtctl update aws monitoring --name "my-aws-monitoring" \
  --regions us-east-1,eu-central-1,ap-southeast-2 \
  --featureSets EC2_essential,RDS_essential

# Delete a monitoring config
dtctl delete aws monitoring my-aws-monitoring

# Delete the connection
dtctl delete aws connection my-aws-connection
```

## Azure Monitoring

dtctl supports two authentication types for Azure connections:

- **`federatedIdentityCredential`** (recommended) — uses workload identity federation; no long-lived secrets to manage
- **`clientSecret`** — uses a service principal with a client secret (password)

### Step 1: Choose a Subscription and Name

Pick the Azure subscription you want to monitor. The connection name is derived from it so multiple connections stay easy to tell apart:

```bash
az account list --output table

SUBSCRIPTION_ID=$(az account show --query id -o tsv)
SUBSCRIPTION_NAME=$(az account show --query name -o tsv)
TENANT_ID=$(az account show --query tenantId -o tsv)

# Subscription names can contain spaces — normalise to dashes
CONNECTION_NAME="dtctl-$(echo "$SUBSCRIPTION_NAME" | tr ' ' '-')"
```

### Option A: Federated Identity Credential (Recommended)

#### Step 2: Create the Service Principal and Assign Reader Role

```bash
CLIENT_ID=$(az ad sp create-for-rbac \
  --name "$CONNECTION_NAME" \
  --create-password false \
  --query appId -o tsv)

az role assignment create \
  --assignee "$CLIENT_ID" \
  --role Reader \
  --scope "/subscriptions/$SUBSCRIPTION_ID"
```

Repeat `az role assignment create` for each additional subscription.

#### Step 3: Create the Dynatrace Connection

The connection must be created first — its ID becomes the federated credential subject in the next step.

```bash
dtctl create azure connection \
  --name "$CONNECTION_NAME" \
  --type federatedIdentityCredential
```

#### Step 4: Configure Federated Credential in Entra ID

`dtctl create azure connection` prints the **Issuer**, **Subject**, and **Audiences** values (and a ready-to-run `az` command) immediately after the connection is created — use those values to add the federated credential.

In the Azure portal (Entra ID > App registrations > `$CONNECTION_NAME` > Certificates & secrets > Federated credentials), add a new credential using those values.

#### Step 5: Finalize the Connection

```bash
dtctl update azure connection \
  --name "$CONNECTION_NAME" \
  --directoryId "$TENANT_ID" \
  --applicationId "$CLIENT_ID"
```

### Option B: Client Secret

#### Step 2: Create the Service Principal and Assign Reader Role

`az ad sp create-for-rbac` prints the client secret **once** — capture it immediately:

```bash
SP_OUTPUT=$(az ad sp create-for-rbac --name "$CONNECTION_NAME" -o json)
CLIENT_ID=$(echo "$SP_OUTPUT" | jq -r .appId)
CLIENT_SECRET=$(echo "$SP_OUTPUT" | jq -r .password)

az role assignment create \
  --assignee "$CLIENT_ID" \
  --role Reader \
  --scope "/subscriptions/$SUBSCRIPTION_ID"
```

Repeat `az role assignment create` for each additional subscription.

#### Step 3: Create the Dynatrace Connection

All credentials are available now — create the connection in one command:

```bash
dtctl create azure connection \
  --name "$CONNECTION_NAME" \
  --type clientSecret \
  --directoryId "$TENANT_ID" \
  --applicationId "$CLIENT_ID" \
  --clientSecret "$CLIENT_SECRET"
```

> **Security tip:** `$CLIENT_SECRET` is a shell variable captured in [Step 2](#step-2-create-the-service-principal-and-assign-reader-role-1) — it doesn’t touch bash history or disk, but the expanded value can still be visible in the `dtctl` process arguments while the command runs (avoid shared machines / process-argument logging).

### Create a Monitoring Configuration

```bash
dtctl create azure monitoring-config \
  --name "$CONNECTION_NAME" \
  --credentials "$CONNECTION_NAME"
```

Optionally scope the monitoring to specific Azure regions or feature sets at creation time:

```bash
dtctl create azure monitoring-config \
  --name "$CONNECTION_NAME" \
  --credentials "$CONNECTION_NAME" \
  --locationFiltering westeurope,northeurope \
  --featureSets microsoft_compute.virtualmachines_essential,microsoft_storage.storageaccounts_essential
```

> **Note:** Monitoring configurations are created in a **disabled** state. Use `dtctl enable azure monitoring` in the next step to activate them.

### Enable the Monitoring Configuration

```bash
dtctl enable azure monitoring --name "$CONNECTION_NAME"

# Option A only — if you need to update credentials and enable in one step:
dtctl enable azure monitoring --name "$CONNECTION_NAME" \
  --directoryId "$TENANT_ID" \
  --applicationId "$CLIENT_ID"
```

## Azure Monitoring — Lifecycle Operations

### Update Location Filtering and Feature Sets

```bash
dtctl update azure monitoring-config "$CONNECTION_NAME" \
  --locationFiltering westeurope,northeurope \
  --featureSets microsoft_compute.virtualmachines_essential,microsoft_storage.storageaccounts_essential
```

### Rotate an Expired Client Secret (clientSecret type)

Use `--append` to add a new secret without immediately invalidating the old one — this gives you time to update dtctl before the old secret expires:

```bash
# Add a new secret alongside the existing one (old secret stays valid)
NEW_SECRET=$(az ad app credential reset \
  --id "$CLIENT_ID" \
  --append \
  --display-name "dtctl-$(date +%Y-%m-%d)" \
  --query password -o tsv)

# Update the connection with the new secret
dtctl update azure connection \
  --name "$CONNECTION_NAME" \
  --clientSecret "$NEW_SECRET"

# Once verified, remove the old secret from Azure portal or:
# az ad app credential delete --id "$CLIENT_ID" --key-id <old-key-id>
```

### Delete

```bash
dtctl delete azure monitoring-config "$CONNECTION_NAME"
dtctl delete azure connection "$CONNECTION_NAME"
```

## GCP Monitoring (Preview)

GCP monitoring support is currently in **Preview**.

### Step 1: Create a GCP Connection

```bash
dtctl create gcp connection --name "my-gcp-connection"
```

### Step 2: Set Up GCP Service Account

Use the `gcloud` CLI to create a service account with the required permissions:

```bash
# Create a service account
gcloud iam service-accounts create dynatrace-monitoring \
  --display-name "Dynatrace Monitoring"

# Grant monitoring read permissions
gcloud projects add-iam-policy-binding <project-id> \
  --member "serviceAccount:dynatrace-monitoring@<project-id>.iam.gserviceaccount.com" \
  --role "roles/monitoring.viewer"

# Configure workload identity federation / impersonation
# (follow the instructions from dtctl describe gcp connection)
```

### Step 3: Update the Connection

```bash
dtctl update gcp connection \
  --name "my-gcp-connection" \
  --projectId <project-id> \
  --serviceAccountEmail "dynatrace-monitoring@<project-id>.iam.gserviceaccount.com"
```

### Step 4: Create a Monitoring Configuration

```bash
# Create a monitoring config linked to the connection (created in disabled state)
dtctl create gcp monitoring-config \
  --connection "my-gcp-connection"
```

> **Note:** Monitoring configurations are created in a **disabled** state. Use `dtctl enable gcp monitoring` in the final step to activate them.

### Step 5: Discover Locations and Feature Sets

```bash
# List available GCP regions and services for monitoring
dtctl get gcp locations --connection "my-gcp-connection"
dtctl get gcp feature-sets --connection "my-gcp-connection"
```

### Step 6: Update and Delete

```bash
# Update monitoring scope
dtctl update gcp monitoring-config <config-id> \
  --locations us-central1,europe-west1 \
  --feature-sets compute,gke

# Delete a monitoring config
dtctl delete gcp monitoring-config <config-id>

# Delete the connection
dtctl delete gcp connection --name "my-gcp-connection"
```

### Step 7: Enable the Monitoring Configuration

```bash
# Enable the monitoring config (optionally setting the service account at the same time)
dtctl enable gcp monitoring --name "my-gcp-monitoring"

# Or update the service account and enable in one step:
dtctl enable gcp monitoring --name "my-gcp-monitoring" \
  --serviceAccountId "sa@project.iam.gserviceaccount.com"
```

## EdgeConnect

dtctl also provides basic management commands for Dynatrace EdgeConnect instances:

```bash
# List all EdgeConnect instances
dtctl get edgeconnects

# Create a new EdgeConnect
dtctl create edgeconnect --name "my-edge" --hostPatterns "*.internal.example.com"

# Delete an EdgeConnect
dtctl delete edgeconnect edge-123
```
