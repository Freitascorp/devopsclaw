package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/freitascorp/devopsclaw/pkg/fleet"
)

func wsTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestWSServer_NewWSServer(t *testing.T) {
	store := fleet.NewMemoryStore()
	logger := wsTestLogger()

	srv := NewWSServer(ServerConfig{MaxNodes: 5, PingInterval: 10 * time.Second}, store, logger)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.config.MaxNodes != 5 {
		t.Errorf("MaxNodes = %d, want 5", srv.config.MaxNodes)
	}
}

func TestWSServer_Defaults(t *testing.T) {
	srv := NewWSServer(ServerConfig{}, nil, wsTestLogger())
	if srv.config.MaxNodes != 1000 {
		t.Errorf("default MaxNodes = %d, want 1000", srv.config.MaxNodes)
	}
	if srv.config.PingInterval != 15*time.Second {
		t.Errorf("default PingInterval = %v, want 15s", srv.config.PingInterval)
	}
}

func TestWSServer_ConnectedNodeIDs_Empty(t *testing.T) {
	srv := NewWSServer(ServerConfig{}, nil, wsTestLogger())
	ids := srv.ConnectedNodeIDs()
	if len(ids) != 0 {
		t.Errorf("expected 0 connected nodes, got %d", len(ids))
	}
}

func TestWSRelayClient_New(t *testing.T) {
	srv := NewWSServer(ServerConfig{}, nil, wsTestLogger())
	client := NewWSRelayClient(srv, wsTestLogger())
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestWSRelayClient_PingNoTunnel(t *testing.T) {
	srv := NewWSServer(ServerConfig{}, nil, wsTestLogger())
	client := NewWSRelayClient(srv, wsTestLogger())

	err := client.Ping(context.Background(), &fleet.Node{ID: "missing"})
	if err == nil {
		t.Error("expected error for missing tunnel")
	}
}

func TestWSServer_SendCommandNoTunnel(t *testing.T) {
	srv := NewWSServer(ServerConfig{}, nil, wsTestLogger())
	_, err := srv.SendCommandWS(context.Background(), "missing", &CommandEnvelope{})
	if err == nil {
		t.Error("expected error for missing tunnel")
	}
}

func TestWSAgent_New(t *testing.T) {
	agent := NewWSAgent(AgentConfig{
		NodeID:    "test-node",
		RelayAddr: "ws://localhost:9999",
	}, nil, wsTestLogger())

	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if agent.config.ReconnectInterval != 5*time.Second {
		t.Errorf("default ReconnectInterval = %v, want 5s", agent.config.ReconnectInterval)
	}
	if agent.config.HeartbeatInterval != 30*time.Second {
		t.Errorf("default HeartbeatInterval = %v, want 30s", agent.config.HeartbeatInterval)
	}
}

func TestWSAgent_StopBeforeRun(t *testing.T) {
	agent := NewWSAgent(AgentConfig{
		NodeID:    "test-node",
		RelayAddr: "ws://localhost:9999",
	}, nil, wsTestLogger())

	// Stop before Run should not panic
	agent.Stop()

	if agent.IsConnected() {
		t.Error("expected not connected before Run")
	}
}

// Integration test: real WS server + agent handshake
func TestWSServer_AgentHandshake(t *testing.T) {
	store := fleet.NewMemoryStore()
	logger := wsTestLogger()
	srv := NewWSServer(ServerConfig{MaxNodes: 10, PingInterval: 1 * time.Hour}, store, logger)

	// Create a test HTTP server with the relay handler mux
	mux := srv.buildMux()
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Connect a WebSocket client to the relay
	wsURL := "ws" + ts.URL[4:] + "/relay/agent" // http → ws
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	// Send registration
	regMsg := WSMessage{
		Type:      "register",
		NodeID:    "test-agent-1",
		Timestamp: time.Now(),
	}
	if err := wsjson.Write(ctx, conn, regMsg); err != nil {
		t.Fatalf("send registration: %v", err)
	}

	// Read ack
	var ackMsg WSMessage
	if err := wsjson.Read(ctx, conn, &ackMsg); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ackMsg.Type != "registered" {
		t.Errorf("ack.Type = %q, want registered", ackMsg.Type)
	}

	// Verify node is now connected
	ids := srv.ConnectedNodeIDs()
	if len(ids) != 1 {
		t.Fatalf("expected 1 connected node, got %d", len(ids))
	}
	if ids[0] != "test-agent-1" {
		t.Errorf("connected node = %q, want test-agent-1", ids[0])
	}

	// Verify node is registered in store
	nodes, _ := store.ListNodes(ctx)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node in store, got %d", len(nodes))
	}
	if nodes[0].Status != fleet.NodeStatusOnline {
		t.Errorf("node status = %q, want online", nodes[0].Status)
	}
}

