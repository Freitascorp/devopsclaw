---
name: prometheus
description: "Query and manage Prometheus metrics. PromQL queries, alerting rules, targets, recording rules, and troubleshooting via the Prometheus HTTP API."
metadata: {"nanobot":{"emoji":"🔥","requires":{"bins":["curl"]}}}
---

# Prometheus Skill

Query Prometheus using its HTTP API with `curl`, or use `promtool` for rule validation. Default endpoint: `http://localhost:9090`.

## Setup

```bash
# Set base URL
export PROM_URL="http://localhost:9090"

# Or for Prometheus behind auth
export PROM_URL="https://prometheus.example.com"
```

## Instant Queries

```bash
# Simple metric query
curl -s "$PROM_URL/api/v1/query?query=up" | jq '.data.result[] | {instance:.metric.instance,job:.metric.job,value:.value[1]}'

# CPU usage by instance
curl -s "$PROM_URL/api/v1/query" --data-urlencode \
  'query=100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)' | jq '.data.result[]'

# Memory usage percentage
curl -s "$PROM_URL/api/v1/query" --data-urlencode \
  'query=(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100' | jq '.data.result[]'

# Disk usage
curl -s "$PROM_URL/api/v1/query" --data-urlencode \
  'query=(1 - node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"}) * 100' | jq '.data.result[]'
```

## Range Queries

```bash
# CPU over the last hour (1-minute steps)
curl -s "$PROM_URL/api/v1/query_range" --data-urlencode \
  'query=100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)' \
  --data-urlencode "start=$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ)" \
  --data-urlencode "end=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --data-urlencode "step=60s" | jq '.data.result[0].values | length'

# Request rate over time
curl -s "$PROM_URL/api/v1/query_range" --data-urlencode \
  'query=sum(rate(http_requests_total[5m])) by (status_code)' \
  --data-urlencode "start=$(date -u -v-6H +%Y-%m-%dT%H:%M:%SZ)" \
  --data-urlencode "end=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --data-urlencode "step=300s" | jq '.data.result[] | {status:.metric.status_code}'
```

## Common PromQL Patterns

```promql
# Request rate (per second)
rate(http_requests_total[5m])

# Error rate percentage
sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) * 100

# 95th percentile latency
histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le))

# Top 5 pods by CPU
topk(5, sum(rate(container_cpu_usage_seconds_total[5m])) by (pod))

# Memory usage per pod
sum(container_memory_working_set_bytes) by (pod, namespace) / 1024 / 1024  # MB

# Disk I/O rate
rate(node_disk_read_bytes_total[5m]) + rate(node_disk_written_bytes_total[5m])

# Network traffic
sum(rate(node_network_receive_bytes_total[5m])) by (instance)

# Pods not ready
kube_pod_status_ready{condition="false"}

# Deployment replicas mismatch
kube_deployment_spec_replicas != kube_deployment_status_available_replicas
```

## Targets & Service Discovery

```bash
# List all targets
curl -s "$PROM_URL/api/v1/targets" | jq '.data.activeTargets[] | {job:.labels.job,instance:.labels.instance,health,lastScrape:.lastScrape}'

# Unhealthy targets only
curl -s "$PROM_URL/api/v1/targets" | jq '[.data.activeTargets[] | select(.health != "up")] | length'

# List all label values for a label
curl -s "$PROM_URL/api/v1/label/job/values" | jq '.data'

# List all metric names
curl -s "$PROM_URL/api/v1/label/__name__/values" | jq '.data | length'
```

## Alerts

```bash
# List active alerts
curl -s "$PROM_URL/api/v1/alerts" | jq '.data.alerts[] | {alertname:.labels.alertname,state,severity:.labels.severity}'

# List alerting rules
curl -s "$PROM_URL/api/v1/rules?type=alert" | jq '.data.groups[].rules[] | select(.type=="alerting") | {name,state,query:.query}'
```

## promtool

```bash
# Check config syntax
promtool check config prometheus.yml

# Check alerting rules
promtool check rules alerts.yml

# Test alerting rules
promtool test rules test.yml

# Query from CLI
promtool query instant http://localhost:9090 'up'
```

## Common Alerting Rules

```yaml
groups:
  - name: infrastructure
    rules:
      - alert: HighCPU
        expr: 100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100) > 80
        for: 5m
        labels: { severity: warning }
        annotations: { summary: "High CPU on {{ $labels.instance }}" }

      - alert: DiskAlmostFull
        expr: (1 - node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"}) * 100 > 85
        for: 10m
        labels: { severity: critical }

      - alert: TargetDown
        expr: up == 0
        for: 3m
        labels: { severity: critical }

      - alert: HighErrorRate
        expr: sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) > 0.05
        for: 5m
        labels: { severity: warning }
```

## Tips

- Use `rate()` for counters, never raw counter values.
- Use `irate()` for volatile, short-lived spikes.
- Use `increase()` for total increase over a time window.
- Use `--data-urlencode` with curl for complex PromQL (avoids shell escaping issues).
- Use `{__name__=~"node_.*"}` to discover metrics for a specific exporter.
- Use `count(up)` to check how many targets exist.

## Bundled Scripts

- Query Prometheus: `{baseDir}/scripts/query-prom.sh -q 'up'` (supports instant and range queries with text/json/csv output)
