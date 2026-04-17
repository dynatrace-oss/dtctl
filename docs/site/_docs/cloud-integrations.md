---
layout: docs
title: Cloud Integrations
---

dtctl supports configuring cloud monitoring integrations for **Azure** and **GCP**. Each integration follows a connection-then-configuration pattern: first establish a connection with credentials, then create a monitoring configuration that defines what to monitor.

## Azure Monitoring

### Step 1: Create an Azure Connection

```bash
# Create a new Azure connection using federated identity credentials
dtctl create azure connection \
  --name "my-azure-connection" \
  --type federatedIdentityCredential
```

### Step 2: Create a Service Principal

Use the Azure CLI to create the service principal that Dynatrace will use:

```bash
# Create a service principal in Azure AD
az ad sp create-for-rbac --name "dynatrace-monitoring"
```

Note the `appId` and `tenant` from the output — you will need them in Step 5.

### Step 3: Assign Reader Role

Grant the service principal read access to the subscriptions you want to monitor:

```bash
az role assignment create \
  --assignee <appId> \
  --role Reader \
  --scope /subscriptions/<subscription-id>
```

### Step 4: Create Federated Credential in Entra ID

In the Azure portal (Entra ID > App registrations > your app > Certificates & secrets > Federated credentials), create a new federated credential using the issuer and subject values provided by `dtctl describe azure connection`.

### Step 5: Finalize the Connection

```bash
# Update the connection with your Azure directory and application IDs
dtctl update azure connection \
  --name "my-azure-connection" \
  --directoryId <tenant-id> \
  --applicationId <app-id>
```

### Step 6: Create a Monitoring Configuration

```bash
# Create a monitoring config linked to the connection (created in disabled state)
dtctl create azure monitoring-config \
  --connection "my-azure-connection"
```

> **Note:** Monitoring configurations are created in a **disabled** state. Use `dtctl enable azure monitoring` in the next step to activate them.

### Step 7: Update Location Filtering and Feature Sets

```bash
# Update monitoring to filter by Azure region or configure feature sets
dtctl update azure monitoring-config <config-id> \
  --locations westeurope,northeurope \
  --feature-sets compute,storage
```

### Step 8: Enable the Monitoring Configuration

```bash
# Enable the monitoring config (optionally updating connection credentials at the same time)
dtctl enable azure monitoring --name "my-azure-monitoring"

# Or update directory/application IDs and enable in one step:
dtctl enable azure monitoring --name "my-azure-monitoring" \
  --directoryId "$TENANT_ID" \
  --applicationId "$CLIENT_ID"
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
