package fleet

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStore_CRUD(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Register a node
	node := &Node{
		ID:           "sqlite-test-1",
		Hostname:     "host1.example.com",
		Address:      "10.0.0.1:9443",
		Labels:       map[string]string{"env": "prod", "role": "web"},
		Groups:       []GroupName{"web", "prod"},
		Status:       NodeStatusOnline,
		Capabilities: []string{"shell", "docker"},
		Resources: NodeResources{
			CPUCores: 4,
			MemoryMB: 8192,
			OS:       "linux",
			Arch:     "amd64",
		},
		LastSeen:     time.Now(),
		RegisteredAt: time.Now(),
		Version:      "1.0.0",
	}

	if err := store.RegisterNode(ctx, node); err != nil {
		t.Fatalf("RegisterNode: %v", err)
	}

	// Get node
	got, err := store.GetNode(ctx, "sqlite-test-1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Hostname != "host1.example.com" {
		t.Errorf("hostname = %q, want %q", got.Hostname, "host1.example.com")
	}
	if got.Labels["env"] != "prod" {
		t.Errorf("label env = %q, want %q", got.Labels["env"], "prod")
	}
	if got.Status != NodeStatusOnline {
		t.Errorf("status = %q, want %q", got.Status, NodeStatusOnline)
	}

	// List all nodes
	nodes, err := store.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("ListNodes = %d nodes, want 1", len(nodes))
	}

	// Update status
	if err := store.UpdateNodeStatus(ctx, "sqlite-test-1", NodeStatusDegraded); err != nil {
		t.Fatalf("UpdateNodeStatus: %v", err)
	}
	got, _ = store.GetNode(ctx, "sqlite-test-1")
	if got.Status != NodeStatusDegraded {
		t.Errorf("status after update = %q, want %q", got.Status, NodeStatusDegraded)
	}

	// Update heartbeat
	newRes := NodeResources{CPUCores: 8, MemoryMB: 16384}
	if err := store.UpdateNodeHeartbeat(ctx, "sqlite-test-1", newRes); err != nil {
		t.Fatalf("UpdateNodeHeartbeat: %v", err)
	}
	got, _ = store.GetNode(ctx, "sqlite-test-1")
	if got.Resources.CPUCores != 8 {
		t.Errorf("cpu_cores after heartbeat = %d, want 8", got.Resources.CPUCores)
	}

	// Deregister
	if err := store.DeregisterNode(ctx, "sqlite-test-1"); err != nil {
		t.Fatalf("DeregisterNode: %v", err)
	}
	nodes, _ = store.ListNodes(ctx)
	if len(nodes) != 0 {
		t.Errorf("ListNodes after deregister = %d, want 0", len(nodes))
	}
}

func TestSQLiteStore_ListByGroup(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	store.RegisterNode(ctx, &Node{ID: "n1", Groups: []GroupName{"web", "prod"}, Status: NodeStatusOnline, Labels: map[string]string{}})
	store.RegisterNode(ctx, &Node{ID: "n2", Groups: []GroupName{"db", "prod"}, Status: NodeStatusOnline, Labels: map[string]string{}})
	store.RegisterNode(ctx, &Node{ID: "n3", Groups: []GroupName{"web", "staging"}, Status: NodeStatusOnline, Labels: map[string]string{}})

	nodes, err := store.ListNodesByGroup(ctx, "web")
	if err != nil {
		t.Fatalf("ListNodesByGroup: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("ListNodesByGroup(web) = %d nodes, want 2", len(nodes))
	}
}

func TestSQLiteStore_ListByLabels(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	store.RegisterNode(ctx, &Node{ID: "n1", Labels: map[string]string{"env": "prod", "tier": "frontend"}, Status: NodeStatusOnline})
	store.RegisterNode(ctx, &Node{ID: "n2", Labels: map[string]string{"env": "prod", "tier": "backend"}, Status: NodeStatusOnline})
	store.RegisterNode(ctx, &Node{ID: "n3", Labels: map[string]string{"env": "staging"}, Status: NodeStatusOnline})

	nodes, err := store.ListNodesByLabels(ctx, map[string]string{"env": "prod"})
	if err != nil {
		t.Fatalf("ListNodesByLabels: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("ListNodesByLabels(env=prod) = %d nodes, want 2", len(nodes))
	}
}

func TestSQLiteStore_Executions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	req := &ExecRequest{
		ID:        "exec-001",
		Requester: "admin",
		CreatedAt: time.Now(),
		Command:   TypedCommand{Type: "shell"},
		Timeout:   30 * time.Second,
	}
	result := &ExecResult{
		RequestID: "exec-001",
		Summary:   ExecSummary{Total: 3, Success: 3},
	}

	// Record execution
	if err := store.RecordExecution(ctx, req, result); err != nil {
		t.Fatalf("RecordExecution: %v", err)
	}

	// Get execution
	gotReq, gotResult, err := store.GetExecution(ctx, "exec-001")
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if gotReq.ID != "exec-001" {
		t.Errorf("request ID = %q, want %q", gotReq.ID, "exec-001")
	}
	if gotResult.Summary.Total != 3 {
		t.Errorf("result total = %d, want 3", gotResult.Summary.Total)
	}

	// List executions
	execs, err := store.ListExecutions(ctx, ListExecOptions{Requester: "admin"})
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	if len(execs) != 1 {
		t.Errorf("ListExecutions = %d, want 1", len(execs))
	}
}

func TestSQLiteStore_Lock(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Acquire lock
	lock, err := store.AcquireLock(ctx, "deploy-lock", 10*time.Second)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	// Extend lock
	if err := lock.Extend(ctx, 20*time.Second); err != nil {
		t.Fatalf("Extend: %v", err)
	}

	// Unlock
	if err := lock.Unlock(ctx); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
}

func TestSQLiteStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")

	// Write data
	store1, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore (write): %v", err)
	}
	store1.RegisterNode(context.Background(), &Node{
		ID:       "persist-node",
		Hostname: "persistent-host",
		Status:   NodeStatusOnline,
		Labels:   map[string]string{"env": "test"},
	})
	store1.Close()

	// Re-open and read
	store2, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore (read): %v", err)
	}
	defer store2.Close()

	node, err := store2.GetNode(context.Background(), "persist-node")
	if err != nil {
		t.Fatalf("GetNode after reopen: %v", err)
	}
	if node.Hostname != "persistent-host" {
		t.Errorf("hostname after reopen = %q, want %q", node.Hostname, "persistent-host")
	}

	// Verify file exists on disk
	_, err = os.Stat(dbPath)
	if err != nil {
		t.Errorf("database file should exist on disk: %v", err)
	}
}

func testStoreLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewStore_Factory(t *testing.T) {
	logger := testStoreLogger()

	// Memory store
	memStore, err := NewStore(StoreConfig{Backend: "memory"}, logger)
	if err != nil {
		t.Fatalf("NewStore(memory): %v", err)
	}
	if _, ok := memStore.(*MemoryStore); !ok {
		t.Error("expected *MemoryStore")
	}

	// SQLite store
	dir := t.TempDir()
	sqlStore, err := NewStore(StoreConfig{
		Backend:    "sqlite",
		SQLitePath: filepath.Join(dir, "factory.db"),
	}, logger)
	if err != nil {
		t.Fatalf("NewStore(sqlite): %v", err)
	}
	if s, ok := sqlStore.(*SQLiteStore); ok {
		s.Close()
	} else {
		t.Error("expected *SQLiteStore")
	}

	// Unknown backend
	_, err = NewStore(StoreConfig{Backend: "redis"}, logger)
	if err == nil {
		t.Error("expected error for unknown backend")
	}
}
