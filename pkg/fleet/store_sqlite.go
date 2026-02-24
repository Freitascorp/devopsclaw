// Package fleet — SQLite-backed durable store for fleet state.
//
// SQLiteStore provides persistent storage for fleet node registrations,
// execution audit logs, and distributed locks. It's suitable for
// single-node production deployments.
//
// For multi-node HA deployments, use PostgresStore instead.
package fleet

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver (no CGo)
)

// SQLiteStore implements the fleet Store interface with SQLite persistence.
type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex // protects lock map for in-process distributed locking
	locks map[string]*sqliteLock
}

// NewSQLiteStore creates a new SQLite-backed fleet store.
// The dbPath is the path to the SQLite database file (e.g., "/var/lib/devopsclaw/fleet.db").
// Use ":memory:" for an in-memory database (testing).
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	store := &SQLiteStore{
		db:    db,
		locks: make(map[string]*sqliteLock),
	}

	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return store, nil
}

// migrate creates or updates the database schema.
func (s *SQLiteStore) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			hostname TEXT NOT NULL DEFAULT '',
			address TEXT NOT NULL DEFAULT '',
			labels TEXT NOT NULL DEFAULT '{}',
			groups_list TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'offline',
			capabilities TEXT NOT NULL DEFAULT '[]',
			resources TEXT NOT NULL DEFAULT '{}',
			last_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			registered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			version TEXT NOT NULL DEFAULT '',
			tunnel_id TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS executions (
			id TEXT PRIMARY KEY,
			request TEXT NOT NULL,
			result TEXT NOT NULL,
			requester TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_requester ON executions(requester)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_created ON executions(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status)`,
		`CREATE TABLE IF NOT EXISTS locks (
			key TEXT PRIMARY KEY,
			holder TEXT NOT NULL,
			expires_at DATETIME NOT NULL
		)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}
	return nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ------------------------------------------------------------------
// Node management
// ------------------------------------------------------------------

func (s *SQLiteStore) RegisterNode(_ context.Context, node *Node) error {
	labelsJSON, _ := json.Marshal(node.Labels)
	groupsJSON, _ := json.Marshal(node.Groups)
	capsJSON, _ := json.Marshal(node.Capabilities)
	resJSON, _ := json.Marshal(node.Resources)

	_, err := s.db.Exec(`
		INSERT INTO nodes (id, hostname, address, labels, groups_list, status, capabilities, resources, last_seen, registered_at, version, tunnel_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			hostname=excluded.hostname, address=excluded.address, labels=excluded.labels,
			groups_list=excluded.groups_list, status=excluded.status, capabilities=excluded.capabilities,
			resources=excluded.resources, last_seen=excluded.last_seen, version=excluded.version,
			tunnel_id=excluded.tunnel_id
	`, node.ID, node.Hostname, node.Address, string(labelsJSON), string(groupsJSON),
		string(node.Status), string(capsJSON), string(resJSON),
		node.LastSeen.UTC(), node.RegisteredAt.UTC(), node.Version, node.TunnelID)

	return err
}

func (s *SQLiteStore) DeregisterNode(_ context.Context, id NodeID) error {
	res, err := s.db.Exec("DELETE FROM nodes WHERE id = ?", string(id))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("node %s not found", id)
	}
	return nil
}

func (s *SQLiteStore) UpdateNodeStatus(_ context.Context, id NodeID, status NodeStatus) error {
	res, err := s.db.Exec("UPDATE nodes SET status = ? WHERE id = ?", string(status), string(id))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("node %s not found", id)
	}
	return nil
}

func (s *SQLiteStore) UpdateNodeHeartbeat(_ context.Context, id NodeID, resources NodeResources) error {
	resJSON, _ := json.Marshal(resources)
	res, err := s.db.Exec("UPDATE nodes SET last_seen = ?, resources = ? WHERE id = ?",
		time.Now().UTC(), string(resJSON), string(id))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("node %s not found", id)
	}
	return nil
}

func (s *SQLiteStore) GetNode(_ context.Context, id NodeID) (*Node, error) {
	row := s.db.QueryRow(`SELECT id, hostname, address, labels, groups_list, status, capabilities, resources, last_seen, registered_at, version, tunnel_id FROM nodes WHERE id = ?`, string(id))
	return scanNode(row)
}

