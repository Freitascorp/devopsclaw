package relay

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/freitascorp/devopsclaw/pkg/fleet"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewServer(t *testing.T) {
	s := NewServer(ServerConfig{
		ListenAddr: ":0",
		AuthToken:  "test-token",
	}, nil, testLogger())

	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.config.MaxNodes != 1000 {
		t.Errorf("expected default MaxNodes 1000, got %d", s.config.MaxNodes)
	}
	if s.config.PingInterval != 15*time.Second {
		t.Errorf("expected default PingInterval 15s, got %v", s.config.PingInterval)
	}
}

func TestNewServer_CustomConfig(t *testing.T) {
	s := NewServer(ServerConfig{
		MaxNodes:     500,
		PingInterval: 30 * time.Second,
	}, nil, testLogger())

	if s.config.MaxNodes != 500 {
		t.Errorf("expected MaxNodes 500, got %d", s.config.MaxNodes)
	}
	if s.config.PingInterval != 30*time.Second {
		t.Errorf("expected PingInterval 30s, got %v", s.config.PingInterval)
	}
}

func TestRegisterTunnel(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())

	tunnel, err := s.RegisterTunnel("node-1", "192.168.1.1:5000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tunnel == nil {
		t.Fatal("expected non-nil tunnel")
	}
	if tunnel.NodeID != "node-1" {
		t.Errorf("expected node-1, got %s", tunnel.NodeID)
	}
	if tunnel.RemoteAddr != "192.168.1.1:5000" {
		t.Errorf("expected 192.168.1.1:5000, got %s", tunnel.RemoteAddr)
	}
}

func TestRegisterTunnel_MaxNodesReached(t *testing.T) {
	s := NewServer(ServerConfig{MaxNodes: 2}, nil, testLogger())

	_, err := s.RegisterTunnel("node-1", "addr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = s.RegisterTunnel("node-2", "addr-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = s.RegisterTunnel("node-3", "addr-3")
	if err == nil {
		t.Fatal("expected error when max nodes reached")
	}
}

func TestRegisterTunnel_Reconnect(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())

	tunnel1, err := s.RegisterTunnel("node-1", "addr-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Register again — should close old and create new
	tunnel2, err := s.RegisterTunnel("node-1", "addr-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Old tunnel's Done channel should be closed
	select {
	case <-tunnel1.Done:
		// expected
	default:
		t.Error("expected old tunnel Done channel to be closed")
	}

	if tunnel2.RemoteAddr != "addr-2" {
		t.Errorf("expected addr-2, got %s", tunnel2.RemoteAddr)
	}
}

func TestDeregisterTunnel(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())

	tunnel, _ := s.RegisterTunnel("node-1", "addr-1")
	s.DeregisterTunnel("node-1")

	// Done channel should be closed
	select {
	case <-tunnel.Done:
	default:
		t.Error("expected Done channel to be closed")
	}

	nodes := s.ConnectedNodes()
	if len(nodes) != 0 {
		t.Errorf("expected 0 connected nodes, got %d", len(nodes))
	}
}

func TestDeregisterTunnel_NotFound(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())
	// Should not panic
	s.DeregisterTunnel("nonexistent")
}

