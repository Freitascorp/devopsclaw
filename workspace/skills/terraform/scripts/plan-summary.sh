#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: plan-summary.sh [options]

Run terraform plan and produce a concise summary of changes.

Options:
  -d, --dir          Working directory (default: current dir)
  -f, --var-file     Path to .tfvars file (optional)
  -t, --target       Target specific resource (optional, repeatable)
  -j, --json         Output raw JSON plan
  -h, --help         Show this help
USAGE
}

work_dir="."
var_file=""
targets=()
json_output=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    -d|--dir)      work_dir="${2-}"; shift 2 ;;
    -f|--var-file) var_file="${2-}"; shift 2 ;;
    -t|--target)   targets+=("${2-}"); shift 2 ;;
    -j|--json)     json_output=true; shift ;;
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

# Build plan command
plan_args=(-detailed-exitcode -no-color)

if [[ -n "$var_file" ]]; then
  plan_args+=(-var-file="$var_file")
fi

for target in "${targets[@]}"; do
  plan_args+=(-target="$target")
done

# Create temp file for plan output
plan_file=$(mktemp /tmp/tf-plan-XXXXXX)
trap 'rm -f "$plan_file"' EXIT

echo "Running terraform plan..."

# terraform plan exits 0=no changes, 1=error, 2=changes
set +e
terraform plan -out="$plan_file" "${plan_args[@]}" > /tmp/tf-plan-output.txt 2>&1
plan_exit=$?
set -e

if [[ "$plan_exit" -eq 1 ]]; then
  echo "✗ Terraform plan failed:" >&2
  cat /tmp/tf-plan-output.txt >&2
  rm -f /tmp/tf-plan-output.txt
  exit 1
fi

if [[ "$json_output" == true ]]; then
  terraform show -json "$plan_file" 2>/dev/null
  rm -f /tmp/tf-plan-output.txt
  exit 0
fi

if [[ "$plan_exit" -eq 0 ]]; then
  echo "✓ No changes — infrastructure matches configuration"
  rm -f /tmp/tf-plan-output.txt
  exit 0
fi

# Parse the plan output for summary
echo ""
echo "=== Terraform Plan Summary ==="
echo ""

# Extract resource changes
terraform show -json "$plan_file" 2>/dev/null | python3 -c "
import json, sys

plan = json.load(sys.stdin)
changes = plan.get('resource_changes', [])

create = []
update = []
delete = []
replace = []
noop = []

for rc in changes:
    actions = rc.get('change', {}).get('actions', [])
    addr = rc.get('address', 'unknown')
    rtype = rc.get('type', '')

    if actions == ['create']:
        create.append(addr)
    elif actions == ['update']:
        update.append(addr)
    elif actions == ['delete']:
        delete.append(addr)
    elif 'delete' in actions and 'create' in actions:
        replace.append(addr)
    elif actions == ['no-op'] or actions == ['read']:
        noop.append(addr)

if create:
    print(f'+ CREATE ({len(create)}):')
    for r in create:
        print(f'  + {r}')
    print()

if update:
    print(f'~ UPDATE ({len(update)}):')
    for r in update:
        print(f'  ~ {r}')
    print()

if replace:
    print(f'-/+ REPLACE ({len(replace)}):')
    for r in replace:
        print(f'  -/+ {r}')
    print()

if delete:
    print(f'- DELETE ({len(delete)}):')
    for r in delete:
        print(f'  - {r}')
    print()

total = len(create) + len(update) + len(delete) + len(replace)
print(f'Total: {len(create)} to create, {len(update)} to update, {len(replace)} to replace, {len(delete)} to delete')

if delete or replace:
    print()
    print('⚠ Destructive changes detected — review carefully before applying')
" 2>/dev/null || {
  # Fallback: just show the plan text summary
  echo "(Could not parse JSON, showing raw summary)"
  grep -E "^(Plan:|  #|  ~|  -|  \+)" /tmp/tf-plan-output.txt || tail -5 /tmp/tf-plan-output.txt
}

rm -f /tmp/tf-plan-output.txt
