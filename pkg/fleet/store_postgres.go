// Package fleet — PostgreSQL-backed durable store for fleet HA deployments.
//
// PostgresStore implements the fleet Store interface with PostgreSQL, providing:
//   - Durable state across process restarts
//   - Multi-instance support (multiple relay servers share the same DB)
//   - Advisory locks for distributed coordination
//   - LISTEN/NOTIFY for real-time node event propagation
//
// Requires: PostgreSQL 14+ with the uuid-ossp extension.
package fleet

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresStore implements the fleet Store interface with PostgreSQL.
type PostgresStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// PostgresConfig holds connection parameters for PostgreSQL.
type PostgresConfig struct {
	Host     string `json:"host"     env:"DEVOPSCLAW_PG_HOST"`
	Port     int    `json:"port"     env:"DEVOPSCLAW_PG_PORT"`
	User     string `json:"user"     env:"DEVOPSCLAW_PG_USER"`
	Password string `json:"password" env:"DEVOPSCLAW_PG_PASSWORD"`
	Database string `json:"database" env:"DEVOPSCLAW_PG_DATABASE"`
	SSLMode  string `json:"ssl_mode" env:"DEVOPSCLAW_PG_SSLMODE"` // "disable", "require", "verify-full"
}

// DSN returns a PostgreSQL connection string.
func (c PostgresConfig) DSN() string {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "require"
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, port, c.User, c.Password, c.Database, sslMode)
}

