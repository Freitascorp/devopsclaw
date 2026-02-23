#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: run-checks.sh [options]

Auto-detect project type and run lint, format, type-check, and tests.
Supports: Go, Python, Node/TypeScript, Rust, Shell.

Options:
  -d, --dir        Project directory (default: current dir)
  -s, --skip       Comma-separated checks to skip: lint,format,typecheck,test
  -f, --fix        Auto-fix lint/format issues where possible
  -v, --verbose    Verbose output
  -h, --help       Show this help
USAGE
}

work_dir="."
skip=""
fix=false
verbose=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    -d|--dir)     work_dir="${2-}"; shift 2 ;;
    -s|--skip)    skip="${2-}"; shift 2 ;;
    -f|--fix)     fix=true; shift ;;
    -v|--verbose) verbose=true; shift ;;
    -h|--help)    usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

cd "$work_dir"

should_skip() {
  echo ",$skip," | grep -qi ",$1,"
}

run_step() {
  local label="$1"
  shift
  echo "--- $label ---"
  if [[ "$verbose" == true ]]; then
    echo "  \$ $*"
  fi
  set +e
  output=$("$@" 2>&1)
  rc=$?
  set -e
  if [[ $rc -eq 0 ]]; then
    echo "  ✓ passed"
  else
    echo "  ✗ failed (exit $rc)"
    if [[ -n "$output" ]]; then
      echo "$output" | head -50
    fi
    return 1
  fi
}

failures=0
detected=""

echo "=== Project Checks ==="
echo "Directory: $(pwd)"
echo ""

# Detect project type(s)
if [[ -f "go.mod" ]]; then
  detected+="go "
fi
if [[ -f "package.json" ]]; then
  detected+="node "
fi
if [[ -f "pyproject.toml" ]] || [[ -f "setup.py" ]] || [[ -f "requirements.txt" ]] || [[ -f "Pipfile" ]]; then
  detected+="python "
fi
if [[ -f "Cargo.toml" ]]; then
  detected+="rust "
fi
# Check for shell scripts
if find . -maxdepth 2 -name "*.sh" -type f 2>/dev/null | head -1 | grep -q .; then
  detected+="shell "
fi

if [[ -z "$detected" ]]; then
  echo "No supported project type detected"
  exit 0
fi

echo "Detected: $detected"
echo ""

# === Go ===
if echo "$detected" | grep -q "go"; then
  echo "==== Go ===="

  if ! should_skip "format"; then
    if [[ "$fix" == true ]]; then
      run_step "Go format (fix)" gofmt -w . || ((failures++))
    else
      set +e
      unformatted=$(gofmt -l . 2>/dev/null)
      set -e
      if [[ -n "$unformatted" ]]; then
        echo "--- Go format ---"
        echo "  ✗ Unformatted files:"
        echo "$unformatted" | sed 's/^/    /'
        ((failures++))
      else
        echo "--- Go format ---"
        echo "  ✓ passed"
      fi
    fi
  fi

  if ! should_skip "lint"; then
    if command -v golangci-lint >/dev/null 2>&1; then
      if [[ "$fix" == true ]]; then
        run_step "Go lint (fix)" golangci-lint run --fix ./... || ((failures++))
      else
        run_step "Go lint" golangci-lint run ./... || ((failures++))
      fi
    else
      run_step "Go vet" go vet ./... || ((failures++))
    fi
  fi

  if ! should_skip "test"; then
    run_step "Go test" go test -race -count=1 ./... || ((failures++))
  fi
  echo ""
fi

# === Node/TypeScript ===
if echo "$detected" | grep -q "node"; then
  echo "==== Node/TypeScript ===="

  # Detect package manager
  pkg_cmd="npm"
  if [[ -f "pnpm-lock.yaml" ]]; then
    pkg_cmd="pnpm"
  elif [[ -f "yarn.lock" ]]; then
    pkg_cmd="yarn"
  elif [[ -f "bun.lockb" ]]; then
    pkg_cmd="bun"
  fi

  if ! should_skip "lint"; then
    if [[ -f "node_modules/.bin/eslint" ]]; then
      if [[ "$fix" == true ]]; then
        run_step "ESLint (fix)" npx eslint --fix . || ((failures++))
      else
        run_step "ESLint" npx eslint . || ((failures++))
      fi
    fi
  fi

  if ! should_skip "format"; then
    if [[ -f "node_modules/.bin/prettier" ]]; then
      if [[ "$fix" == true ]]; then
        run_step "Prettier (fix)" npx prettier --write . || ((failures++))
      else
        run_step "Prettier" npx prettier --check . || ((failures++))
      fi
    fi
  fi

  if ! should_skip "typecheck"; then
    if [[ -f "tsconfig.json" ]]; then
      run_step "TypeScript" npx tsc --noEmit || ((failures++))
    fi
  fi

  if ! should_skip "test"; then
    if grep -q '"test"' package.json 2>/dev/null; then
      run_step "Tests" $pkg_cmd test || ((failures++))
    fi
  fi
  echo ""
fi

# === Python ===
if echo "$detected" | grep -q "python"; then
  echo "==== Python ===="

  if ! should_skip "lint" || ! should_skip "format"; then
    if command -v ruff >/dev/null 2>&1; then
      if ! should_skip "lint"; then
        if [[ "$fix" == true ]]; then
          run_step "Ruff lint (fix)" ruff check --fix . || ((failures++))
        else
          run_step "Ruff lint" ruff check . || ((failures++))
        fi
      fi
      if ! should_skip "format"; then
        if [[ "$fix" == true ]]; then
          run_step "Ruff format (fix)" ruff format . || ((failures++))
        else
          run_step "Ruff format" ruff format --check . || ((failures++))
        fi
      fi
    fi
  fi

  if ! should_skip "typecheck"; then
    if command -v mypy >/dev/null 2>&1; then
      run_step "Mypy" mypy . || ((failures++))
    fi
  fi

  if ! should_skip "test"; then
    if command -v pytest >/dev/null 2>&1; then
      run_step "Pytest" pytest -q || ((failures++))
    fi
  fi
  echo ""
fi

# === Rust ===
if echo "$detected" | grep -q "rust"; then
  echo "==== Rust ===="

  if ! should_skip "format"; then
    if [[ "$fix" == true ]]; then
      run_step "Rustfmt (fix)" cargo fmt || ((failures++))
    else
      run_step "Rustfmt" cargo fmt -- --check || ((failures++))
    fi
  fi

  if ! should_skip "lint"; then
    run_step "Clippy" cargo clippy -- -D warnings || ((failures++))
  fi

  if ! should_skip "test"; then
    run_step "Cargo test" cargo test || ((failures++))
  fi
  echo ""
fi

# === Shell ===
if echo "$detected" | grep -q "shell"; then
  if ! should_skip "lint"; then
    if command -v shellcheck >/dev/null 2>&1; then
      echo "==== Shell ===="
      set +e
      sh_files=$(find . -maxdepth 3 -name "*.sh" -type f 2>/dev/null)
      set -e
      if [[ -n "$sh_files" ]]; then
        run_step "ShellCheck" shellcheck $sh_files || ((failures++))
      fi
      echo ""
    fi
  fi
fi

# Summary
echo "=== Summary ==="
if [[ "$failures" -eq 0 ]]; then
  echo "✓ All checks passed"
  exit 0
else
  echo "✗ $failures check(s) failed"
  exit 1
fi
