#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: query-prom.sh -q query [options]

Execute a PromQL query against Prometheus and format the results.

Options:
  -q, --query       PromQL query (required)
  -u, --url         Prometheus URL (default: PROMETHEUS_URL or http://localhost:9090)
  -t, --time        Evaluation timestamp (RFC3339 or Unix, default: now)
  -r, --range       Range query: start,end,step (e.g. "1h,now,5m")
  -o, --output      Output format: text, json, csv (default: text)
  -h, --help        Show this help

Environment:
  PROMETHEUS_URL     Prometheus server URL

Examples:
  query-prom.sh -q 'up'
  query-prom.sh -q 'rate(http_requests_total[5m])' -r "1h,now,1m"
  query-prom.sh -q 'node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes * 100'
USAGE
}

query=""
prom_url="${PROMETHEUS_URL:-http://localhost:9090}"
eval_time=""
range_spec=""
output_fmt="text"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -q|--query)   query="${2-}"; shift 2 ;;
    -u|--url)     prom_url="${2-}"; shift 2 ;;
    -t|--time)    eval_time="${2-}"; shift 2 ;;
    -r|--range)   range_spec="${2-}"; shift 2 ;;
    -o|--output)  output_fmt="${2-}"; shift 2 ;;
    -h|--help)    usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$query" ]]; then
  echo "query is required" >&2
  usage
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl not found in PATH" >&2
  exit 1
fi

# Determine if instant or range query
if [[ -n "$range_spec" ]]; then
  # Parse range: start,end,step
  IFS=',' read -r range_start range_end range_step <<< "$range_spec"

  # Convert relative times
  now_epoch=$(date +%s)
  convert_time() {
    local t="$1"
    case "$t" in
      now) echo "$now_epoch" ;;
      *h)  echo "$((now_epoch - ${t%h} * 3600))" ;;
      *m)  echo "$((now_epoch - ${t%m} * 60))" ;;
      *d)  echo "$((now_epoch - ${t%d} * 86400))" ;;
      *)   echo "$t" ;;
    esac
  }

  start_ts=$(convert_time "$range_start")
  end_ts=$(convert_time "$range_end")

  api_path="/api/v1/query_range"
  curl_data=(
    --data-urlencode "query=$query"
    --data-urlencode "start=$start_ts"
    --data-urlencode "end=$end_ts"
    --data-urlencode "step=$range_step"
  )
else
  api_path="/api/v1/query"
  curl_data=(--data-urlencode "query=$query")
  if [[ -n "$eval_time" ]]; then
    curl_data+=(--data-urlencode "time=$eval_time")
  fi
fi

# Execute query
response=$(curl -sS --fail-with-body \
  "${prom_url}${api_path}" \
  "${curl_data[@]}" 2>&1) || {
  echo "✗ Query failed:" >&2
  echo "$response" >&2
  exit 1
}

# Check for API error
status=$(echo "$response" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
if [[ "$status" != "success" ]]; then
  error_msg=$(echo "$response" | python3 -c "import sys,json; print(json.load(sys.stdin).get('error','unknown error'))" 2>/dev/null || echo "$response")
  echo "✗ Prometheus error: $error_msg" >&2
  exit 1
fi

if [[ "$output_fmt" == "json" ]]; then
  echo "$response" | python3 -m json.tool 2>/dev/null || echo "$response"
  exit 0
fi

# Format output
echo "$response" | python3 -c "
import json, sys
from datetime import datetime, timezone

data = json.load(sys.stdin)
result = data.get('data', {})
result_type = result.get('resultType', 'unknown')
results = result.get('result', [])

output_fmt = '$output_fmt'

if not results:
    print('(no results)')
    sys.exit(0)

if result_type == 'vector':
    # Instant query results
    if output_fmt == 'csv':
        print('metric,value,timestamp')
        for r in results:
            labels = ','.join(f'{k}={v}' for k, v in sorted(r.get('metric', {}).items()))
            ts, val = r['value']
            print(f'{labels},{val},{ts}')
    else:
        print(f'Results: {len(results)} series')
        print()
        for r in results:
            metric = r.get('metric', {})
            name = metric.get('__name__', '')
            labels = ', '.join(f'{k}={v}' for k, v in sorted(metric.items()) if k != '__name__')
            ts, val = r['value']
            ts_str = datetime.fromtimestamp(ts, tz=timezone.utc).strftime('%H:%M:%S')
            label_str = f'{{{labels}}}' if labels else ''
            print(f'  {name}{label_str} = {val}  @ {ts_str}')

elif result_type == 'matrix':
    # Range query results
    if output_fmt == 'csv':
        print('metric,timestamp,value')
        for r in results:
            labels = ','.join(f'{k}={v}' for k, v in sorted(r.get('metric', {}).items()))
            for ts, val in r.get('values', []):
                print(f'{labels},{ts},{val}')
    else:
        print(f'Results: {len(results)} series (range)')
        print()
        for r in results:
            metric = r.get('metric', {})
            name = metric.get('__name__', '')
            labels = ', '.join(f'{k}={v}' for k, v in sorted(metric.items()) if k != '__name__')
            label_str = f'{{{labels}}}' if labels else ''
            values = r.get('values', [])
            print(f'  {name}{label_str} ({len(values)} points)')
            # Show first, last, min, max
            if values:
                nums = [float(v) for _, v in values]
                print(f'    range: {min(nums):.4g} — {max(nums):.4g}')
                print(f'    first: {values[0][1]}  last: {values[-1][1]}')

elif result_type == 'scalar':
    ts, val = result.get('result', [0, ''])
    print(f'Scalar: {val}')

else:
    print(f'Result type: {result_type}')
    print(json.dumps(results, indent=2))
" 2>/dev/null || {
  echo "(Could not format — raw response:)"
  echo "$response"
}
