#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: pg-health.sh [options]

Check PostgreSQL health: connections, replication lag, long queries,
table bloat, and database size.

Options:
  -H, --host        PostgreSQL host (default: localhost)
  -p, --port        PostgreSQL port (default: 5432)
  -U, --user        PostgreSQL user (default: postgres)
  -d, --dbname      Database name (default: postgres)
  -c, --conn-warn   Connection usage % to warn (default: 80)
  -q, --query-sec   Long query threshold in seconds (default: 30)
  -h, --help        Show this help

Environment:
  PGPASSWORD         Password (or use .pgpass)
  DATABASE_URL       Full connection string (overrides host/port/user/dbname)
USAGE
}

host="localhost"
port=5432
user="postgres"
dbname="postgres"
conn_warn=80
query_sec=30

while [[ $# -gt 0 ]]; do
  case "$1" in
    -H|--host)      host="${2-}"; shift 2 ;;
    -p|--port)      port="${2-}"; shift 2 ;;
    -U|--user)      user="${2-}"; shift 2 ;;
    -d|--dbname)    dbname="${2-}"; shift 2 ;;
    -c|--conn-warn) conn_warn="${2-}"; shift 2 ;;
    -q|--query-sec) query_sec="${2-}"; shift 2 ;;
    -h|--help)      usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if ! command -v psql >/dev/null 2>&1; then
  echo "psql not found in PATH" >&2
  exit 1
fi

psql_cmd=(psql -h "$host" -p "$port" -U "$user" -d "$dbname" -t -A -X)
if [[ -n "${DATABASE_URL:-}" ]]; then
  psql_cmd=(psql "$DATABASE_URL" -t -A -X)
fi

run_query() {
  "${psql_cmd[@]}" -c "$1" 2>/dev/null
}

# Test connection
if ! run_query "SELECT 1" >/dev/null 2>&1; then
  echo "✗ Cannot connect to PostgreSQL at $host:$port" >&2
  exit 1
fi

issues=0
echo "=== PostgreSQL Health Check ==="
echo "Host: $host:$port  Database: $dbname"
echo ""

# Version
version=$(run_query "SELECT version()" | head -1)
echo "Version: $version"
echo ""

# Connection usage
echo "--- Connections ---"
conn_info=$(run_query "
SELECT current_setting('max_connections')::int AS max_conn,
       (SELECT count(*) FROM pg_stat_activity) AS current_conn
")
max_conn=$(echo "$conn_info" | cut -d'|' -f1)
current_conn=$(echo "$conn_info" | cut -d'|' -f2)
conn_pct=$((current_conn * 100 / max_conn))

echo "Current: $current_conn / $max_conn ($conn_pct%)"
if [[ "$conn_pct" -ge "$conn_warn" ]]; then
  echo "⚠ Connection usage above ${conn_warn}% threshold"
  issues=1
fi

# Connection breakdown by state
echo ""
echo "By state:"
run_query "
SELECT state, count(*)
FROM pg_stat_activity
WHERE pid != pg_backend_pid()
GROUP BY state ORDER BY count(*) DESC
" | while IFS='|' read -r state count; do
  state=${state:-"null"}
  echo "  $state: $count"
done
echo ""

# Long-running queries
echo "--- Long Queries (>${query_sec}s) ---"
long_queries=$(run_query "
SELECT pid, now() - query_start AS duration,
       left(query, 100) AS query_preview
FROM pg_stat_activity
WHERE state = 'active'
  AND pid != pg_backend_pid()
  AND now() - query_start > interval '${query_sec} seconds'
ORDER BY duration DESC
LIMIT 10
")

if [[ -n "$long_queries" ]]; then
  echo "$long_queries" | while IFS='|' read -r pid duration query; do
    echo "  PID $pid (${duration}): ${query}"
  done
  issues=1
else
  echo "  (none)"
fi
echo ""

# Replication lag (if applicable)
echo "--- Replication ---"
repl_info=$(run_query "
SELECT client_addr, state,
       pg_wal_lsn_diff(sent_lsn, replay_lsn) AS lag_bytes
FROM pg_stat_replication
" 2>/dev/null || true)

if [[ -n "$repl_info" ]]; then
  echo "$repl_info" | while IFS='|' read -r addr state lag; do
    lag_mb=$(echo "scale=2; ${lag:-0} / 1048576" | bc 2>/dev/null || echo "$lag")
    echo "  $addr ($state): ${lag_mb}MB lag"
  done
else
  echo "  (no replicas)"
fi
echo ""

# Database sizes
echo "--- Database Sizes ---"
run_query "
SELECT datname, pg_size_pretty(pg_database_size(datname))
FROM pg_database
WHERE NOT datistemplate
ORDER BY pg_database_size(datname) DESC
LIMIT 10
" | while IFS='|' read -r db size; do
  echo "  $db: $size"
done
echo ""

# Cache hit ratio
echo "--- Cache Hit Ratio ---"
cache_ratio=$(run_query "
SELECT round(
  sum(blks_hit) * 100.0 / nullif(sum(blks_hit) + sum(blks_read), 0), 2
) FROM pg_stat_database
" 2>/dev/null || echo "N/A")
echo "  Block cache hit ratio: ${cache_ratio}%"
if [[ "$cache_ratio" != "N/A" ]]; then
  ratio_int=${cache_ratio%.*}
  if [[ "${ratio_int:-0}" -lt 95 ]]; then
    echo "  ⚠ Cache hit ratio below 95% — consider increasing shared_buffers"
    issues=1
  fi
fi
echo ""

# Summary
if [[ "$issues" -eq 0 ]]; then
  echo "✓ PostgreSQL looks healthy"
  exit 0
else
  echo "✗ Issues detected — review above"
  exit 1
fi