func TestConnectedNodes(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())

	nodes := s.ConnectedNodes()
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}

	s.RegisterTunnel("node-1", "addr-1")
	s.RegisterTunnel("node-2", "addr-2")

	nodes = s.ConnectedNodes()
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestSendCommand(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())

	nodeID := fleet.NodeID("node-1")
	tunnel, _ := s.RegisterTunnel(nodeID, "addr-1")

	env := &CommandEnvelope{
		RequestID: "req-1",
		Command:   fleet.TypedCommand{Type: "shell"},
		Deadline:  time.Now().Add(5 * time.Second),
	}

	// Simulate node processing in background
	go func() {
		cmd := <-tunnel.CommandCh
		tunnel.ResultCh <- &ResultEnvelope{
			RequestID: cmd.RequestID,
			Result:    fleet.NodeResult{Status: "success"},
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := s.SendCommand(ctx, nodeID, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequestID != "req-1" {
		t.Errorf("expected request ID req-1, got %s", result.RequestID)
	}
	if result.Result.Status != "success" {
		t.Error("expected success result")
	}
}

func TestSendCommand_NoTunnel(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())

	ctx := context.Background()
	_, err := s.SendCommand(ctx, "nonexistent", &CommandEnvelope{RequestID: "req-1"})
	if err == nil {
		t.Fatal("expected error for missing tunnel")
	}
}

func TestSendCommand_ContextCancelled(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())

	nodeID := fleet.NodeID("node-1")
	s.RegisterTunnel(nodeID, "addr-1")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := s.SendCommand(ctx, nodeID, &CommandEnvelope{
		RequestID: "req-1",
		Command:   fleet.TypedCommand{Type: "shell"},
	})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestSendCommand_TunnelClosedDuringSend(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())

	nodeID := fleet.NodeID("node-1")
	tunnel, _ := s.RegisterTunnel(nodeID, "addr-1")

	// Fill the command channel to block the send
	for i := 0; i < 32; i++ {
		tunnel.CommandCh <- &CommandEnvelope{RequestID: fmt.Sprintf("fill-%d", i)}
	}

	// Close the tunnel to unblock
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(tunnel.Done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := s.SendCommand(ctx, nodeID, &CommandEnvelope{RequestID: "req-blocked"})
	if err == nil {
		t.Fatal("expected error when tunnel closed")
	}
}

// --- Agent tests ---

type mockExecutor struct{}

func (m *mockExecutor) Execute(ctx context.Context, cmd fleet.TypedCommand) (*fleet.NodeResult, error) {
	return &fleet.NodeResult{Status: "success"}, nil
}

func TestNewAgent(t *testing.T) {
	a := NewAgent(AgentConfig{
		NodeID:    "node-1",
		RelayAddr: "localhost:9443",
	}, &mockExecutor{}, testLogger())

	if a == nil {
		t.Fatal("expected non-nil agent")
	}
	if a.config.ReconnectInterval != 5*time.Second {
		t.Errorf("expected default reconnect interval 5s, got %v", a.config.ReconnectInterval)
	}
	if a.config.HeartbeatInterval != 30*time.Second {
		t.Errorf("expected default heartbeat interval 30s, got %v", a.config.HeartbeatInterval)
	}
}

func TestNewAgent_CustomConfig(t *testing.T) {
	a := NewAgent(AgentConfig{
		ReconnectInterval: 10 * time.Second,
		HeartbeatInterval: 60 * time.Second,
	}, &mockExecutor{}, testLogger())

	if a.config.ReconnectInterval != 10*time.Second {
		t.Errorf("expected 10s, got %v", a.config.ReconnectInterval)
	}
	if a.config.HeartbeatInterval != 60*time.Second {
		t.Errorf("expected 60s, got %v", a.config.HeartbeatInterval)
	}
}

func TestAgentIsConnected(t *testing.T) {
	a := NewAgent(AgentConfig{}, &mockExecutor{}, testLogger())
	if a.IsConnected() {
		t.Fatal("new agent should not be connected")
	}
}

func TestAgentStop(t *testing.T) {
	a := NewAgent(AgentConfig{}, &mockExecutor{}, testLogger())
	a.Stop() // should not panic
}

func TestAgentRun_CancelledContext(t *testing.T) {
	a := NewAgent(AgentConfig{
		NodeID:            "node-1",
		RelayAddr:         "localhost:9443",
		ReconnectInterval: 100 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
	}, &mockExecutor{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- a.Run(ctx)
	}()

	// Give agent a moment to start, then cancel
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not stop in time")
	}
}

// --- TunnelRelayClient tests ---

func TestNewTunnelRelayClient(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())
	c := NewTunnelRelayClient(s, testLogger())
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestTunnelRelayClient_Execute(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())
	c := NewTunnelRelayClient(s, testLogger())

	nodeID := fleet.NodeID("node-1")
	tunnel, _ := s.RegisterTunnel(nodeID, "addr-1")

	// Simulate node processing
	go func() {
		cmd := <-tunnel.CommandCh
		tunnel.ResultCh <- &ResultEnvelope{
			RequestID: cmd.RequestID,
			Result:    fleet.NodeResult{Status: "success", Output: "done"},
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	node := &fleet.Node{ID: nodeID}
	result, err := c.Execute(ctx, node, fleet.TypedCommand{Type: "shell"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "success" {
		t.Error("expected success")
	}
	if result.Output != "done" {
		t.Errorf("expected output 'done', got %s", result.Output)
	}
}

func TestTunnelRelayClient_Ping(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())
	c := NewTunnelRelayClient(s, testLogger())

	nodeID := fleet.NodeID("node-1")
	s.RegisterTunnel(nodeID, "addr-1")

	err := c.Ping(context.Background(), &fleet.Node{ID: nodeID})
	if err != nil {
		t.Fatalf("expected no error for connected node, got %v", err)
	}
}

func TestTunnelRelayClient_Ping_NoTunnel(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, testLogger())
	c := NewTunnelRelayClient(s, testLogger())

	err := c.Ping(context.Background(), &fleet.Node{ID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing tunnel")
	}
}
