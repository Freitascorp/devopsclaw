#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: drift-check.sh [options]

Detect Terraform state drift by running a refresh and comparing.

Options:
  -d, --dir          Working directory (default: current dir)
  -f, --var-file     Path to .tfvars file (optional)
  -h, --help         Show this help

Note: This runs terraform plan -refresh-only which is safe (read-only).
USAGE
}

work_dir="."
var_file=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    -d|--dir)      work_dir="${2-}"; shift 2 ;;
    -f|--var-file) var_file="${2-}"; shift 2 ;;
    -h|--help)     usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if ! command -v terraform >/dev/null 2>&1; then
  echo "terraform not found in PATH" >&2
  exit 1
fi

if [[ ! -d "$work_dir" ]]; then
  echo "Directory not found: $work_dir" >&2
  exit 1
fi

cd "$work_dir"

plan_args=(-refresh-only -detailed-exitcode -no-color)
if [[ -n "$var_file" ]]; then
  plan_args+=(-var-file="$var_file")
fi

echo "=== Terraform Drift Check ==="
echo "Directory: $(pwd)"
echo ""
echo "Running refresh-only plan..."

set +e
output=$(terraform plan "${plan_args[@]}" 2>&1)
exit_code=$?
set -e

case "$exit_code" in
  0)
    echo ""
    echo "✓ No drift detected — state matches infrastructure"
    exit 0
    ;;
  2)
    echo ""
    echo "⚠ Drift detected — infrastructure has changed outside Terraform"
    echo ""
    # Show the drifted resources
    echo "$output" | grep -E "(has changed|must be replaced|will be|~ |# )" || echo "$output" | tail -30
    echo ""
    echo "To update state: terraform apply -refresh-only"
    echo "To see full diff: terraform plan"
    exit 1
    ;;
  *)
    echo ""
    echo "✗ Terraform plan failed:" >&2
    echo "$output" >&2
    exit 1
    ;;
esac