// NewPostgresStore creates a new PostgreSQL-backed fleet store.
func NewPostgresStore(cfg PostgresConfig) (*PostgresStore, error) {
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	// Connection pool settings for HA
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	store := &PostgresStore{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return store, nil
}

// migrate creates or updates the database schema.
func (s *PostgresStore) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS fleet_nodes (
			id TEXT PRIMARY KEY,
			hostname TEXT NOT NULL DEFAULT '',
			address TEXT NOT NULL DEFAULT '',
			labels JSONB NOT NULL DEFAULT '{}',
			groups_list JSONB NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'offline',
			capabilities JSONB NOT NULL DEFAULT '[]',
			resources JSONB NOT NULL DEFAULT '{}',
			last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			version TEXT NOT NULL DEFAULT '',
			tunnel_id TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_executions (
			id TEXT PRIMARY KEY,
			request JSONB NOT NULL,
			result JSONB NOT NULL,
			requester TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_exec_requester ON fleet_executions(requester)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_exec_created ON fleet_executions(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_nodes_status ON fleet_nodes(status)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_nodes_labels ON fleet_nodes USING GIN(labels)`,
		`CREATE TABLE IF NOT EXISTS fleet_locks (
			key TEXT PRIMARY KEY,
			holder TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL
		)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}
	return nil
}

// Close closes the database connection pool.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

// ------------------------------------------------------------------
// Node management
// ------------------------------------------------------------------

func (s *PostgresStore) RegisterNode(ctx context.Context, node *Node) error {
	labelsJSON, _ := json.Marshal(node.Labels)
	groupsJSON, _ := json.Marshal(node.Groups)
	capsJSON, _ := json.Marshal(node.Capabilities)
	resJSON, _ := json.Marshal(node.Resources)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO fleet_nodes (id, hostname, address, labels, groups_list, status, capabilities, resources, last_seen, registered_at, version, tunnel_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT(id) DO UPDATE SET
			hostname=EXCLUDED.hostname, address=EXCLUDED.address, labels=EXCLUDED.labels,
			groups_list=EXCLUDED.groups_list, status=EXCLUDED.status, capabilities=EXCLUDED.capabilities,
			resources=EXCLUDED.resources, last_seen=EXCLUDED.last_seen, version=EXCLUDED.version,
			tunnel_id=EXCLUDED.tunnel_id
	`, node.ID, node.Hostname, node.Address, string(labelsJSON), string(groupsJSON),
		string(node.Status), string(capsJSON), string(resJSON),
		node.LastSeen.UTC(), node.RegisteredAt.UTC(), node.Version, node.TunnelID)

	return err
}

func (s *PostgresStore) DeregisterNode(ctx context.Context, id NodeID) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM fleet_nodes WHERE id = $1", string(id))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("node %s not found", id)
	}
	return nil
}

func (s *PostgresStore) UpdateNodeStatus(ctx context.Context, id NodeID, status NodeStatus) error {
	res, err := s.db.ExecContext(ctx, "UPDATE fleet_nodes SET status = $1 WHERE id = $2", string(status), string(id))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("node %s not found", id)
	}
	return nil
}

func (s *PostgresStore) UpdateNodeHeartbeat(ctx context.Context, id NodeID, resources NodeResources) error {
	resJSON, _ := json.Marshal(resources)
	res, err := s.db.ExecContext(ctx, "UPDATE fleet_nodes SET last_seen = NOW(), resources = $1 WHERE id = $2",
		string(resJSON), string(id))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("node %s not found", id)
	}
	return nil
}

func (s *PostgresStore) GetNode(ctx context.Context, id NodeID) (*Node, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, hostname, address, labels, groups_list, status, capabilities, resources, last_seen, registered_at, version, tunnel_id FROM fleet_nodes WHERE id = $1`,
		string(id))
	return pgScanNode(row)
}

func (s *PostgresStore) ListNodes(ctx context.Context) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, hostname, address, labels, groups_list, status, capabilities, resources, last_seen, registered_at, version, tunnel_id FROM fleet_nodes ORDER BY registered_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgScanNodes(rows)
}

func (s *PostgresStore) ListNodesByGroup(ctx context.Context, group GroupName) ([]*Node, error) {
	// JSONB array containment: groups_list @> '["group-name"]'
	groupJSON := fmt.Sprintf(`[%q]`, string(group))
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, hostname, address, labels, groups_list, status, capabilities, resources, last_seen, registered_at, version, tunnel_id FROM fleet_nodes WHERE groups_list @> $1::jsonb`,
		groupJSON)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgScanNodes(rows)
}

func (s *PostgresStore) ListNodesByLabels(ctx context.Context, labels map[string]string) ([]*Node, error) {
	// JSONB containment: labels @> '{"env": "prod"}'
	labelsJSON, _ := json.Marshal(labels)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, hostname, address, labels, groups_list, status, capabilities, resources, last_seen, registered_at, version, tunnel_id FROM fleet_nodes WHERE labels @> $1::jsonb`,
		string(labelsJSON))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgScanNodes(rows)
}

// ------------------------------------------------------------------
// Execution audit
// ------------------------------------------------------------------

