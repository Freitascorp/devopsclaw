#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: check-cert-expiry.sh [options]

Check TLS certificate expiry for cert-manager certificates.
Flags certificates expiring within the warning threshold.

Options:
  -n, --namespace   Kubernetes namespace (default: all namespaces)
  -w, --warn-days   Days before expiry to warn (default: 30)
  -c, --context     kubectl context to use (optional)
  -o, --output      Output format: text, json (default: text)
  -h, --help        Show this help
USAGE
}

namespace=""
warn_days=30
kube_context=""
output_fmt="text"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -n|--namespace) namespace="${2-}"; shift 2 ;;
    -w|--warn-days) warn_days="${2-}"; shift 2 ;;
    -c|--context)   kube_context="${2-}"; shift 2 ;;
    -o|--output)    output_fmt="${2-}"; shift 2 ;;
    -h|--help)      usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if ! [[ "$warn_days" =~ ^[0-9]+$ ]]; then
  echo "warn-days must be an integer" >&2
  exit 1
fi

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl not found in PATH" >&2
  exit 1
fi

ctx_args=()
if [[ -n "$kube_context" ]]; then
  ctx_args=(--context "$kube_context")
fi

ns_args=(-A)
if [[ -n "$namespace" ]]; then
  ns_args=(-n "$namespace")
fi

now_epoch=$(date +%s)
warn_epoch=$((now_epoch + warn_days * 86400))

# Get all certificates
certs_json=$(kubectl "${ctx_args[@]}" get certificates "${ns_args[@]}" -o json 2>/dev/null) || {
  echo "Failed to list certificates. Is cert-manager installed?" >&2
  exit 1
}

cert_count=$(echo "$certs_json" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('items',[])))" 2>/dev/null || echo "0")

if [[ "$cert_count" == "0" ]]; then
  echo "No certificates found"
  exit 0
fi

expiring=0
expired=0
healthy=0
results="[]"

# Process each certificate
results=$(echo "$certs_json" | python3 -c "
import json, sys
from datetime import datetime, timezone

data = json.load(sys.stdin)
now = datetime.now(timezone.utc)
warn_days = $warn_days
results = []

for cert in data.get('items', []):
    name = cert['metadata']['name']
    ns = cert['metadata']['namespace']
    
    # Get expiry from status
    not_after = cert.get('status', {}).get('notAfter', '')
    ready = 'Unknown'
    for cond in cert.get('status', {}).get('conditions', []):
        if cond.get('type') == 'Ready':
            ready = cond.get('status', 'Unknown')
            break
    
    if not not_after:
        results.append({
            'namespace': ns, 'name': name, 'status': 'unknown',
            'ready': ready, 'expiry': 'N/A', 'days_left': -1
        })
        continue
    
    # Parse ISO 8601 date
    try:
        expiry = datetime.fromisoformat(not_after.replace('Z', '+00:00'))
    except:
        expiry = datetime.strptime(not_after, '%Y-%m-%dT%H:%M:%SZ').replace(tzinfo=timezone.utc)
    
    days_left = (expiry - now).days
    
    if days_left < 0:
        status = 'expired'
    elif days_left <= warn_days:
        status = 'expiring'
    else:
        status = 'ok'
    
    results.append({
        'namespace': ns, 'name': name, 'status': status,
        'ready': ready, 'expiry': not_after, 'days_left': days_left
    })

# Sort: expired first, then expiring, then ok
order = {'expired': 0, 'expiring': 1, 'unknown': 2, 'ok': 3}
results.sort(key=lambda r: (order.get(r['status'], 9), r['days_left']))

print(json.dumps(results))
" 2>/dev/null)

if [[ "$output_fmt" == "json" ]]; then
  echo "$results" | python3 -m json.tool 2>/dev/null || echo "$results"
  exit 0
fi

# Text output
echo "=== Certificate Expiry Report ==="
echo "Warning threshold: ${warn_days} days"
echo "Total certificates: $cert_count"
echo ""

echo "$results" | python3 -c "
import json, sys
results = json.loads(sys.stdin.read())

expired = [r for r in results if r['status'] == 'expired']
expiring = [r for r in results if r['status'] == 'expiring']
ok = [r for r in results if r['status'] == 'ok']
unknown = [r for r in results if r['status'] == 'unknown']

if expired:
    print('--- EXPIRED ---')
    for r in expired:
        print(f\"  ✗ {r['namespace']}/{r['name']} — expired {abs(r['days_left'])} days ago (Ready: {r['ready']})\")
    print()

if expiring:
    print('--- EXPIRING SOON ---')
    for r in expiring:
        print(f\"  ⚠ {r['namespace']}/{r['name']} — {r['days_left']} days left (expires: {r['expiry']}, Ready: {r['ready']})\")
    print()

if unknown:
    print('--- UNKNOWN ---')
    for r in unknown:
        print(f\"  ? {r['namespace']}/{r['name']} — no expiry date (Ready: {r['ready']})\")
    print()

if ok:
    print(f'--- HEALTHY ({len(ok)}) ---')
    for r in ok:
        print(f\"  ✓ {r['namespace']}/{r['name']} — {r['days_left']} days left\")
    print()

total_issues = len(expired) + len(expiring)
if total_issues == 0:
    print(f'✓ All {len(results)} certificates healthy')
else:
    print(f'✗ {total_issues} certificate(s) need attention')
" 2>/dev/null

# Exit with error if any are expired or expiring
has_issues=$(echo "$results" | python3 -c "
import json, sys
results = json.loads(sys.stdin.read())
issues = [r for r in results if r['status'] in ('expired', 'expiring')]
print('1' if issues else '0')
" 2>/dev/null || echo "0")

exit "$has_issues"
