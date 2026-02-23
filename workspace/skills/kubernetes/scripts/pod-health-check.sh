#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: pod-health-check.sh -n namespace [options]

Check pod health across a namespace. Reports crash loops, pending pods,
high restart counts, and OOMKilled containers.

Options:
  -n, --namespace   Kubernetes namespace (required, use "all" for all namespaces)
  -l, --selector    Label selector to filter pods (optional)
  -c, --context     kubectl context to use (optional)
  -r, --restarts    Restart threshold to flag (default: 5)
  -h, --help        Show this help
USAGE
}

namespace=""
selector=""
kube_context=""
restart_threshold=5

while [[ $# -gt 0 ]]; do
  case "$1" in
    -n|--namespace)  namespace="${2-}"; shift 2 ;;
    -l|--selector)   selector="${2-}"; shift 2 ;;
    -c|--context)    kube_context="${2-}"; shift 2 ;;
    -r|--restarts)   restart_threshold="${2-}"; shift 2 ;;
    -h|--help)       usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$namespace" ]]; then
  echo "namespace is required" >&2
  usage
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

ns_args=(-n "$namespace")
if [[ "$namespace" == "all" ]]; then
  ns_args=(-A)
fi

sel_args=()
if [[ -n "$selector" ]]; then
  sel_args=(-l "$selector")
fi

issues_found=0

echo "=== Pod Health Check ==="
echo "Namespace: $namespace"
echo ""

# Get all pods as JSON for parsing
pods_json=$(kubectl "${ctx_args[@]}" get pods "${ns_args[@]}" "${sel_args[@]}" \
  -o json 2>/dev/null) || {
  echo "Failed to list pods" >&2
  exit 1
}

total_pods=$(echo "$pods_json" | python3 -c "import sys,json; print(len(json.load(sys.stdin)['items']))" 2>/dev/null || echo "0")
echo "Total pods: $total_pods"
echo ""

# Check for pods not in Running/Succeeded state
echo "--- Non-Running Pods ---"
not_running=$(kubectl "${ctx_args[@]}" get pods "${ns_args[@]}" "${sel_args[@]}" \
  --field-selector='status.phase!=Running,status.phase!=Succeeded' \
  --no-headers 2>/dev/null || true)

if [[ -n "$not_running" ]]; then
  echo "$not_running"
  issues_found=1
else
  echo "(none)"
fi
echo ""

# Check for crash-looping containers
echo "--- CrashLoopBackOff ---"
crashloop=$(kubectl "${ctx_args[@]}" get pods "${ns_args[@]}" "${sel_args[@]}" \
  -o jsonpath='{range .items[*]}{range .status.containerStatuses[*]}{.state.waiting.reason}{"\t"}{..metadata.namespace}/{..metadata.name}/{.name}{"\n"}{end}{end}' 2>/dev/null \
  | grep -i "CrashLoopBackOff" || true)

if [[ -n "$crashloop" ]]; then
  echo "$crashloop" | awk -F'\t' '{print $2}'
  issues_found=1
else
  echo "(none)"
fi
echo ""

# Check for high restart counts
echo "--- High Restarts (>$restart_threshold) ---"
high_restarts=$(kubectl "${ctx_args[@]}" get pods "${ns_args[@]}" "${sel_args[@]}" \
  -o jsonpath='{range .items[*]}{range .status.containerStatuses[*]}{.restartCount}{"\t"}{..metadata.namespace}/{..metadata.name}/{.name}{"\n"}{end}{end}' 2>/dev/null \
  | awk -F'\t' -v thresh="$restart_threshold" '$1+0 > thresh {printf "%s (restarts: %s)\n", $2, $1}' || true)

if [[ -n "$high_restarts" ]]; then
  echo "$high_restarts"
  issues_found=1
else
  echo "(none)"
fi
echo ""

# Check for OOMKilled
echo "--- OOMKilled ---"
oomkilled=$(kubectl "${ctx_args[@]}" get pods "${ns_args[@]}" "${sel_args[@]}" \
  -o jsonpath='{range .items[*]}{range .status.containerStatuses[*]}{.lastState.terminated.reason}{"\t"}{..metadata.namespace}/{..metadata.name}/{.name}{"\n"}{end}{end}' 2>/dev/null \
  | grep -i "OOMKilled" || true)

if [[ -n "$oomkilled" ]]; then
  echo "$oomkilled" | awk -F'\t' '{print $2}'
  issues_found=1
else
  echo "(none)"
fi
echo ""

# Summary
if [[ "$issues_found" -eq 0 ]]; then
  echo "✓ All $total_pods pods healthy"
  exit 0
else
  echo "✗ Issues detected — review above"
  exit 1
fi
