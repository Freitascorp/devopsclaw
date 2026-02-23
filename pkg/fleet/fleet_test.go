package fleet

import (
	"context"
	"testing"
	"time"
)

func TestTargetSelector_Resolve_All(t *testing.T) {
	roster := testRoster()
	sel := &TargetSelector{All: true}
	result := sel.Resolve(roster)
	if len(result) != 3 { // 3 online/degraded out of 4
		t.Errorf("expected 3 nodes, got %d", len(result))
	}
}

func TestTargetSelector_Resolve_ByGroup(t *testing.T) {
	roster := testRoster()
	sel := &TargetSelector{Groups: []GroupName{"web"}}
	result := sel.Resolve(roster)
	if len(result) != 2 {
		t.Errorf("expected 2 web nodes, got %d", len(result))
	}
}

func TestTargetSelector_Resolve_ByLabels(t *testing.T) {
	roster := testRoster()
	sel := &TargetSelector{Labels: map[string]string{"env": "prod"}}
	result := sel.Resolve(roster)
	if len(result) != 2 {
		t.Errorf("expected 2 prod nodes, got %d", len(result))
	}
}

func TestTargetSelector_Resolve_ByNodeIDs(t *testing.T) {
	roster := testRoster()
	sel := &TargetSelector{NodeIDs: []NodeID{"node-1", "node-3"}}
	result := sel.Resolve(roster)
	if len(result) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result))
	}
}

func TestTargetSelector_Resolve_MaxNodes(t *testing.T) {
	roster := testRoster()
	sel := &TargetSelector{All: true, MaxNodes: 1}
	result := sel.Resolve(roster)
	if len(result) != 1 {
		t.Errorf("expected 1 node, got %d", len(result))
	}
}

func TestMemoryStore_CRUD(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	node := &Node{
		ID:       "test-1",
		Hostname: "host1",
		Status:   NodeStatusOnline,
		Groups:   []GroupName{"web"},
		Labels:   map[string]string{"env": "prod"},
	}

	// Register
	if err := store.RegisterNode(ctx, node); err != nil {
		t.Fatal(err)
	}

	// Get
	got, err := store.GetNode(ctx, "test-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hostname != "host1" {
		t.Errorf("expected hostname host1, got %s", got.Hostname)
	}

	// List
	nodes, err := store.ListNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(nodes))
	}

	// Update status
	if err := store.UpdateNodeStatus(ctx, "test-1", NodeStatusDegraded); err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetNode(ctx, "test-1")
	if got.Status != NodeStatusDegraded {
		t.Errorf("expected degraded, got %s", got.Status)
	}

	// Deregister
	if err := store.DeregisterNode(ctx, "test-1"); err != nil {
		t.Fatal(err)
	}
	nodes, _ = store.ListNodes(ctx)
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestExecRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     ExecRequest
		wantErr bool
	}{
		{"empty ID", ExecRequest{}, true},
		{"empty command type", ExecRequest{ID: "1"}, true},
		{"unknown command type", ExecRequest{ID: "1", Command: TypedCommand{Type: "unknown"}}, true},
		{"valid shell", ExecRequest{ID: "1", Command: TypedCommand{Type: "shell"}}, false},
		{"valid deploy", ExecRequest{ID: "1", Command: TypedCommand{Type: "deploy"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func testRoster() []*Node {
	return []*Node{
		{ID: "node-1", Hostname: "web-1", Status: NodeStatusOnline, Groups: []GroupName{"web"}, Labels: map[string]string{"env": "prod"}},
		{ID: "node-2", Hostname: "web-2", Status: NodeStatusOnline, Groups: []GroupName{"web", "api"}, Labels: map[string]string{"env": "prod"}},
		{ID: "node-3", Hostname: "db-1", Status: NodeStatusDegraded, Groups: []GroupName{"db"}, Labels: map[string]string{"env": "staging"}},
		{ID: "node-4", Hostname: "offline-1", Status: NodeStatusOffline, Groups: []GroupName{"web"}, Labels: map[string]string{"env": "staging"}},
	}
}

func TestMemoryStore_ListByGroup(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	for _, n := range testRoster() {
		store.RegisterNode(ctx, n)
	}

	nodes, err := store.ListNodesByGroup(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 { // node-1, node-2, node-4
		t.Errorf("expected 3 web nodes, got %d", len(nodes))
	}
}

func TestMemoryStore_RecordExecution(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	req := &ExecRequest{
		ID:        "exec-1",
		Requester: "admin",
		CreatedAt: time.Now(),
		Command:   TypedCommand{Type: "shell"},
	}
	result := &ExecResult{
		RequestID: "exec-1",
		Summary:   ExecSummary{Total: 2, Success: 2},
	}

	if err := store.RecordExecution(ctx, req, result); err != nil {
		t.Fatal(err)
	}

	gotReq, gotResult, err := store.GetExecution(ctx, "exec-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotReq.Requester != "admin" {
		t.Errorf("expected admin, got %s", gotReq.Requester)
	}
	if gotResult.Summary.Success != 2 {
		t.Errorf("expected 2 success, got %d", gotResult.Summary.Success)
	}
}
