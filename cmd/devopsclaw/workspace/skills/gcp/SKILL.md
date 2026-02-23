---
name: gcp
description: "Manage Google Cloud infrastructure using the `gcloud` CLI. Covers Compute Engine, GKE, Cloud Run, Cloud Storage, IAM, Cloud SQL, and more."
metadata: {"nanobot":{"emoji":"🟡","requires":{"bins":["gcloud"]},"install":[{"id":"brew","kind":"brew","formula":"google-cloud-sdk","bins":["gcloud"],"label":"Install Google Cloud SDK (brew)"}]}}
---

# Google Cloud Platform Skill

Use the `gcloud` CLI to manage GCP resources. Use `gsutil` for Cloud Storage and `bq` for BigQuery (both included in the SDK).

## Authentication

```bash
# Login (opens browser)
gcloud auth login

# Application default credentials (for SDKs)
gcloud auth application-default login

# Service account
gcloud auth activate-service-account --key-file=sa-key.json

# Check active account and project
gcloud config list

# Set default project
gcloud config set project my-project-id

# Set default region/zone
gcloud config set compute/region us-central1
gcloud config set compute/zone us-central1-a

# List projects
gcloud projects list
```

## Compute Engine — VMs

```bash
# List instances
gcloud compute instances list

# Create an instance
gcloud compute instances create my-vm \
  --machine-type=e2-medium --image-family=ubuntu-2204-lts --image-project=ubuntu-os-cloud \
  --zone=us-central1-a --tags=http-server

# Start / stop / reset
gcloud compute instances start my-vm --zone=us-central1-a
gcloud compute instances stop my-vm --zone=us-central1-a
gcloud compute instances reset my-vm --zone=us-central1-a

# SSH into an instance
gcloud compute ssh my-vm --zone=us-central1-a

# Run a command remotely
gcloud compute ssh my-vm --zone=us-central1-a --command="uptime"

# Get serial console output (debugging boot issues)
gcloud compute instances get-serial-port-output my-vm --zone=us-central1-a

# List machine types
gcloud compute machine-types list --filter="zone:us-central1-a" --format="table(name,memoryMb,guestCpus)"
```

## GKE — Kubernetes

```bash
# List clusters
gcloud container clusters list

# Get credentials (configures kubectl)
gcloud container clusters get-credentials my-cluster --zone=us-central1-a

# Create a cluster
gcloud container clusters create my-cluster --num-nodes=3 --machine-type=e2-standard-4 \
  --zone=us-central1-a --enable-autoscaling --min-nodes=1 --max-nodes=10

# Resize a node pool
gcloud container clusters resize my-cluster --node-pool=default-pool --num-nodes=5 --zone=us-central1-a

# Upgrade cluster
gcloud container clusters upgrade my-cluster --zone=us-central1-a
```

## Cloud Run — Serverless Containers

```bash
# List services
gcloud run services list

# Deploy a container
gcloud run deploy my-service --image=gcr.io/my-project/my-app:latest \
  --region=us-central1 --allow-unauthenticated --port=8080 \
  --set-env-vars="DB_HOST=10.0.0.1,NODE_ENV=production"

# View logs
gcloud run services logs read my-service --region=us-central1 --limit=50

# Update traffic split (canary)
gcloud run services update-traffic my-service --region=us-central1 \
  --to-revisions=my-service-00002=10,my-service-00001=90

# Describe a service
gcloud run services describe my-service --region=us-central1
```

## Cloud Storage (gsutil)

```bash
# List buckets
gsutil ls

# List objects
gsutil ls -l gs://my-bucket/

# Copy files
gsutil cp file.txt gs://my-bucket/path/
gsutil cp gs://my-bucket/path/file.txt ./local/

# Sync directories
gsutil -m rsync -d -r ./dist/ gs://my-bucket/assets/

# Get bucket size
gsutil du -s -h gs://my-bucket/

# Signed URL (1 hour)
gsutil signurl -d 1h sa-key.json gs://my-bucket/file.zip
```

## IAM

```bash
# List service accounts
gcloud iam service-accounts list

# Create a service account
gcloud iam service-accounts create deploy-bot --display-name="Deploy Bot"

# Grant a role
gcloud projects add-iam-policy-binding my-project \
  --member="serviceAccount:deploy-bot@my-project.iam.gserviceaccount.com" \
  --role="roles/storage.admin"

# List role bindings
gcloud projects get-iam-policy my-project --format="table(bindings.role,bindings.members)"
```

## Cloud SQL

```bash
# List instances
gcloud sql instances list

# Create a PostgreSQL instance
gcloud sql instances create my-db --database-version=POSTGRES_15 \
  --tier=db-custom-2-8192 --region=us-central1

# Connect via cloud-sql-proxy
gcloud sql connect my-db --user=postgres

# Create a backup
gcloud sql backups create --instance=my-db

# List backups
gcloud sql backups list --instance=my-db
```

## Cloud Functions

```bash
# List functions
gcloud functions list

# Deploy a function
gcloud functions deploy my-func --runtime=python312 --trigger-http \
  --entry-point=handler --source=./src/ --allow-unauthenticated

# View logs
gcloud functions logs read my-func --limit=20

# Invoke
gcloud functions call my-func --data='{"key":"value"}'
```

## Networking

```bash
# List VPCs
gcloud compute networks list

# List firewall rules
gcloud compute firewall-rules list --format="table(name,direction,allowed,sourceRanges,targetTags)"

# Create a firewall rule
gcloud compute firewall-rules create allow-https \
  --network=default --allow=tcp:443 --source-ranges=0.0.0.0/0 --target-tags=http-server

# List external IPs
gcloud compute addresses list
```

## Monitoring

```bash
# List alerting policies
gcloud alpha monitoring policies list

# View logs (last 1 hour)
gcloud logging read "resource.type=gce_instance AND severity>=ERROR" --limit=20 --freshness=1h

# Stream logs
gcloud logging tail "resource.type=gce_instance"
```

## BigQuery (bq)

```bash
# List datasets
bq ls

# Run a query
bq query --use_legacy_sql=false 'SELECT COUNT(*) FROM `project.dataset.table`'

# Show table schema
bq show --schema project:dataset.table
```

## Cost

```bash
# Billing export must be configured. Query via BigQuery:
bq query --use_legacy_sql=false '
  SELECT service.description, SUM(cost) as total
  FROM `billing_export.gcp_billing_export_v1_XXXXX`
  WHERE invoice.month = FORMAT_DATE("%Y%m", CURRENT_DATE())
  GROUP BY 1 ORDER BY 2 DESC LIMIT 20'
```

## Tips

- Use `--format` to control output: `table`, `json`, `csv`, `yaml`, `value`.
- Use `--filter` for server-side filtering: `--filter="status=RUNNING"`.
- Use `gcloud config configurations` to manage multiple projects/accounts.
- Use `--quiet` to suppress prompts in scripts.
- Use `gcloud components update` to keep the SDK current.
