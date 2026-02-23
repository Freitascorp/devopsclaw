#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: wait-for-ci.sh -r owner/repo -p pr_number [options]

Wait for all CI checks on a GitHub PR to complete.

Options:
  -r, --repo        Repository (owner/repo format, required)
  -p, --pr          PR number (required)
  -T, --timeout     Timeout in seconds (default: 600)
  -i, --interval    Poll interval in seconds (default: 30)
  -h, --help        Show this help

Environment:
  GH_TOKEN / GITHUB_TOKEN    GitHub authentication token
USAGE
}

repo=""
pr=""
timeout=600
interval=30

while [[ $# -gt 0 ]]; do
  case "$1" in
    -r|--repo)     repo="${2-}"; shift 2 ;;
    -p|--pr)       pr="${2-}"; shift 2 ;;
    -T|--timeout)  timeout="${2-}"; shift 2 ;;
    -i|--interval) interval="${2-}"; shift 2 ;;
    -h|--help)     usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$repo" || -z "$pr" ]]; then
  echo "repo and pr are required" >&2
  usage
  exit 1
fi

if ! [[ "$timeout" =~ ^[0-9]+$ ]]; then
  echo "timeout must be an integer" >&2
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "gh (GitHub CLI) not found in PATH" >&2
  exit 1
fi

echo "Waiting for CI checks on $repo#$pr (timeout: ${timeout}s)..."

start_epoch=$(date +%s)
deadline=$((start_epoch + timeout))

while true; do
  # Get all check runs
  checks_json=$(gh pr checks "$pr" --repo "$repo" --json name,state,completedAt,conclusion 2>/dev/null || echo "[]")

  total=$(echo "$checks_json" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
  
  if [[ "$total" == "0" ]]; then
    now=$(date +%s)
    if (( now >= deadline )); then
      echo "✗ Timed out waiting for checks to appear" >&2
      exit 1
    fi
    echo "  No checks found yet, waiting..."
    sleep "$interval"
    continue
  fi

  # Parse check states
  status=$(echo "$checks_json" | python3 -c "
import json, sys

checks = json.load(sys.stdin)
pending = []
failed = []
passed = []

for c in checks:
    name = c.get('name', 'unknown')
    state = c.get('state', '').upper()
    conclusion = c.get('conclusion', '').upper()

    if state in ('COMPLETED', 'SUCCESS'):
        if conclusion in ('SUCCESS', 'NEUTRAL', 'SKIPPED', ''):
            passed.append(name)
        else:
            failed.append((name, conclusion))
    else:
        pending.append((name, state))

if failed:
    print('FAILED')
    for name, conclusion in failed:
        print(f'  ✗ {name}: {conclusion}', file=sys.stderr)
elif pending:
    print('PENDING')
    for name, state in pending:
        print(f'  ⏳ {name}: {state}', file=sys.stderr)
else:
    print('PASSED')

print(f'{len(passed)}/{len(checks)} passed, {len(failed)} failed, {len(pending)} pending', file=sys.stderr)
" 2>/dev/null)

  state=$(echo "$status" | head -1)

  case "$state" in
    PASSED)
      echo "✓ All $total checks passed on $repo#$pr"
      exit 0
      ;;
    FAILED)
      echo "✗ CI checks failed on $repo#$pr" >&2
      echo ""
      gh pr checks "$pr" --repo "$repo" 2>/dev/null || true
      exit 1
      ;;
    PENDING)
      now=$(date +%s)
      if (( now >= deadline )); then
        echo "✗ Timed out after ${timeout}s — checks still pending" >&2
        echo ""
        gh pr checks "$pr" --repo "$repo" 2>/dev/null || true
        exit 1
      fi
      elapsed=$((now - start_epoch))
      echo "  [${elapsed}s] Checks still running..."
      sleep "$interval"
      ;;
    *)
      echo "✗ Unexpected state: $state" >&2
      exit 1
      ;;
  esac
done
