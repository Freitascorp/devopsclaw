---
name: datadog
description: "Monitor infrastructure and applications with Datadog. Use the `datadog-ci` CLI and Datadog API for metrics, logs, monitors, dashboards, and APM."
metadata: {"nanobot":{"emoji":"🐶","requires":{"bins":["curl"]}}}
---

# Datadog Skill

Interact with Datadog via its REST API using `curl`, or use `datadog-ci` for CI/CD integrations. Use `dogstatsd` for custom metrics.

## Setup

```bash
export DD_API_KEY="your-api-key"
export DD_APP_KEY="your-app-key"  # for read operations
export DD_SITE="datadoghq.com"    # or datadoghq.eu, us3.datadoghq.com, etc.
export DD_API="https://api.$DD_SITE"
```

## Metrics

```bash
# Query a metric (last 1 hour)
curl -s -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  "$DD_API/api/v1/query?from=$(date -v-1H +%s)&to=$(date +%s)&query=avg:system.cpu.user{*}" \
  | jq '.series[] | {scope,pointlist:(.pointlist | length)}'

# Submit a custom metric
curl -s -X POST -H "DD-API-KEY: $DD_API_KEY" -H "Content-Type: application/json" \
  "$DD_API/api/v2/series" -d "{
    \"series\": [{
      \"metric\": \"deploy.count\",
      \"type\": 1,
      \"points\": [{\"timestamp\": $(date +%s), \"value\": 1}],
      \"tags\": [\"env:production\", \"service:myapp\"]
    }]
  }"
```

## Monitors (Alerts)

```bash
# List monitors
curl -s -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  "$DD_API/api/v1/monitor" | jq '.[] | {id,name,overall_state,type}'

# List triggered monitors only
curl -s -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  "$DD_API/api/v1/monitor?monitor_tags=env:production" \
  | jq '[.[] | select(.overall_state != "OK")] | .[] | {id,name,overall_state}'

# Create a monitor
curl -s -X POST -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  -H "Content-Type: application/json" \
  "$DD_API/api/v1/monitor" -d '{
    "name": "High CPU on production",
    "type": "metric alert",
    "query": "avg(last_5m):avg:system.cpu.user{env:production} > 80",
    "message": "@pagerduty CPU above 80% on {{host.name}}",
    "tags": ["env:production", "team:platform"],
    "options": {
      "thresholds": {"critical": 80, "warning": 70},
      "notify_no_data": true,
      "no_data_timeframe": 10
    }
  }'

# Mute a monitor
curl -s -X POST -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  "$DD_API/api/v1/monitor/MONITOR_ID/mute"

# Unmute
curl -s -X POST -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  "$DD_API/api/v1/monitor/MONITOR_ID/unmute"
```

## Logs

```bash
# Search logs (last 15 minutes)
curl -s -X POST -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  -H "Content-Type: application/json" \
  "$DD_API/api/v2/logs/events/search" -d '{
    "filter": {
      "from": "now-15m",
      "to": "now",
      "query": "service:myapp status:error"
    },
    "sort": "-timestamp",
    "page": {"limit": 20}
  }' | jq '.data[] | {timestamp:.attributes.timestamp,message:.attributes.message}'

# Send a log
curl -s -X POST -H "DD-API-KEY: $DD_API_KEY" -H "Content-Type: application/json" \
  "https://http-intake.logs.$DD_SITE/api/v2/logs" -d '[{
    "ddsource": "devopsclaw",
    "ddtags": "env:production,service:deploy",
    "hostname": "laptop",
    "message": "Deployed myapp v2.1.0 to production"
  }]'
```

## Events

```bash
# Post a deployment event
curl -s -X POST -H "DD-API-KEY: $DD_API_KEY" -H "Content-Type: application/json" \
  "$DD_API/api/v1/events" -d '{
    "title": "Deployed myapp v2.1.0",
    "text": "Rolling deployment to 3 web servers completed successfully.",
    "tags": ["env:production", "service:myapp"],
    "alert_type": "info",
    "source_type_name": "devopsclaw"
  }'

# List recent events
curl -s -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  "$DD_API/api/v1/events?start=$(date -v-24H +%s)&end=$(date +%s)&tags=env:production" \
  | jq '.events[] | {title,date_happened,alert_type}'
```

## Hosts & Infrastructure

```bash
# List hosts
curl -s -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  "$DD_API/api/v1/hosts?count=20" | jq '.host_list[] | {name,apps,up,meta:.meta.platform}'

# Mute a host
curl -s -X POST -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  "$DD_API/api/v1/host/HOST_NAME/mute" -d '{"message":"maintenance","end":'$(date -v+2H +%s)'}'
```

## Dashboards

```bash
# List dashboards
curl -s -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  "$DD_API/api/v1/dashboard" | jq '.dashboards[] | {id,title,url}'

# Get a dashboard
curl -s -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APP_KEY" \
  "$DD_API/api/v1/dashboard/DASHBOARD_ID" > dashboard.json
```

## datadog-ci (CI/CD)

```bash
# Upload source maps
datadog-ci sourcemaps upload ./dist --service myapp --release-version v2.1.0

# Mark deployment
datadog-ci deployment mark --service myapp --env production --revision v2.1.0

# Trace a CI pipeline
datadog-ci tag --level pipeline --tags "deploy.version:v2.1.0"
```

## Tips

- Use tags everywhere: `env:X`, `service:X`, `team:X` for filtering and cost allocation.
- Use `@pagerduty`, `@slack-channel`, `@email` in monitor messages for notifications.
- Use Datadog Events to mark deployments — they appear as overlays on dashboard graphs.
- DD_SITE varies by region: `datadoghq.com` (US1), `datadoghq.eu` (EU), `us3.datadoghq.com` (US3), etc.
- Use `dogstatsd` (UDP) for high-volume custom metrics from applications.