func (s *SQLiteStore) ListNodes(_ context.Context) ([]*Node, error) {
	rows, err := s.db.Query(`SELECT id, hostname, address, labels, groups_list, status, capabilities, resources, last_seen, registered_at, version, tunnel_id FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

func (s *SQLiteStore) ListNodesByGroup(_ context.Context, group GroupName) ([]*Node, error) {
	// JSON array search: groups_list contains the group name
	rows, err := s.db.Query(`SELECT id, hostname, address, labels, groups_list, status, capabilities, resources, last_seen, registered_at, version, tunnel_id FROM nodes WHERE groups_list LIKE ?`,
		fmt.Sprintf("%%%s%%", string(group)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes, err := scanNodes(rows)
	if err != nil {
		return nil, err
	}

	// Filter precisely (the LIKE is an approximation)
	var filtered []*Node
	for _, n := range nodes {
		for _, g := range n.Groups {
			if g == group {
				filtered = append(filtered, n)
				break
			}
		}
	}
	return filtered, nil
}

func (s *SQLiteStore) ListNodesByLabels(_ context.Context, labels map[string]string) ([]*Node, error) {
	// Fetch all and filter in Go (label matching is complex for SQL)
	rows, err := s.db.Query(`SELECT id, hostname, address, labels, groups_list, status, capabilities, resources, last_seen, registered_at, version, tunnel_id FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes, err := scanNodes(rows)
	if err != nil {
		return nil, err
	}

	var filtered []*Node
	for _, n := range nodes {
		if matchLabels(n.Labels, labels) {
			filtered = append(filtered, n)
		}
	}
	return filtered, nil
}

// ------------------------------------------------------------------
// Execution audit
// ------------------------------------------------------------------

func (s *SQLiteStore) RecordExecution(_ context.Context, req *ExecRequest, result *ExecResult) error {
	reqJSON, _ := json.Marshal(req)
	resJSON, _ := json.Marshal(result)
	_, err := s.db.Exec(`INSERT INTO executions (id, request, result, requester, created_at) VALUES (?, ?, ?, ?, ?)`,
		req.ID, string(reqJSON), string(resJSON), req.Requester, req.CreatedAt.UTC())
	return err
}

func (s *SQLiteStore) GetExecution(_ context.Context, id string) (*ExecRequest, *ExecResult, error) {
	var reqJSON, resJSON string
	err := s.db.QueryRow("SELECT request, result FROM executions WHERE id = ?", id).Scan(&reqJSON, &resJSON)
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

func (s *SQLiteStore) ListExecutions(_ context.Context, opts ListExecOptions) ([]*ExecRequest, error) {
	query := "SELECT request FROM executions WHERE 1=1"
	var args []any

	if opts.Requester != "" {
		query += " AND requester = ?"
		args = append(args, opts.Requester)
	}
	if !opts.Since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, opts.Since.UTC())
	}

	query += " ORDER BY created_at DESC"

	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}
	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := s.db.Query(query, args...)
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
// Distributed locking (process-level for SQLite)
// ------------------------------------------------------------------

func (s *SQLiteStore) AcquireLock(_ context.Context, key string, ttl time.Duration) (Lock, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean expired locks
	now := time.Now()
	if existing, ok := s.locks[key]; ok {
		if existing.expiresAt.After(now) {
			return nil, fmt.Errorf("lock %s is held until %s", key, existing.expiresAt.Format(time.RFC3339))
		}
		delete(s.locks, key)
	}

	// Also try to clean up in the database
	s.db.Exec("DELETE FROM locks WHERE key = ? AND expires_at < ?", key, now.UTC())

	// Try to acquire
	expiresAt := now.Add(ttl)
	_, err := s.db.Exec(`INSERT INTO locks (key, holder, expires_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET holder=excluded.holder, expires_at=excluded.expires_at
		WHERE locks.expires_at < ?`,
		key, "self", expiresAt.UTC(), now.UTC())
	if err != nil {
		return nil, fmt.Errorf("acquire lock %s: %w", key, err)
	}

	lock := &sqliteLock{
		store:     s,
		key:       key,
		expiresAt: expiresAt,
	}
	s.locks[key] = lock
	return lock, nil
}

type sqliteLock struct {
	store     *SQLiteStore
	key       string
	expiresAt time.Time
}

func (l *sqliteLock) Unlock(_ context.Context) error {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	delete(l.store.locks, l.key)
	_, err := l.store.db.Exec("DELETE FROM locks WHERE key = ?", l.key)
	return err
}

func (l *sqliteLock) Extend(_ context.Context, ttl time.Duration) error {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	l.expiresAt = time.Now().Add(ttl)
	_, err := l.store.db.Exec("UPDATE locks SET expires_at = ? WHERE key = ?", l.expiresAt.UTC(), l.key)
	return err
}

// ------------------------------------------------------------------
// Scan helpers
// ------------------------------------------------------------------

type scanner interface {
	Scan(dest ...any) error
}

func scanNode(row scanner) (*Node, error) {
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

func scanNodes(rows *sql.Rows) ([]*Node, error) {
	var nodes []*Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}
