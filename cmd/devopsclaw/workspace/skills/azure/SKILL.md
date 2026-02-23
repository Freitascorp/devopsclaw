---
name: azure
description: "Manage Azure infrastructure using the `az` CLI. Covers VMs, AKS, App Service, Storage, Key Vault, Azure SQL, Resource Groups, and more."
metadata: {"nanobot":{"emoji":"🔷","requires":{"bins":["az"]},"install":[{"id":"brew","kind":"brew","formula":"azure-cli","bins":["az"],"label":"Install Azure CLI (brew)"},{"id":"pip","kind":"pip","package":"azure-cli","bins":["az"],"label":"Install Azure CLI (pip)"}]}}
---

# Azure Cloud Skill

Use the `az` CLI to manage Microsoft Azure resources. Default output is JSON; use `--output table` for human-readable output, `--output tsv` for scripting.

## Authentication

```bash
# Login (opens browser)
az login

# Login with service principal
az login --service-principal -u APP_ID -p SECRET --tenant TENANT_ID

# Check current account
az account show

# List subscriptions
az account list --output table

# Switch subscription
az account set --subscription "My Subscription"
```

## Resource Groups

```bash
# List resource groups
az group list --output table

# Create a resource group
az group create --name my-rg --location eastus

# Delete a resource group (and all resources in it)
az group delete --name my-rg --yes --no-wait
```

## Virtual Machines

```bash
# List VMs
az vm list --output table
az vm list -d --output table  # includes IP addresses and power state

# Create a VM
az vm create --resource-group my-rg --name my-vm \
  --image Ubuntu2204 --size Standard_B2s \
  --admin-username azureuser --generate-ssh-keys

# Start / stop / restart / deallocate
az vm start --resource-group my-rg --name my-vm
az vm stop --resource-group my-rg --name my-vm
az vm restart --resource-group my-rg --name my-vm
az vm deallocate --resource-group my-rg --name my-vm  # stops billing

# SSH into a VM
az ssh vm --resource-group my-rg --name my-vm

# Get VM IP address
az vm show -d --resource-group my-rg --name my-vm --query publicIps -o tsv

# List VM sizes available in a region
az vm list-sizes --location eastus --output table
```

## AKS — Kubernetes

```bash
# List clusters
az aks list --output table

# Get credentials (configures kubectl)
az aks get-credentials --resource-group my-rg --name my-cluster

# Scale node pool
az aks nodepool scale --resource-group my-rg --cluster-name my-cluster \
  --name default --node-count 5

# Upgrade cluster
az aks upgrade --resource-group my-rg --name my-cluster --kubernetes-version 1.29.0

# Show cluster status
az aks show --resource-group my-rg --name my-cluster \
  --query "{name:name,status:provisioningState,version:kubernetesVersion,nodes:agentPoolProfiles[0].count}" --output table
```

## App Service — Web Apps

```bash
# List web apps
az webapp list --output table

# Create a web app
az webapp create --resource-group my-rg --plan my-plan --name my-app --runtime "NODE:20-lts"

# Deploy from local zip
az webapp deploy --resource-group my-rg --name my-app --src-path app.zip --type zip

# View logs
az webapp log tail --resource-group my-rg --name my-app

# Set environment variables
az webapp config appsettings set --resource-group my-rg --name my-app \
  --settings DB_HOST=mydb.postgres.database.azure.com NODE_ENV=production

# Restart
az webapp restart --resource-group my-rg --name my-app

# List deployment slots
az webapp deployment slot list --resource-group my-rg --name my-app --output table

# Swap staging to production
az webapp deployment slot swap --resource-group my-rg --name my-app --slot staging
```

## Storage

```bash
# List storage accounts
az storage account list --output table

# Create a storage account
az storage account create --name mystorage --resource-group my-rg \
  --location eastus --sku Standard_LRS

# List containers
az storage container list --account-name mystorage --output table

# Upload a blob
az storage blob upload --account-name mystorage --container-name data \
  --file local-file.csv --name remote/path/file.csv

# Download a blob
az storage blob download --account-name mystorage --container-name data \
  --name remote/path/file.csv --file ./local-file.csv

# Generate SAS URL (1 hour)
az storage blob generate-sas --account-name mystorage --container-name data \
  --name file.csv --permissions r --expiry $(date -u -v+1H +%Y-%m-%dT%H:%MZ) --full-uri
```

## Key Vault — Secrets

```bash
# List vaults
az keyvault list --output table

# Set a secret
az keyvault secret set --vault-name my-vault --name db-password --value "s3cret!"

# Get a secret
az keyvault secret show --vault-name my-vault --name db-password --query value -o tsv

# List secrets
az keyvault secret list --vault-name my-vault --query "[].{name:name,enabled:attributes.enabled}" --output table
```

## Azure SQL / PostgreSQL

```bash
# List SQL servers
az sql server list --output table

# List databases on a server
az sql db list --server my-server --resource-group my-rg --output table

# Show database usage
az sql db show --server my-server --resource-group my-rg --name my-db \
  --query "{name:name,status:status,maxSizeBytes:maxSizeBytes,currentServiceObjectiveName:currentServiceObjectiveName}" --output table

# List PostgreSQL flexible servers
az postgres flexible-server list --output table
```

## Networking

```bash
# List VNets
az network vnet list --output table

# List NSGs
az network nsg list --output table

# List public IPs
az network public-ip list --output table

# List load balancers
az network lb list --output table
```

## Monitor & Logs

```bash
# List activity log (last 24h)
az monitor activity-log list --start-time $(date -u -v-24H +%Y-%m-%dT%H:%M:%SZ) \
  --query "[].{time:eventTimestamp,operation:operationName.localizedValue,status:status.localizedValue}" --output table

# Query Log Analytics workspace
az monitor log-analytics query --workspace WORKSPACE_ID \
  --analytics-query "AzureActivity | where TimeGenerated > ago(1h) | take 20"
```

## Cost

```bash
# Show current billing period cost
az consumption usage list --start-date $(date +%Y-%m-01) --end-date $(date +%Y-%m-%d) \
  --query "[].{resource:instanceName,cost:pretaxCost,currency:currency}" --output table
```

## Tips

- Use `--query` with JMESPath to filter output (same as AWS).
- Use `--no-wait` for long-running operations to avoid blocking.
- Use `az interactive` for auto-completion and docs inline.
- Use `az find "keyword"` to discover commands.
- Default output is JSON; `--output table` for readability, `--output tsv` for scripts.
- Tag resources: `--tags env=prod team=platform` on create commands.
