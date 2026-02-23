#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: pg-slow-queries.sh [options]

Extract and analyze slow queries from PostgreSQL using pg_stat_statements.

Options:
  -H, --host        PostgreSQL host (default: localhost)
  -p, --port        PostgreSQL port (default: 5432)
  -U, --user        PostgreSQL user (default: postgres)
  -d, --dbname      Database name (default: postgres)
  -l, --limit       Number of top queries to show (default: 20)
  -s, --sort        Sort by: total_time, mean_time, calls, rows (default: total_time)
  -m, --min-calls   Minimum call count to include (default: 1)
  -h, --help        Show this help

Environment:
  PGPASSWORD         Password (or use .pgpass)
  DATABASE_URL       Full connection string (overrides host/port/user/dbname)

Note: Requires pg_stat_statements extension to be installed and loaded.
USAGE
}

host="localhost"
port=5432
user="postgres"
dbname="postgres"
limit=20
sort_by="total_time"
min_calls=1

while [[ $# -gt 0 ]]; do
  case "$1" in
    -H|--host)      host="${2-}"; shift 2 ;;
    -p|--port)      port="${2-}"; shift 2 ;;
    -U|--user)      user="${2-}"; shift 2 ;;
    -d|--dbname)    dbname="${2-}"; shift 2 ;;
    -l|--limit)     limit="${2-}"; shift 2 ;;
    -s|--sort)      sort_by="${2-}"; shift 2 ;;
    -m|--min-calls) min_calls="${2-}"; shift 2 ;;
    -h|--help)      usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if ! command -v psql >/dev/null 2>&1; then
  echo "psql not found in PATH" >&2
  exit 1
fi

# Validate sort column
case "$sort_by" in
  total_time|mean_time|calls|rows) ;;
  *) echo "Invalid sort: $sort_by (use total_time, mean_time, calls, rows)" >&2; exit 1 ;;
esac

psql_cmd=(psql -h "$host" -p "$port" -U "$user" -d "$dbname" -X)
if [[ -n "${DATABASE_URL:-}" ]]; then
  psql_cmd=(psql "$DATABASE_URL" -X)
fi

run_query() {
  "${psql_cmd[@]}" -t -A -c "$1" 2>/dev/null
}

# Check if pg_stat_statements is available
if ! run_query "SELECT 1 FROM pg_stat_statements LIMIT 1" >/dev/null 2>&1; then
  echo "✗ pg_stat_statements not available" >&2
  echo "" >&2
  echo "To install:" >&2
  echo "  1. Add to postgresql.conf: shared_preload_libraries = 'pg_stat_statements'" >&2
  echo "  2. Restart PostgreSQL" >&2
  echo "  3. Run: CREATE EXTENSION IF NOT EXISTS pg_stat_statements;" >&2
  exit 1
fi

# Detect column names (PG13+ uses total_exec_time, older uses total_time)
has_exec_time=$(run_query "
SELECT column_name FROM information_schema.columns
WHERE table_name = 'pg_stat_statements' AND column_name = 'total_exec_time'
" 2>/dev/null || true)

if [[ -n "$has_exec_time" ]]; then
  time_col="total_exec_time"
  mean_col="mean_exec_time"
else
  time_col="total_time"
  mean_col="mean_time"
fi

# Map sort_by to actual column
case "$sort_by" in
  total_time) order_col="$time_col" ;;
  mean_time)  order_col="$mean_col" ;;
  calls)      order_col="calls" ;;
  rows)       order_col="rows" ;;
esac

echo "=== Slow Query Report ==="
echo "Database: $dbname | Sort: $sort_by | Top $limit | Min calls: $min_calls"
echo ""

# Run the analysis
"${psql_cmd[@]}" <<SQL
SELECT
  row_number() OVER (ORDER BY $order_col DESC) AS rank,
  round(${time_col}::numeric / 1000, 2) AS total_sec,
  calls,
  round(${mean_col}::numeric / 1000, 2) AS mean_ms,
  rows,
  round((${time_col} * 100.0 / nullif(sum(${time_col}) OVER (), 0))::numeric, 1) AS pct,
  left(regexp_replace(query, E'[\\n\\r]+', ' ', 'g'), 120) AS query
FROM pg_stat_statements
WHERE calls >= $min_calls
  AND dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
ORDER BY $order_col DESC
LIMIT $limit;
SQL

echo ""
echo "--- Summary ---"
run_query "
SELECT 'Total queries: ' || count(*)::text ||
       ' | Total time: ' || round(sum(${time_col})::numeric / 1000 / 1000, 2)::text || 's' ||
       ' | Total calls: ' || sum(calls)::text
FROM pg_stat_statements
WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
"

echo ""
echo "Tip: Reset stats with SELECT pg_stat_statements_reset();"
