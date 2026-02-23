---
name: postgres
description: "Manage PostgreSQL databases. Queries, backups, replication, performance tuning, user management, and troubleshooting via psql and pg_dump."
metadata: {"nanobot":{"emoji":"🐘","requires":{"bins":["psql"]},"install":[{"id":"brew","kind":"brew","formula":"postgresql","bins":["psql"],"label":"Install PostgreSQL client (brew)"}]}}
---

# PostgreSQL Skill

Use `psql` for interactive queries and administration, `pg_dump` for backups, and system views for monitoring.

## Connection

```bash
# Connect
psql -h localhost -U postgres -d mydb
psql "postgresql://user:pass@host:5432/mydb?sslmode=require"

# Set connection via env vars
export PGHOST=localhost PGUSER=postgres PGDATABASE=mydb PGPASSWORD=secret

# Run a single query
psql -c "SELECT version();"

# Run a SQL file
psql -f schema.sql

# List databases
psql -l
```

## Database Management

```sql
-- Create database
CREATE DATABASE myapp OWNER myuser;

-- Drop database
DROP DATABASE IF EXISTS myapp;

-- List databases
\l

-- Connect to a database
\c mydb

-- List tables
\dt
\dt+  -- with sizes

-- Describe a table
\d users
\d+ users  -- with storage info

-- List indexes
\di

-- Table sizes
SELECT schemaname, tablename,
  pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS total_size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

## User Management

```sql
-- Create user
CREATE USER deploy WITH PASSWORD 's3cret' LOGIN;

-- Grant privileges
GRANT CONNECT ON DATABASE mydb TO deploy;
GRANT USAGE ON SCHEMA public TO deploy;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO deploy;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO deploy;

-- Create read-write role
CREATE ROLE readwrite;
GRANT CONNECT ON DATABASE mydb TO readwrite;
GRANT USAGE, CREATE ON SCHEMA public TO readwrite;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO readwrite;
GRANT readwrite TO myuser;

-- List users and roles
\du

-- Change password
ALTER USER myuser WITH PASSWORD 'new-password';
```

## Backup & Restore

```bash
# Dump a database (custom format — best for restore)
pg_dump -Fc -h localhost -U postgres mydb > mydb.dump

# Dump as SQL
pg_dump -h localhost -U postgres mydb > mydb.sql

# Dump specific tables
pg_dump -h localhost -U postgres -t users -t orders mydb > tables.sql

# Dump schema only
pg_dump -s -h localhost -U postgres mydb > schema.sql

# Dump data only
pg_dump -a -h localhost -U postgres mydb > data.sql

# Restore from custom format
pg_restore -h localhost -U postgres -d mydb --clean --if-exists mydb.dump

# Restore from SQL
psql -h localhost -U postgres -d mydb < mydb.sql

# Dump all databases
pg_dumpall -h localhost -U postgres > all_databases.sql
```

## Performance & Monitoring

```sql
-- Active queries
SELECT pid, now() - pg_stat_activity.query_start AS duration, query, state
FROM pg_stat_activity
WHERE state != 'idle' AND query NOT LIKE '%pg_stat_activity%'
ORDER BY duration DESC;

-- Long-running queries (>5 seconds)
SELECT pid, now() - pg_stat_activity.query_start AS duration, query
FROM pg_stat_activity
WHERE state = 'active' AND now() - pg_stat_activity.query_start > interval '5 seconds';

-- Kill a query
SELECT pg_cancel_backend(PID);     -- graceful
SELECT pg_terminate_backend(PID);  -- force

-- Table statistics
SELECT relname, seq_scan, idx_scan, n_live_tup, n_dead_tup,
  round(100.0 * idx_scan / NULLIF(seq_scan + idx_scan, 0), 1) AS idx_hit_rate
FROM pg_stat_user_tables
ORDER BY n_live_tup DESC;

-- Index usage
SELECT indexrelname, idx_scan, pg_size_pretty(pg_relation_size(indexrelid)) AS size
FROM pg_stat_user_indexes
ORDER BY idx_scan;

-- Database size
SELECT pg_size_pretty(pg_database_size('mydb'));

-- Connection count
SELECT count(*) FROM pg_stat_activity;
SELECT datname, count(*) FROM pg_stat_activity GROUP BY datname;

-- Cache hit ratio (should be >99%)
SELECT sum(heap_blks_hit) / NULLIF(sum(heap_blks_hit) + sum(heap_blks_read), 0) AS ratio
FROM pg_statio_user_tables;

-- Vacuum stats
SELECT relname, last_vacuum, last_autovacuum, n_dead_tup
FROM pg_stat_user_tables
ORDER BY n_dead_tup DESC;

-- Explain a query
EXPLAIN ANALYZE SELECT * FROM users WHERE email = 'test@example.com';
```

## Replication

```sql
-- Check replication status (on primary)
SELECT client_addr, state, sent_lsn, replay_lsn,
  sent_lsn - replay_lsn AS replication_lag
FROM pg_stat_replication;

-- Check if this is primary or replica
SELECT pg_is_in_recovery();  -- false = primary, true = replica

-- Replication lag (on replica)
SELECT now() - pg_last_xact_replay_timestamp() AS replication_lag;
```

## Maintenance

```bash
# Vacuum (reclaim dead rows)
vacuumdb -h localhost -U postgres -d mydb --analyze

# Reindex
reindexdb -h localhost -U postgres -d mydb

# Check for bloat
psql -c "SELECT schemaname, tablename,
  pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as total,
  round(100.0 * n_dead_tup / NULLIF(n_live_tup + n_dead_tup, 0), 1) as dead_pct
FROM pg_stat_user_tables
ORDER BY n_dead_tup DESC LIMIT 10;"
```

## Tips

- Use `\x` in psql for vertical (expanded) output on wide tables.
- Use `\timing` in psql to show query execution time.
- Use `EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)` for detailed query plans.
- Use `pg_dump -Fc` (custom format) — it supports parallel restore with `-j`.
- Monitor `n_dead_tup` — high values mean autovacuum may need tuning.
- Use connection pooling (PgBouncer) in production.

## Bundled Scripts

- Health check: `{baseDir}/scripts/pg-health.sh -H host -d dbname` (connections, replication, long queries, cache ratio)
- Slow query analysis: `{baseDir}/scripts/pg-slow-queries.sh -d dbname` (requires pg_stat_statements)
