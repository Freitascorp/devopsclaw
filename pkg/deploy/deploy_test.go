package deploy

import (
	"fmt"
	"testing"

	"github.com/freitascorp/devopsclaw/pkg/fleet"
)

func TestSpec_Validation(t *testing.T) {
	tests := []struct {
		name    string
		spec    Spec
		wantErr string
	}{
		{
			name:    "missing service",
			spec:    Spec{Version: "v1", DeployCommand: "deploy.sh"},
			wantErr: "service name is required",
		},
		{
			name:    "missing version",
			spec:    Spec{Service: "myapp", DeployCommand: "deploy.sh"},
			wantErr: "version is required",
		},
		{
			name:    "missing deploy command",
			spec:    Spec{Service: "myapp", Version: "v1"},
			wantErr: "deploy_command is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a deployer with nil executor/store to test validation only
			d := &Deployer{active: make(map[string]*Result)}
			_, err := d.Deploy(nil, tt.spec)
			if err == nil {
				t.Fatalf("expected error %q", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestSplitIntoBatches(t *testing.T) {
	nodes := make([]*fleet.Node, 10)
	for i := range nodes {
		nodes[i] = &fleet.Node{ID: fleet.NodeID(string(rune('a' + i)))}
	}

	tests := []struct {
		batchSize int
		wantCount int
		wantSizes []int
	}{
		{batchSize: 3, wantCount: 4, wantSizes: []int{3, 3, 3, 1}},
		{batchSize: 5, wantCount: 2, wantSizes: []int{5, 5}},
		{batchSize: 10, wantCount: 1, wantSizes: []int{10}},
		{batchSize: 1, wantCount: 10, wantSizes: []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}},
		{batchSize: 15, wantCount: 1, wantSizes: []int{10}},
	}

	for _, tt := range tests {
		batches := splitIntoBatches(nodes, tt.batchSize)
		if len(batches) != tt.wantCount {
			t.Errorf("splitIntoBatches(%d): got %d batches, want %d", tt.batchSize, len(batches), tt.wantCount)
			continue
		}
		for i, batch := range batches {
			if len(batch) != tt.wantSizes[i] {
				t.Errorf("splitIntoBatches(%d): batch[%d] has %d nodes, want %d", tt.batchSize, i, len(batch), tt.wantSizes[i])
			}
		}
	}
}

func TestSplitIntoBatches_Empty(t *testing.T) {
	batches := splitIntoBatches(nil, 3)
	if len(batches) != 0 {
		t.Errorf("expected 0 batches for nil input, got %d", len(batches))
	}
}

func TestNodeIDs(t *testing.T) {
	nodes := []*fleet.Node{
		{ID: "node-1"},
		{ID: "node-2"},
		{ID: "node-3"},
	}
	ids := nodeIDs(nodes)
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "node-1" {
		t.Errorf("ids[0] = %q, want node-1", ids[0])
	}
}

func TestStrategy_Constants(t *testing.T) {
	// Ensure strategy constants are correct string values
	if StrategyRolling != "rolling" {
		t.Errorf("StrategyRolling = %q", StrategyRolling)
	}
	if StrategyCanary != "canary" {
		t.Errorf("StrategyCanary = %q", StrategyCanary)
	}
	if StrategyBlueGreen != "blue-green" {
		t.Errorf("StrategyBlueGreen = %q", StrategyBlueGreen)
	}
	if StrategyAllAtOnce != "all-at-once" {
		t.Errorf("StrategyAllAtOnce = %q", StrategyAllAtOnce)
	}
	if StrategySerial != "serial" {
		t.Errorf("StrategySerial = %q", StrategySerial)
	}
}

func TestState_Constants(t *testing.T) {
	if StatePending != "pending" {
		t.Errorf("StatePending = %q", StatePending)
	}
	if StateRunning != "running" {
		t.Errorf("StateRunning = %q", StateRunning)
	}
	if StateComplete != "complete" {
		t.Errorf("StateComplete = %q", StateComplete)
	}
	if StateFailed != "failed" {
		t.Errorf("StateFailed = %q", StateFailed)
	}
	if StateRollback != "rollback" {
		t.Errorf("StateRollback = %q", StateRollback)
	}
}

func TestNewDeployer(t *testing.T) {
	d := NewDeployer(nil, nil, nil)
	if d == nil {
		t.Fatal("expected non-nil deployer")
	}
	if d.active == nil {
		t.Error("expected active map to be initialized")
	}
}

func TestDeployer_ActiveDeployments_Empty(t *testing.T) {
	d := NewDeployer(nil, nil, nil)
	active := d.ActiveDeployments()
	if len(active) != 0 {
		t.Errorf("expected 0 active deployments, got %d", len(active))
	}
}

func TestDeployer_FailHelper(t *testing.T) {
	d := NewDeployer(nil, nil, nil)
	result := &Result{ID: "test"}
	got, err := d.fail(result, fmt.Errorf("test error"))
	if err == nil {
		t.Fatal("expected error")
	}
	if got.State != StateFailed {
		t.Errorf("State = %q, want failed", got.State)
	}
	if got.Error != "test error" {
		t.Errorf("Error = %q, want test error", got.Error)
	}
}