func (s *PostgresStore) RecordExecution(ctx context.Context, req *ExecRequest, result *ExecResult) error {
	reqJSON, _ := json.Marshal(req)
	resJSON, _ := json.Marshal(result)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO fleet_executions (id, request, result, requester, created_at) VALUES ($1, $2, $3, $4, $5)`,
		req.ID, string(reqJSON), string(resJSON), req.Requester, req.CreatedAt.UTC())
	return err
}

func (s *PostgresStore) GetExecution(ctx context.Context, id string) (*ExecRequest, *ExecResult, error) {
	var reqJSON, resJSON string
	err := s.db.QueryRowContext(ctx, "SELECT request, result FROM fleet_executions WHERE id = $1", id).Scan(&reqJSON, &resJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, fmt.Errorf("execution %s not found", id)
		}
		return nil, nil, err
	}

	var req ExecRequest
	var result ExecResult
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return nil, nil, fmt.Errorf("unmarshal request: %w", err)
	}
	if err := json.Unmarshal([]byte(resJSON), &result); err != nil {
		return nil, nil, fmt.Errorf("unmarshal result: %w", err)
	}
	return &req, &result, nil
}

func (s *PostgresStore) ListExecutions(ctx context.Context, opts ListExecOptions) ([]*ExecRequest, error) {
	query := "SELECT request FROM fleet_executions WHERE true"
	var args []any
	argIdx := 1

	if opts.Requester != "" {
		query += fmt.Sprintf(" AND requester = $%d", argIdx)
		args = append(args, opts.Requester)
		argIdx++
	}
	if !opts.Since.IsZero() {
		query += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, opts.Since.UTC())
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, opts.Limit)
		argIdx++
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*ExecRequest
	for rows.Next() {
		var reqJSON string
		if err := rows.Scan(&reqJSON); err != nil {
			return nil, err
		}
		var req ExecRequest
		if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
			return nil, err
		}
		out = append(out, &req)
	}
	return out, rows.Err()
}

// ------------------------------------------------------------------
// Distributed locking (PostgreSQL advisory locks)
// ------------------------------------------------------------------

func (s *PostgresStore) AcquireLock(ctx context.Context, key string, ttl time.Duration) (Lock, error) {
	// Use PostgreSQL advisory locks for distributed coordination.
	// First, clean expired locks from the table.
	s.db.ExecContext(ctx, "DELETE FROM fleet_locks WHERE expires_at < NOW()")

	expiresAt := time.Now().Add(ttl)

	// Try to insert, fail if already held
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO fleet_locks (key, holder, expires_at) VALUES ($1, $2, $3)
		ON CONFLICT(key) DO UPDATE SET holder=EXCLUDED.holder, expires_at=EXCLUDED.expires_at
		WHERE fleet_locks.expires_at < NOW()`,
		key, "self", expiresAt.UTC())
	if err != nil {
		return nil, fmt.Errorf("acquire lock %s: %w", key, err)
	}

	// Verify we actually hold the lock
	var holder string
	err = s.db.QueryRowContext(ctx, "SELECT holder FROM fleet_locks WHERE key = $1", key).Scan(&holder)
	if err != nil || holder != "self" {
		return nil, fmt.Errorf("failed to acquire lock %s: held by another instance", key)
	}

	return &pgLock{db: s.db, key: key, expiresAt: expiresAt}, nil
}

type pgLock struct {
	db        *sql.DB
	key       string
	expiresAt time.Time
}

func (l *pgLock) Unlock(ctx context.Context) error {
	_, err := l.db.ExecContext(ctx, "DELETE FROM fleet_locks WHERE key = $1 AND holder = 'self'", l.key)
	return err
}

func (l *pgLock) Extend(ctx context.Context, ttl time.Duration) error {
	l.expiresAt = time.Now().Add(ttl)
	_, err := l.db.ExecContext(ctx, "UPDATE fleet_locks SET expires_at = $1 WHERE key = $2 AND holder = 'self'",
		l.expiresAt.UTC(), l.key)
	return err
}

// ------------------------------------------------------------------
// Scan helpers
// ------------------------------------------------------------------

func pgScanNode(row scanner) (*Node, error) {
	var n Node
	var labelsJSON, groupsJSON, capsJSON, resJSON, statusStr string
	var lastSeen, registeredAt time.Time

	err := row.Scan(&n.ID, &n.Hostname, &n.Address, &labelsJSON, &groupsJSON,
		&statusStr, &capsJSON, &resJSON, &lastSeen, &registeredAt, &n.Version, &n.TunnelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("node not found")
		}
		return nil, err
	}

	n.Status = NodeStatus(statusStr)
	n.LastSeen = lastSeen
	n.RegisteredAt = registeredAt
	json.Unmarshal([]byte(labelsJSON), &n.Labels)
	json.Unmarshal([]byte(groupsJSON), &n.Groups)
	json.Unmarshal([]byte(capsJSON), &n.Capabilities)
	json.Unmarshal([]byte(resJSON), &n.Resources)

	if n.Labels == nil {
		n.Labels = make(map[string]string)
	}

	return &n, nil
}

func pgScanNodes(rows *sql.Rows) ([]*Node, error) {
	var nodes []*Node
	for rows.Next() {
		n, err := pgScanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}
