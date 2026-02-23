#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: cleanup-dangling.sh [options]

Remove dangling Docker resources (images, volumes, networks, stopped containers).

Options:
  -a, --all         Also remove unused (not just dangling) images
  -v, --volumes     Also prune unused volumes
  -n, --dry-run     Show what would be removed without removing
  -f, --force       Skip confirmation prompts
  -h, --help        Show this help
USAGE
}

all=false
volumes=false
dry_run=false
force=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    -a|--all)      all=true; shift ;;
    -v|--volumes)  volumes=true; shift ;;
    -n|--dry-run)  dry_run=true; shift ;;
    -f|--force)    force=true; shift ;;
    -h|--help)     usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not found in PATH" >&2
  exit 1
fi

echo "=== Docker Cleanup ==="
echo ""

# Show current disk usage
echo "--- Current Disk Usage ---"
docker system df 2>/dev/null || true
echo ""

if [[ "$dry_run" == true ]]; then
  echo "[DRY RUN] The following would be removed:"
  echo ""

  echo "--- Stopped Containers ---"
  docker ps -a --filter status=exited --format '{{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Size}}' 2>/dev/null || true
  echo ""

  echo "--- Dangling Images ---"
  docker images --filter dangling=true --format '{{.ID}}\t{{.Repository}}:{{.Tag}}\t{{.Size}}' 2>/dev/null || true

  if [[ "$all" == true ]]; then
    echo ""
    echo "--- Unused Images ---"
    docker images --format '{{.ID}}\t{{.Repository}}:{{.Tag}}\t{{.Size}}' 2>/dev/null || true
  fi

  echo ""
  echo "--- Dangling Volumes ---"
  docker volume ls --filter dangling=true --format '{{.Name}}\t{{.Driver}}' 2>/dev/null || true

  echo ""
  echo "--- Unused Networks ---"
  docker network ls --filter type=custom --format '{{.ID}}\t{{.Name}}\t{{.Driver}}' 2>/dev/null || true

  echo ""
  echo "Run without --dry-run to remove these resources."
  exit 0
fi

force_flag=""
if [[ "$force" == true ]]; then
  force_flag="--force"
fi

# Remove stopped containers
echo "--- Removing Stopped Containers ---"
stopped=$(docker ps -aq --filter status=exited 2>/dev/null || true)
if [[ -n "$stopped" ]]; then
  echo "$stopped" | xargs docker rm $force_flag 2>/dev/null || true
  echo "Done"
else
  echo "(none)"
fi
echo ""

# Prune images
echo "--- Pruning Images ---"
if [[ "$all" == true ]]; then
  docker image prune -a $force_flag 2>/dev/null || true
else
  docker image prune $force_flag 2>/dev/null || true
fi
echo ""

# Prune volumes (only if explicitly requested)
if [[ "$volumes" == true ]]; then
  echo "--- Pruning Volumes ---"
  docker volume prune $force_flag 2>/dev/null || true
  echo ""
fi

# Prune networks
echo "--- Pruning Networks ---"
docker network prune $force_flag 2>/dev/null || true
echo ""

# Final disk usage
echo "--- Disk Usage After Cleanup ---"
docker system df 2>/dev/null || true
echo ""
echo "✓ Cleanup complete"
