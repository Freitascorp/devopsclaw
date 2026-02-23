package audit

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func tempStore(t *testing.T) *FileStore {
	t.Helper()
	dir := t.TempDir()
	return NewFileStore(dir)
}

func TestFileStore_AppendAndQuery(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	// Append event
	event := &Event{
		Type:   EventFleetExec,
		User:   "alice",
		Action: "fleet.exec",
		Target: &EventTarget{Command: "uptime"},
		Result: &EventResult{Status: "success", NodesTotal: 3, NodesSuccess: 3},
	}
	if err := store.Append(ctx, event); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// ID and timestamp should be auto-populated
	if event.ID == "" {
		t.Error("expected event.ID to be set")
	}
	if event.Timestamp.IsZero() {
		t.Error("expected event.Timestamp to be set")
	}

	// Query all
	events, err := store.Query(ctx, QueryOptions{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].User != "alice" {
		t.Errorf("User = %q, want alice", events[0].User)
	}
	if events[0].Target.Command != "uptime" {
		t.Errorf("Target.Command = %q, want uptime", events[0].Target.Command)
	}
}

func TestFileStore_QueryFilterByUser(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	store.Append(ctx, &Event{User: "alice", Type: EventFleetExec, Action: "run"})
	store.Append(ctx, &Event{User: "bob", Type: EventFleetExec, Action: "run"})
	store.Append(ctx, &Event{User: "alice", Type: EventBrowse, Action: "browse"})

	events, err := store.Query(ctx, QueryOptions{User: "alice"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events for alice, got %d", len(events))
	}
}

func TestFileStore_QueryFilterByType(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	store.Append(ctx, &Event{User: "alice", Type: EventFleetExec, Action: "run"})
	store.Append(ctx, &Event{User: "bob", Type: EventBrowse, Action: "browse"})

	events, err := store.Query(ctx, QueryOptions{Type: EventBrowse})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 browse event, got %d", len(events))
	}
	if events[0].User != "bob" {
		t.Errorf("User = %q, want bob", events[0].User)
	}
}

func TestFileStore_QueryFilterBySince(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	oldEvent := &Event{User: "alice", Type: EventFleetExec, Action: "old", Timestamp: time.Now().Add(-2 * time.Hour)}
	store.Append(ctx, oldEvent)
	store.Append(ctx, &Event{User: "alice", Type: EventFleetExec, Action: "new"})

	events, err := store.Query(ctx, QueryOptions{Since: time.Now().Add(-1 * time.Hour)})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 recent event, got %d", len(events))
	}
	if events[0].Action != "new" {
		t.Errorf("Action = %q, want new", events[0].Action)
	}
}

func TestFileStore_QueryLimit(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		store.Append(ctx, &Event{User: "alice", Type: EventFleetExec, Action: "run"})
	}

	events, err := store.Query(ctx, QueryOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestFileStore_Export(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	store.Append(ctx, &Event{User: "alice", Type: EventFleetExec, Action: "run"})
	store.Append(ctx, &Event{User: "bob", Type: EventBrowse, Action: "browse"})

	events, err := store.Export(ctx, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestFileStore_EmptyLog(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	events, err := store.Query(ctx, QueryOptions{})
	if err != nil {
		t.Fatalf("Query empty: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestFileStore_ConcurrentAppend(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	n := 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			store.Append(ctx, &Event{
				User:   "concurrent",
				Type:   EventFleetExec,
				Action: "run",
			})
		}(i)
	}
	wg.Wait()

	events, err := store.Query(ctx, QueryOptions{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != n {
		t.Fatalf("expected %d events, got %d", n, len(events))
	}
}

func TestFileStore_MalformedLines(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()

	// Write some valid events
	store.Append(ctx, &Event{User: "alice", Type: EventFleetExec, Action: "run"})

	// Corrupt the file with malformed JSON
	f, _ := os.OpenFile(filepath.Join(dir, "audit.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	f.Write([]byte("not-valid-json\n"))
	f.Close()

	store.Append(ctx, &Event{User: "bob", Type: EventBrowse, Action: "browse"})

	// Should skip malformed line and return the valid ones
	events, err := store.Query(ctx, QueryOptions{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 valid events (skipping malformed), got %d", len(events))
	}
}

func TestLogger_LogFleetExec(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	logger := NewLogger(store, "admin")
	err := logger.LogFleetExec(ctx, "df -h", &EventTarget{Command: "df -h"}, &EventResult{Status: "success"})
	if err != nil {
		t.Fatalf("LogFleetExec: %v", err)
	}

	events, _ := store.Query(ctx, QueryOptions{})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventFleetExec {
		t.Errorf("Type = %q, want fleet.exec", events[0].Type)
	}
	if events[0].User != "admin" {
		t.Errorf("User = %q, want admin", events[0].User)
	}
}

func TestLogger_LogDeploy(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	logger := NewLogger(store, "ops")
	err := logger.LogDeploy(ctx, "myapp", "v2.0", "rolling", &EventResult{Status: "success"})
	if err != nil {
		t.Fatalf("LogDeploy: %v", err)
	}

	events, _ := store.Query(ctx, QueryOptions{})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventFleetDeploy {
		t.Errorf("Type = %q, want fleet.deploy", events[0].Type)
	}
}

func TestLogger_LogRunbook(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	logger := NewLogger(store, "ops")
	err := logger.LogRunbook(ctx, "incident-db", false, &EventResult{Status: "success"})
	if err != nil {
		t.Fatalf("LogRunbook: %v", err)
	}

	events, _ := store.Query(ctx, QueryOptions{})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventRunbook {
		t.Errorf("Type = %q, want runbook.run", events[0].Type)
	}
}

func TestLogger_LogBrowse(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	logger := NewLogger(store, "ops")
	err := logger.LogBrowse(ctx, "https://example.com", "check status", &EventResult{Status: "success"})
	if err != nil {
		t.Fatalf("LogBrowse: %v", err)
	}

	events, _ := store.Query(ctx, QueryOptions{})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventBrowse {
		t.Errorf("Type = %q, want browse", events[0].Type)
	}
}

func TestFileStore_QueryFilterByUntil(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	store.Append(ctx, &Event{User: "alice", Type: EventFleetExec, Action: "old", Timestamp: time.Now().Add(-2 * time.Hour)})
	store.Append(ctx, &Event{User: "alice", Type: EventFleetExec, Action: "new"})

	events, err := store.Query(ctx, QueryOptions{Until: time.Now().Add(-1 * time.Hour)})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 old event, got %d", len(events))
	}
	if events[0].Action != "old" {
		t.Errorf("Action = %q, want old", events[0].Action)
	}
}

func TestFileStore_CustomID(t *testing.T) {
	store := tempStore(t)
	ctx := context.Background()

	event := &Event{ID: "custom-123", User: "alice", Type: EventFleetExec, Action: "run"}
	store.Append(ctx, event)

	events, _ := store.Query(ctx, QueryOptions{})
	if events[0].ID != "custom-123" {
		t.Errorf("ID = %q, want custom-123", events[0].ID)
	}
}
