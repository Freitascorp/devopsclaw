#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: wait-healthy.sh -c container [options]

Wait for a Docker container to report healthy status.

Options:
  -c, --container   Container name or ID (required)
  -T, --timeout     Timeout in seconds (default: 120)
  -i, --interval    Poll interval in seconds (default: 2)
  -h, --help        Show this help
USAGE
}

container=""
timeout=120
interval=2

while [[ $# -gt 0 ]]; do
  case "$1" in
    -c|--container) container="${2-}"; shift 2 ;;
    -T|--timeout)   timeout="${2-}"; shift 2 ;;
    -i|--interval)  interval="${2-}"; shift 2 ;;
    -h|--help)      usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$container" ]]; then
  echo "container is required" >&2
  usage
  exit 1
fi

if ! [[ "$timeout" =~ ^[0-9]+$ ]]; then
  echo "timeout must be an integer" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not found in PATH" >&2
  exit 1
fi

# Verify container exists
if ! docker inspect "$container" >/dev/null 2>&1; then
  echo "Container '$container' not found" >&2
  exit 1
fi

# Check if container has a healthcheck defined
has_healthcheck=$(docker inspect "$container" \
  --format='{{if .Config.Healthcheck}}true{{else}}false{{end}}' 2>/dev/null || echo "false")

if [[ "$has_healthcheck" == "false" ]]; then
  echo "Warning: Container '$container' has no healthcheck defined" >&2
  echo "Falling back to checking if container is running..." >&2

  status=$(docker inspect "$container" --format='{{.State.Status}}' 2>/dev/null || echo "unknown")
  if [[ "$status" == "running" ]]; then
    echo "✓ Container is running (no healthcheck to verify)"
    exit 0
  else
    echo "✗ Container status: $status" >&2
    exit 1
  fi
fi

echo "Waiting for container '$container' to become healthy (timeout: ${timeout}s)..."

start_epoch=$(date +%s)
deadline=$((start_epoch + timeout))

while true; do
  health=$(docker inspect "$container" \
    --format='{{.State.Health.Status}}' 2>/dev/null || echo "unknown")

  case "$health" in
    healthy)
      echo "✓ Container '$container' is healthy"
      exit 0
      ;;
    unhealthy)
      echo "✗ Container '$container' is unhealthy" >&2
      echo "--- Last Health Check Log ---" >&2
      docker inspect "$container" \
        --format='{{range .State.Health.Log}}{{.Output}}{{end}}' 2>/dev/null >&2 || true
      exit 1
      ;;
    *)
      # starting or unknown — keep waiting
      ;;
  esac

  now=$(date +%s)
  if (( now >= deadline )); then
    echo "✗ Timed out after ${timeout}s (last status: $health)" >&2
    echo "--- Container Logs (last 20 lines) ---" >&2
    docker logs --tail 20 "$container" 2>&1 >&2 || true
    exit 1
  fi

  sleep "$interval"
done