// Test command send and response through relay
func TestWSServer_CommandExecution(t *testing.T) {
	store := fleet.NewMemoryStore()
	logger := wsTestLogger()
	srv := NewWSServer(ServerConfig{MaxNodes: 10, PingInterval: 1 * time.Hour}, store, logger)

	mux := srv.buildMux()
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + ts.URL[4:] + "/relay/agent"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	// Register
	wsjson.Write(ctx, conn, WSMessage{Type: "register", NodeID: "exec-node", Timestamp: time.Now()})
	var ack WSMessage
	wsjson.Read(ctx, conn, &ack)

	// Send a command from the server side in a goroutine
	cmdPayload, _ := json.Marshal(fleet.TypedCommand{Type: "shell", Data: json.RawMessage(`{"command":"echo hello"}`)})
	env := &CommandEnvelope{
		RequestID: "test-req-1",
		Command:   fleet.TypedCommand{Type: "shell", Data: cmdPayload},
		Deadline:  time.Now().Add(5 * time.Second),
	}

	resultCh := make(chan *ResultEnvelope, 1)
	errCh := make(chan error, 1)
	go func() {
		r, err := srv.SendCommandWS(ctx, "exec-node", env)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- r
	}()

	// Agent side: read command and send result
	var cmdMsg WSMessage
	if err := wsjson.Read(ctx, conn, &cmdMsg); err != nil {
		t.Fatalf("read command: %v", err)
	}
	if cmdMsg.Type != "command" {
		t.Fatalf("expected command message, got %q", cmdMsg.Type)
	}
	if cmdMsg.RequestID != "test-req-1" {
		t.Errorf("RequestID = %q, want test-req-1", cmdMsg.RequestID)
	}

	// Send result back
	resultPayload, _ := json.Marshal(fleet.NodeResult{
		NodeID: "exec-node",
		Status: "success",
		Output: "hello\n",
	})
	wsjson.Write(ctx, conn, WSMessage{
		Type:      "result",
		RequestID: "test-req-1",
		NodeID:    "exec-node",
		Payload:   resultPayload,
		Timestamp: time.Now(),
	})

	// Wait for result
	select {
	case result := <-resultCh:
		if result.Result.Status != "success" {
			t.Errorf("result status = %q, want success", result.Result.Status)
		}
		if result.Result.Output != "hello\n" {
			t.Errorf("result output = %q, want hello\\n", result.Result.Output)
		}
	case err := <-errCh:
		t.Fatalf("SendCommandWS error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for result")
	}
}

// Test auth token rejection
func TestWSServer_AuthTokenRejection(t *testing.T) {
	store := fleet.NewMemoryStore()
	srv := NewWSServer(ServerConfig{
		AuthToken: "secret-token",
		MaxNodes:  10,
		PingInterval: 1 * time.Hour,
	}, store, wsTestLogger())

	mux := srv.buildMux()
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + ts.URL[4:] + "/relay/agent"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try connecting without token — should fail
	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatal("expected error for unauthorized connection")
	}
	if resp != nil && resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	// Connect with correct token
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{
			"Authorization": {fmt.Sprintf("Bearer %s", "secret-token")},
		},
	})
	if err != nil {
		t.Fatalf("dial with token: %v", err)
	}
	conn.Close(websocket.StatusNormalClosure, "test")
}

// Test max capacity
func TestWSServer_MaxCapacity(t *testing.T) {
	store := fleet.NewMemoryStore()
	srv := NewWSServer(ServerConfig{MaxNodes: 1, PingInterval: 1 * time.Hour}, store, wsTestLogger())

	mux := srv.buildMux()
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + ts.URL[4:] + "/relay/agent"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First connection
	conn1, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	defer conn1.Close(websocket.StatusNormalClosure, "test")

	// Register first node
	wsjson.Write(ctx, conn1, WSMessage{Type: "register", NodeID: "node-1", Timestamp: time.Now()})
	var ack WSMessage
	wsjson.Read(ctx, conn1, &ack)

	// Second connection should be rejected at capacity (after registering)
	conn2, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("second dial: %v", err)
	}
	defer conn2.Close(websocket.StatusNormalClosure, "test")

	wsjson.Write(ctx, conn2, WSMessage{Type: "register", NodeID: "node-2", Timestamp: time.Now()})
	// The server should close the connection
	var ack2 WSMessage
	err = wsjson.Read(ctx, conn2, &ack2)
	// Either read error (connection closed) or ack type is not "registered" indicates capacity rejection
	if err == nil && ack2.Type == "registered" {
		t.Error("expected second connection to be rejected at max capacity")
	}
}

// Test health endpoint
func TestWSServer_HealthEndpoint(t *testing.T) {
	srv := NewWSServer(ServerConfig{MaxNodes: 100, PingInterval: 1 * time.Hour}, nil, wsTestLogger())

	mux := srv.buildMux()
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/relay/health")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("health status = %v, want ok", body["status"])
	}
}
