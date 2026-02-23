#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: redis-health.sh [options]

Check Redis health: memory, connections, replication, keyspace,
slow log, and persistence status.

Options:
  -H, --host       Redis host (default: 127.0.0.1)
  -p, --port       Redis port (default: 6379)
  -a, --auth       Redis password (or use REDIS_PASSWORD env var)
  -n, --db         Database number (default: 0)
  -u, --url        Redis URL (overrides host/port/auth)
  -m, --mem-warn   Memory usage % to warn (default: 80)
  -h, --help       Show this help

Environment:
  REDIS_PASSWORD    Redis password
  REDIS_URL         Redis connection URL
USAGE
}

host="127.0.0.1"
port=6379
auth="${REDIS_PASSWORD:-}"
db=0
url="${REDIS_URL:-}"
mem_warn=80

while [[ $# -gt 0 ]]; do
  case "$1" in
    -H|--host)     host="${2-}"; shift 2 ;;
    -p|--port)     port="${2-}"; shift 2 ;;
    -a|--auth)     auth="${2-}"; shift 2 ;;
    -n|--db)       db="${2-}"; shift 2 ;;
    -u|--url)      url="${2-}"; shift 2 ;;
    -m|--mem-warn) mem_warn="${2-}"; shift 2 ;;
    -h|--help)     usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if ! command -v redis-cli >/dev/null 2>&1; then
  echo "redis-cli not found in PATH" >&2
  exit 1
fi

redis_cmd=(redis-cli)
if [[ -n "$url" ]]; then
  redis_cmd=(redis-cli -u "$url")
else
  redis_cmd=(redis-cli -h "$host" -p "$port" -n "$db")
  if [[ -n "$auth" ]]; then
    redis_cmd+=(-a "$auth" --no-auth-warning)
  fi
fi

rcli() {
  "${redis_cmd[@]}" "$@" 2>/dev/null
}

get_info() {
  rcli INFO "$1" 2>/dev/null
}

get_field() {
  echo "$1" | grep "^$2:" | cut -d: -f2 | tr -d '[:space:]'
}

# Test connection
if ! rcli PING | grep -q "PONG"; then
  echo "✗ Cannot connect to Redis at $host:$port" >&2
  exit 1
fi

issues=0
echo "=== Redis Health Check ==="
echo ""

# Server info
server_info=$(get_info server)
redis_version=$(get_field "$server_info" "redis_version")
uptime_days=$(get_field "$server_info" "uptime_in_days")
echo "Version: $redis_version | Uptime: ${uptime_days} days"
echo ""

# Memory
echo "--- Memory ---"
memory_info=$(get_info memory)
used_memory_human=$(get_field "$memory_info" "used_memory_human")
used_memory=$(get_field "$memory_info" "used_memory")
maxmemory=$(get_field "$memory_info" "maxmemory")
maxmemory_human=$(get_field "$memory_info" "maxmemory_human")
maxmemory_policy=$(get_field "$memory_info" "maxmemory_policy")
mem_frag=$(get_field "$memory_info" "mem_fragmentation_ratio")

echo "Used: $used_memory_human | Max: ${maxmemory_human:-unlimited} | Policy: ${maxmemory_policy:-noeviction}"
echo "Fragmentation ratio: $mem_frag"

if [[ "${maxmemory:-0}" != "0" ]]; then
  mem_pct=$((used_memory * 100 / maxmemory))
  echo "Usage: ${mem_pct}%"
  if [[ "$mem_pct" -ge "$mem_warn" ]]; then
    echo "⚠ Memory usage above ${mem_warn}% threshold"
    issues=1
  fi
fi

# Check fragmentation
frag_int=${mem_frag%.*}
if [[ "${frag_int:-1}" -gt 2 ]]; then
  echo "⚠ High memory fragmentation (${mem_frag}) — consider restart"
  issues=1
fi
echo ""

# Clients
echo "--- Connections ---"
clients_info=$(get_info clients)
connected_clients=$(get_field "$clients_info" "connected_clients")
blocked_clients=$(get_field "$clients_info" "blocked_clients")
rejected_connections=$(get_field "$clients_info" "rejected_connections" 2>/dev/null || echo "0")
echo "Connected: $connected_clients | Blocked: $blocked_clients | Rejected: ${rejected_connections:-0}"

if [[ "${blocked_clients:-0}" -gt 0 ]]; then
  echo "⚠ $blocked_clients blocked clients"
  issues=1
fi
echo ""

# Keyspace
echo "--- Keyspace ---"
keyspace_info=$(get_info keyspace)
if echo "$keyspace_info" | grep -q "^db"; then
  echo "$keyspace_info" | grep "^db" | while IFS=: read -r db_name details; do
    keys=$(echo "$details" | grep -o 'keys=[0-9]*' | cut -d= -f2)
    expires=$(echo "$details" | grep -o 'expires=[0-9]*' | cut -d= -f2)
    echo "  $db_name: ${keys} keys (${expires} with TTL)"
  done
else
  echo "  (empty)"
fi
echo ""

# Replication
echo "--- Replication ---"
repl_info=$(get_info replication)
role=$(get_field "$repl_info" "role")
echo "Role: $role"

if [[ "$role" == "master" ]]; then
  connected_slaves=$(get_field "$repl_info" "connected_slaves")
  echo "Connected replicas: $connected_slaves"
  echo "$repl_info" | grep "^slave[0-9]" | while IFS=: read -r _ details; do
    ip=$(echo "$details" | grep -o 'ip=[^,]*' | cut -d= -f2)
    state=$(echo "$details" | grep -o 'state=[^,]*' | cut -d= -f2)
    lag=$(echo "$details" | grep -o 'lag=[^,]*' | cut -d= -f2)
    echo "  $ip: $state (lag: ${lag}s)"
  done
elif [[ "$role" == "slave" ]]; then
  master_host=$(get_field "$repl_info" "master_host")
  master_link=$(get_field "$repl_info" "master_link_status")
  echo "Master: $master_host | Link: $master_link"
  if [[ "$master_link" != "up" ]]; then
    echo "⚠ Replication link is down"
    issues=1
  fi
fi
echo ""

# Persistence
echo "--- Persistence ---"
persist_info=$(get_info persistence)
rdb_status=$(get_field "$persist_info" "rdb_last_bgsave_status")
aof_enabled=$(get_field "$persist_info" "aof_enabled")
echo "RDB last save: $rdb_status | AOF enabled: $aof_enabled"

if [[ "$rdb_status" != "ok" ]]; then
  echo "⚠ Last RDB save failed"
  issues=1
fi

if [[ "$aof_enabled" == "1" ]]; then
  aof_status=$(get_field "$persist_info" "aof_last_bgrewrite_status")
  echo "AOF last rewrite: $aof_status"
fi
echo ""

# Slow log
echo "--- Slow Log (last 5) ---"
slowlog=$(rcli SLOWLOG GET 5 2>/dev/null || true)
if [[ -n "$slowlog" && "$slowlog" != "(empty"* ]]; then
  echo "$slowlog"
else
  echo "  (none)"
fi
echo ""

# Summary
if [[ "$issues" -eq 0 ]]; then
  echo "✓ Redis looks healthy"
  exit 0
else
  echo "✗ Issues detected — review above"
  exit 1
fi
