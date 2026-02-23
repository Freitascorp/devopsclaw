---
name: grafana
description: "Manage Grafana dashboards, data sources, alerts, and users via the Grafana HTTP API. Create, export, and import dashboards programmatically."
metadata: {"nanobot":{"emoji":"📊","requires":{"bins":["curl"]}}}
---

# Grafana Skill

Manage Grafana through its HTTP API using `curl`. Default endpoint: `http://localhost:3000`.

## Setup

```bash
export GRAFANA_URL="http://localhost:3000"
export GRAFANA_TOKEN="your-api-key-or-service-account-token"

# All requests use:
# curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" "$GRAFANA_URL/api/..."
```

## Dashboards

```bash
# Search dashboards
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/search?type=dash-db" | jq '.[] | {uid,title,url}'

# Search by tag
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/search?tag=production" | jq '.[] | {title,url}'

# Get a dashboard by UID
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/dashboards/uid/DASHBOARD_UID" | jq '.dashboard.title'

# Export a dashboard (for backup/versioning)
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/dashboards/uid/DASHBOARD_UID" | jq '.dashboard' > dashboard.json

# Import a dashboard
curl -s -X POST -H "Authorization: Bearer $GRAFANA_TOKEN" \
  -H "Content-Type: application/json" \
  "$GRAFANA_URL/api/dashboards/db" \
  -d "{\"dashboard\": $(cat dashboard.json), \"overwrite\": true, \"folderId\": 0}"

# Delete a dashboard
curl -s -X DELETE -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/dashboards/uid/DASHBOARD_UID"
```

## Data Sources

```bash
# List data sources
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/datasources" | jq '.[] | {id,name,type,url}'

# Add a Prometheus data source
curl -s -X POST -H "Authorization: Bearer $GRAFANA_TOKEN" \
  -H "Content-Type: application/json" \
  "$GRAFANA_URL/api/datasources" -d '{
    "name": "Prometheus",
    "type": "prometheus",
    "url": "http://prometheus:9090",
    "access": "proxy",
    "isDefault": true
  }'

# Test a data source
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/datasources/proxy/1/api/v1/query?query=up" | jq '.data.result | length'
```

## Folders

```bash
# List folders
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/folders" | jq '.[] | {uid,title}'

# Create a folder
curl -s -X POST -H "Authorization: Bearer $GRAFANA_TOKEN" \
  -H "Content-Type: application/json" \
  "$GRAFANA_URL/api/folders" -d '{"title": "Production Dashboards"}'
```

## Alerts

```bash
# List alert rules
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/v1/provisioning/alert-rules" | jq '.[] | {uid,title,condition}'

# List firing alerts
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/alertmanager/grafana/api/v2/alerts" | jq '.[] | {alertname:.labels.alertname,status:.status.state}'

# Silence an alert
curl -s -X POST -H "Authorization: Bearer $GRAFANA_TOKEN" \
  -H "Content-Type: application/json" \
  "$GRAFANA_URL/api/alertmanager/grafana/api/v2/silences" -d '{
    "matchers": [{"name": "alertname", "value": "HighCPU", "isRegex": false}],
    "startsAt": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'",
    "endsAt": "'$(date -u -v+2H +%Y-%m-%dT%H:%M:%SZ)'",
    "createdBy": "devopsclaw",
    "comment": "Maintenance window"
  }'

# List contact points
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/v1/provisioning/contact-points" | jq '.[] | {name,type}'
```

## Users & Organizations

```bash
# List users
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/org/users" | jq '.[] | {login,role,email}'

# Current user
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/user" | jq '{login,email,isGrafanaAdmin}'
```

## Annotations

```bash
# Add an annotation (deployment marker)
curl -s -X POST -H "Authorization: Bearer $GRAFANA_TOKEN" \
  -H "Content-Type: application/json" \
  "$GRAFANA_URL/api/annotations" -d '{
    "text": "Deployed myapp v2.1.0",
    "tags": ["deploy", "myapp"]
  }'

# List recent annotations
curl -s -H "Authorization: Bearer $GRAFANA_TOKEN" \
  "$GRAFANA_URL/api/annotations?limit=10" | jq '.[] | {text,tags,time}'
```

## Tips

- Use Grafana provisioning (YAML files in `/etc/grafana/provisioning/`) for GitOps-style management.
- Use `grafana-cli` for plugin management: `grafana-cli plugins install grafana-piechart-panel`.
- Create API keys at `$GRAFANA_URL/org/apikeys` or use service accounts.
- Use annotations to mark deployments on dashboards — shows as vertical lines on graphs.
- Export dashboards to JSON and version-control them in git.
