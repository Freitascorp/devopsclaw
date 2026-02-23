// Package audit provides an immutable, structured audit log for DevOpsClaw.
//
// Every CLI command, fleet execution, browser session, deploy, and RBAC decision
// is recorded as a structured event. Events are append-only and can be exported
// to JSON for SIEM ingestion.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventType categorizes audit events.
type EventType string

const (
	EventFleetExec    EventType = "fleet.exec"
	EventFleetDeploy  EventType = "fleet.deploy"
	EventNodeRegister EventType = "node.register"
	EventNodeRemove   EventType = "node.remove"
	EventBrowse       EventType = "browse"
	EventRunbook      EventType = "runbook.run"
	EventShellExec    EventType = "shell.exec"
	EventFileWrite    EventType = "file.write"
	EventAuth         EventType = "auth"
	EventRBAC         EventType = "rbac.decision"
	EventCron         EventType = "cron.exec"
	EventConfig       EventType = "config.change"
)

// Event is a single immutable audit record.
type Event struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"ts"`
	Type      EventType      `json:"type"`
	User      string         `json:"user"`
	Action    string         `json:"action"`
	Target    *EventTarget   `json:"target,omitempty"`
	Result    *EventResult   `json:"result,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// EventTarget describes what was targeted by the action.
type EventTarget struct {
	NodeIDs []string          `json:"node_ids,omitempty"`
	Tags    map[string]string `json:"tags,omitempty"`
	Env     string            `json:"env,omitempty"`
	Command string            `json:"command,omitempty"`
}

// EventResult captures the outcome of the action.
type EventResult struct {
	Status       string        `json:"status"` // "success", "failure", "partial"
	NodesTotal   int           `json:"nodes_total,omitempty"`
	NodesSuccess int           `json:"nodes_success,omitempty"`
	NodesFailed  int           `json:"nodes_failed,omitempty"`
	Duration     time.Duration `json:"duration_ms,omitempty"`
	Error        string        `json:"error,omitempty"`
}

// QueryOptions filters audit log queries.
type QueryOptions struct {
	User  string
	Type  EventType
	Since time.Time
	Until time.Time
	Limit int
}

// Store is the persistence interface for the audit log.
type Store interface {
	// Append writes an event to the audit log. Events are immutable once written.
	Append(ctx context.Context, event *Event) error

	// Query retrieves events matching the given filters.
	Query(ctx context.Context, opts QueryOptions) ([]*Event, error)

	// Export writes all events since the given time as JSON lines to the writer.
	Export(ctx context.Context, since time.Time) ([]*Event, error)
}

// ------------------------------------------------------------------
// File-based audit store (append-only JSONL)
// ------------------------------------------------------------------

// FileStore is an append-only file-based audit store using JSON Lines format.
// Each line is a complete JSON event. The file is never modified, only appended to.
type FileStore struct {
	dir string
	mu  sync.Mutex
}

// NewFileStore creates a file-based audit store at the given directory.
func NewFileStore(dir string) *FileStore {
	os.MkdirAll(dir, 0o700)
	return &FileStore{dir: dir}
}

func (s *FileStore) logFile() string {
	return filepath.Join(s.dir, "audit.jsonl")
}

// Append writes an event to the audit log.
func (s *FileStore) Append(ctx context.Context, event *Event) error {
	if event.ID == "" {
		event.ID = fmt.Sprintf("evt_%d", time.Now().UnixNano())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.logFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}

	return nil
}

// Query reads events matching the given filters.
func (s *FileStore) Query(ctx context.Context, opts QueryOptions) ([]*Event, error) {
	all, err := s.readAll()
	if err != nil {
		return nil, err
	}

	var results []*Event
	for _, e := range all {
		if opts.User != "" && e.User != opts.User {
			continue
		}
		if opts.Type != "" && e.Type != opts.Type {
			continue
		}
		if !opts.Since.IsZero() && e.Timestamp.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && e.Timestamp.After(opts.Until) {
			continue
		}
		results = append(results, e)
		if opts.Limit > 0 && len(results) >= opts.Limit {
			break
		}
	}

	return results, nil
}

// Export returns all events since the given time.
func (s *FileStore) Export(ctx context.Context, since time.Time) ([]*Event, error) {
	return s.Query(ctx, QueryOptions{Since: since})
}

func (s *FileStore) readAll() ([]*Event, error) {
	data, err := os.ReadFile(s.logFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var events []*Event
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip malformed lines
		}
		events = append(events, &e)
	}
	return events, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := range data {
		if data[i] == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// ------------------------------------------------------------------
// Logger is a convenience wrapper for emitting audit events
// ------------------------------------------------------------------

// Logger provides helper methods for common audit patterns.
type Logger struct {
	store Store
	user  string
}

// NewLogger creates an audit logger for the given user.
func NewLogger(store Store, user string) *Logger {
	return &Logger{store: store, user: user}
}

// LogFleetExec records a fleet execution event.
func (l *Logger) LogFleetExec(ctx context.Context, command string, target *EventTarget, result *EventResult) error {
	return l.store.Append(ctx, &Event{
		Type:   EventFleetExec,
		User:   l.user,
		Action: "fleet.exec",
		Target: target,
		Result: result,
		Metadata: map[string]any{
			"command": command,
		},
	})
}

// LogDeploy records a deployment event.
func (l *Logger) LogDeploy(ctx context.Context, service, version, strategy string, result *EventResult) error {
	return l.store.Append(ctx, &Event{
		Type:   EventFleetDeploy,
		User:   l.user,
		Action: "fleet.deploy",
		Result: result,
		Metadata: map[string]any{
			"service":  service,
			"version":  version,
			"strategy": strategy,
		},
	})
}

// LogBrowse records a browser automation event.
func (l *Logger) LogBrowse(ctx context.Context, url, task string, result *EventResult) error {
	return l.store.Append(ctx, &Event{
		Type:   EventBrowse,
		User:   l.user,
		Action: "browse",
		Result: result,
		Metadata: map[string]any{
			"url":  url,
			"task": task,
		},
	})
}

// LogRunbook records a runbook execution event.
func (l *Logger) LogRunbook(ctx context.Context, name string, dryRun bool, result *EventResult) error {
	return l.store.Append(ctx, &Event{
		Type:   EventRunbook,
		User:   l.user,
		Action: "runbook.run",
		Result: result,
		Metadata: map[string]any{
			"runbook": name,
			"dry_run": dryRun,
		},
	})
}
