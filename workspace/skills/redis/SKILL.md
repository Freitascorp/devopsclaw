---
name: redis
description: "Manage Redis instances using the `redis-cli`. Keys, data structures, persistence, replication, Sentinel, memory analysis, and troubleshooting."
metadata: {"nanobot":{"emoji":"🔴","requires":{"bins":["redis-cli"]},"install":[{"id":"brew","kind":"brew","formula":"redis","bins":["redis-cli"],"label":"Install Redis (brew)"}]}}
---

# Redis Skill

Use `redis-cli` to manage Redis instances — caching, sessions, queues, and pub/sub.

## Connection

```bash
# Connect
redis-cli
redis-cli -h redis.example.com -p 6379
redis-cli -h redis.example.com -p 6379 -a password
redis-cli --tls -h redis.example.com -p 6380

# Run a single command
redis-cli PING
redis-cli INFO server

# Connect to a specific database
redis-cli -n 1
```

## Key Operations

```bash
# Set / get
redis-cli SET mykey "hello"
redis-cli GET mykey

# Set with expiry (seconds)
redis-cli SET session:abc123 "data" EX 3600

# Check TTL
redis-cli TTL session:abc123

# Delete
redis-cli DEL mykey

# Check if key exists
redis-cli EXISTS mykey

# Find keys (avoid KEYS in production — use SCAN)
redis-cli SCAN 0 MATCH "session:*" COUNT 100

# Type of a key
redis-cli TYPE mykey

# Key count
redis-cli DBSIZE
```

## Server Info & Monitoring

```bash
# Full server info
redis-cli INFO

# Memory info
redis-cli INFO memory
redis-cli MEMORY USAGE mykey  # bytes used by a key

# Connected clients
redis-cli INFO clients
redis-cli CLIENT LIST

# Keyspace stats
redis-cli INFO keyspace

# Slow log
redis-cli SLOWLOG GET 10

# Real-time command monitoring (careful in prod!)
redis-cli MONITOR  # Ctrl+C to stop

# Server stats
redis-cli INFO stats | grep -E "instantaneous_ops|connected_clients|used_memory_human"
```

## Memory Analysis

```bash
# Memory overview
redis-cli INFO memory | grep -E "used_memory_human|maxmemory_human|mem_fragmentation_ratio"

# Memory usage of top keys (requires redis-cli 6.2+)
redis-cli --memkeys -i 0.01

# Big keys scan
redis-cli --bigkeys

# Memory doctor
redis-cli MEMORY DOCTOR
```

## Persistence

```bash
# Trigger RDB snapshot
redis-cli BGSAVE

# Last save time
redis-cli LASTSAVE

# Trigger AOF rewrite
redis-cli BGREWRITEAOF

# Check persistence status
redis-cli INFO persistence
```

## Replication

```bash
# Check replication status
redis-cli INFO replication

# On primary:
# role:master, connected_slaves:2

# On replica:
# role:slave, master_host, master_link_status:up
```

## Common Data Structures

```bash
# Hash (objects)
redis-cli HSET user:1001 name "Alice" email "alice@example.com" role "admin"
redis-cli HGETALL user:1001
redis-cli HGET user:1001 email

# List (queue)
redis-cli LPUSH queue:jobs '{"type":"email","to":"alice@example.com"}'
redis-cli RPOP queue:jobs
redis-cli LLEN queue:jobs

# Set
redis-cli SADD online:users "user1" "user2" "user3"
redis-cli SMEMBERS online:users
redis-cli SCARD online:users

# Sorted set (leaderboard)
redis-cli ZADD leaderboard 100 "player1" 200 "player2"
redis-cli ZREVRANGE leaderboard 0 9 WITHSCORES

# Stream (event log)
redis-cli XADD events '*' type "deploy" service "myapp" version "v2.1.0"
redis-cli XRANGE events - + COUNT 10
```

## Flush & Maintenance

```bash
# Flush current database
redis-cli FLUSHDB

# Flush all databases
redis-cli FLUSHALL

# CONFIG
redis-cli CONFIG GET maxmemory
redis-cli CONFIG SET maxmemory "256mb"
redis-cli CONFIG GET maxmemory-policy
redis-cli CONFIG SET maxmemory-policy allkeys-lru
```

## Tips

- Use `SCAN` instead of `KEYS` in production — `KEYS *` blocks the server.
- Use `--bigkeys` regularly to find memory hogs.
- Use `EX` (seconds) or `PX` (milliseconds) with `SET` to auto-expire keys.
- Monitor `mem_fragmentation_ratio` — values >1.5 indicate fragmentation issues.
- Use `maxmemory-policy allkeys-lru` for caching use cases.
- Use `redis-cli --stat` for real-time stats overview.
- Use pipelines for bulk operations: `cat commands.txt | redis-cli --pipe`.
