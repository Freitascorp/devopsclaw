#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: wait-for-rollout.sh -n namespace -d deployment [options]

Wait for a Kubernetes deployment rollout to complete.

Options:
  -n, --namespace    Kubernetes namespace (required)
  -d, --deployment   Deployment name (required)
  -T, --timeout      Timeout in seconds (default: 300)
  -c, --context      kubectl context to use (optional)
  -h, --help         Show this help
USAGE
}

namespace=""
deployment=""
timeout=300
kube_context=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    -n|--namespace)  namespace="${2-}"; shift 2 ;;
    -d|--deployment) deployment="${2-}"; shift 2 ;;
    -T|--timeout)    timeout="${2-}"; shift 2 ;;
    -c|--context)    kube_context="${2-}"; shift 2 ;;
    -h|--help)       usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$namespace" || -z "$deployment" ]]; then
  echo "namespace and deployment are required" >&2
  usage
  exit 1
fi

if ! [[ "$timeout" =~ ^[0-9]+$ ]]; then
  echo "timeout must be an integer" >&2
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

echo "Waiting for deployment/$deployment in namespace $namespace (timeout: ${timeout}s)..."

if kubectl "${ctx_args[@]}" rollout status deployment/"$deployment" \
  -n "$namespace" --timeout="${timeout}s" 2>&1; then
  echo "✓ Rollout complete: deployment/$deployment"

  # Print final status summary
  kubectl "${ctx_args[@]}" get deployment "$deployment" -n "$namespace" \
    -o jsonpath='{.metadata.name}: {.status.readyReplicas}/{.spec.replicas} ready, generation {.status.observedGeneration}' 2>/dev/null
  echo
  exit 0
else
  echo "✗ Rollout failed or timed out" >&2

  # Print diagnostic info
  echo "--- Deployment Status ---" >&2
  kubectl "${ctx_args[@]}" get deployment "$deployment" -n "$namespace" -o wide 2>/dev/null >&2 || true

  echo "--- Recent Events ---" >&2
  kubectl "${ctx_args[@]}" get events -n "$namespace" \
    --field-selector "involvedObject.name=$deployment" \
    --sort-by='.lastTimestamp' 2>/dev/null | tail -10 >&2 || true

  exit 1
fi
