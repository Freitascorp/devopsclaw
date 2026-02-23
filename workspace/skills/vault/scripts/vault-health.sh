#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: vault-health.sh [options]

Check HashiCorp Vault health: seal status, HA mode, audit backends,
secret engine mounts, token info, and storage backend.

Options:
  -a, --addr        Vault address (default: VAULT_ADDR or http://127.0.0.1:8200)
  -t, --token       Vault token (default: VAULT_TOKEN env var)
  -k, --skip-tls    Skip TLS verification
  -h, --help        Show this help

Environment:
  VAULT_ADDR         Vault server address
  VAULT_TOKEN        Vault authentication token
USAGE
}

addr="${VAULT_ADDR:-http://127.0.0.1:8200}"
token="${VAULT_TOKEN:-}"
skip_tls=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    -a|--addr)     addr="${2-}"; shift 2 ;;
    -t|--token)    token="${2-}"; shift 2 ;;
    -k|--skip-tls) skip_tls=true; shift ;;
    -h|--help)     usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if ! command -v vault >/dev/null 2>&1; then
  echo "vault not found in PATH" >&2
  exit 1
fi

export VAULT_ADDR="$addr"
if [[ -n "$token" ]]; then
  export VAULT_TOKEN="$token"
fi
if [[ "$skip_tls" == true ]]; then
  export VAULT_SKIP_VERIFY=true
fi

issues=0
echo "=== Vault Health Check ==="
echo "Address: $addr"
echo ""

# Health endpoint (works even without auth)
echo "--- Status ---"
set +e
health_json=$(vault status -format=json 2>/dev/null)
status_exit=$?
set -e

if [[ -z "$health_json" ]]; then
  echo "✗ Cannot reach Vault at $addr" >&2
  exit 1
fi

sealed=$(echo "$health_json" | python3 -c "import sys,json; print(json.load(sys.stdin).get('sealed','unknown'))" 2>/dev/null)
version=$(echo "$health_json" | python3 -c "import sys,json; print(json.load(sys.stdin).get('version','unknown'))" 2>/dev/null)
ha_enabled=$(echo "$health_json" | python3 -c "import sys,json; print(json.load(sys.stdin).get('ha_enabled','unknown'))" 2>/dev/null)
cluster_name=$(echo "$health_json" | python3 -c "import sys,json; print(json.load(sys.stdin).get('cluster_name',''))" 2>/dev/null)

echo "Version: $version"
echo "Sealed: $sealed"
echo "HA enabled: $ha_enabled"
if [[ -n "$cluster_name" ]]; then
  echo "Cluster: $cluster_name"
fi

if [[ "$sealed" == "True" || "$sealed" == "true" ]]; then
  echo "✗ Vault is SEALED — unseal required" >&2
  echo ""
  echo "Unseal progress:"
  echo "$health_json" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f\"  Progress: {d.get('unseal_progress', 0)}/{d.get('unseal_threshold', 'unknown')}\")
print(f\"  Unseal nonce: {d.get('unseal_nonce', 'N/A')}\")
" 2>/dev/null
  exit 1
fi
echo ""

# The rest requires authentication
if [[ -z "$token" ]]; then
  echo "(Skipping authenticated checks — no VAULT_TOKEN set)"
  echo ""
  if [[ "$issues" -eq 0 ]]; then
    echo "✓ Vault is unsealed and reachable"
  fi
  exit 0
fi

# Token info
echo "--- Token Info ---"
set +e
token_info=$(vault token lookup -format=json 2>/dev/null)
set -e

if [[ -n "$token_info" ]]; then
  echo "$token_info" | python3 -c "
import sys, json
d = json.load(sys.stdin).get('data', {})
print(f\"  Display name: {d.get('display_name', 'N/A')}\")
print(f\"  Policies: {', '.join(d.get('policies', []))}\")
ttl = d.get('ttl', 0)
if ttl == 0:
    print('  TTL: infinite (root or no expiry)')
elif ttl < 3600:
    print(f'  ⚠ TTL: {ttl}s — token expiring soon!')
else:
    print(f'  TTL: {ttl // 3600}h {(ttl % 3600) // 60}m')
" 2>/dev/null || echo "  (could not parse token info)"
else
  echo "  ⚠ Token lookup failed — token may be invalid"
  issues=1
fi
echo ""

# Secret engines
echo "--- Secret Engines ---"
vault secrets list -format=json 2>/dev/null | python3 -c "
import sys, json
engines = json.load(sys.stdin)
for path, info in sorted(engines.items()):
    etype = info.get('type', 'unknown')
    desc = info.get('description', '')
    version = info.get('options', {}).get('version', '') if info.get('options') else ''
    ver_str = f' v{version}' if version else ''
    desc_str = f' — {desc}' if desc else ''
    print(f'  {path} ({etype}{ver_str}){desc_str}')
" 2>/dev/null || echo "  (could not list secret engines)"
echo ""

# Auth methods
echo "--- Auth Methods ---"
vault auth list -format=json 2>/dev/null | python3 -c "
import sys, json
methods = json.load(sys.stdin)
for path, info in sorted(methods.items()):
    mtype = info.get('type', 'unknown')
    desc = info.get('description', '')
    desc_str = f' — {desc}' if desc else ''
    print(f'  {path} ({mtype}){desc_str}')
" 2>/dev/null || echo "  (could not list auth methods)"
echo ""

# Audit devices
echo "--- Audit Devices ---"
audit_json=$(vault audit list -format=json 2>/dev/null || echo "{}")
audit_count=$(echo "$audit_json" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")

if [[ "$audit_count" == "0" ]]; then
  echo "  ⚠ No audit devices enabled — compliance risk"
  issues=1
else
  echo "$audit_json" | python3 -c "
import sys, json
devices = json.load(sys.stdin)
for path, info in sorted(devices.items()):
    dtype = info.get('type', 'unknown')
    print(f'  {path} ({dtype})')
" 2>/dev/null
fi
echo ""

# Summary
if [[ "$issues" -eq 0 ]]; then
  echo "✓ Vault is healthy and unsealed"
  exit 0
else
  echo "✗ Issues detected — review above"
  exit 1
fi
