package fleet

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStore is an in-process fleet store for development and testing.
// For production, use SQLiteStore or PostgresStore.
type MemoryStore struct {
	mu         sync.RWMutex
	nodes      map[NodeID]*Node
	executions map[string]*executionRecord
}

type executionRecord struct {
	Request *ExecRequest
	Result  *ExecResult
}

// NewMemoryStore creates an in-memory fleet store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nodes:      make(map[NodeID]*Node),
		executions: make(map[string]*executionRecord),
	}
}

func (s *MemoryStore) RegisterNode(_ context.Context, node *Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[node.ID] = node
	return nil
}

func (s *MemoryStore) DeregisterNode(_ context.Context, id NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[id]; !ok {
		return fmt.Errorf("node %s not found", id)
	}
	delete(s.nodes, id)
	return nil
}

func (s *MemoryStore) UpdateNodeStatus(_ context.Context, id NodeID, status NodeStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok {
		return fmt.Errorf("node %s not found", id)
	}
	n.Status = status
	return nil
}

func (s *MemoryStore) UpdateNodeHeartbeat(_ context.Context, id NodeID, resources NodeResources) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok {
		return fmt.Errorf("node %s not found", id)
	}
	n.LastSeen = time.Now()
	n.Resources = resources
	return nil
}

func (s *MemoryStore) GetNode(_ context.Context, id NodeID) (*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node %s not found", id)
	}
	return n, nil
}

func (s *MemoryStore) ListNodes(_ context.Context) ([]*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Node
	for _, n := range s.nodes {
		out = append(out, n)
	}
	return out, nil
}

func (s *MemoryStore) ListNodesByGroup(_ context.Context, group GroupName) ([]*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Node
	for _, n := range s.nodes {
		for _, g := range n.Groups {
			if g == group {
				out = append(out, n)
				break
			}
		}
	}
	return out, nil
}

func (s *MemoryStore) ListNodesByLabels(_ context.Context, labels map[string]string) ([]*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Node
	for _, n := range s.nodes {
		if matchLabels(n.Labels, labels) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (s *MemoryStore) RecordExecution(_ context.Context, req *ExecRequest, result *ExecResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions[req.ID] = &executionRecord{Request: req, Result: result}
	return nil
}

func (s *MemoryStore) GetExecution(_ context.Context, id string) (*ExecRequest, *ExecResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.executions[id]
	if !ok {
		return nil, nil, fmt.Errorf("execution %s not found", id)
	}
	return rec.Request, rec.Result, nil
}

func (s *MemoryStore) ListExecutions(_ context.Context, opts ListExecOptions) ([]*ExecRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*ExecRequest
	for _, rec := range s.executions {
		if opts.Requester != "" && rec.Request.Requester != opts.Requester {
			continue
		}
		if !opts.Since.IsZero() && rec.Request.CreatedAt.Before(opts.Since) {
			continue
		}
		out = append(out, rec.Request)
	}
	// Simple pagination
	if opts.Offset > 0 && opts.Offset < len(out) {
		out = out[opts.Offset:]
	}
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

func (s *MemoryStore) AcquireLock(_ context.Context, key string, ttl time.Duration) (Lock, error) {
	// Simple in-memory lock — not suitable for multi-process use.
	return &memoryLock{key: key}, nil
}

type memoryLock struct {
	key string
}

func (l *memoryLock) Unlock(_ context.Context) error   { return nil }
func (l *memoryLock) Extend(_ context.Context, _ time.Duration) error { return nil }
