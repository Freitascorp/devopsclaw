// Package relay provides NAT-traversal connectivity between the control plane
// and fleet nodes. Nodes make outbound connections to the relay server,
// eliminating the need for port forwarding, VPNs, or direct SSH access.
//
// Architecture:
//
//	[Control Plane / CLI / Chat] ─── gRPC ───► [Relay Server] ◄─── gRPC ─── [Node Agent]
//	                                              │                              │
//	                                         mTLS + auth                   outbound only
//	                                         session mgmt                  auto-reconnect
//	                                         multiplexing                  heartbeat
//
// The relay server runs as a standalone service or embedded in the control plane.
// Node agents connect outbound (NAT-friendly) and maintain persistent streams.
// Commands are forwarded through the relay to target nodes.
package relay

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/freitascorp/devopsclaw/pkg/fleet"
)

// ------------------------------------------------------------------
// Relay Server
// ------------------------------------------------------------------

// ServerConfig configures the relay server.
type ServerConfig struct {
	ListenAddr string        `json:"listen_addr"` // e.g., ":9443"
	TLSConfig  *tls.Config   `json:"tls_config,omitempty"`
	AuthToken  string        `json:"auth_token"` // shared secret for node auth (legacy, prefer mTLS)
	MaxNodes   int           `json:"max_nodes"`
	PingInterval time.Duration `json:"ping_interval"`
	MTLS       *MTLSConfig   `json:"mtls,omitempty"` // mTLS config (replaces AuthToken)
}

// Server is the relay server that brokers connections between the
// control plane and fleet nodes.
type Server struct {
	config  ServerConfig
	logger  *slog.Logger

	mu       sync.RWMutex
	tunnels  map[fleet.NodeID]*Tunnel
	store    fleet.Store
}

// Tunnel represents a persistent connection from a node to the relay.
type Tunnel struct {
	NodeID     fleet.NodeID
	ConnectedAt time.Time
	LastPing   time.Time
	RemoteAddr string
	CommandCh  chan *CommandEnvelope  // commands to send to node
	ResultCh   chan *ResultEnvelope   // results from node
	Done       chan struct{}
}

// CommandEnvelope wraps a command with routing metadata.
type CommandEnvelope struct {
	RequestID string
	Command   fleet.TypedCommand
	Deadline  time.Time
}

// ResultEnvelope wraps a result with routing metadata.
type ResultEnvelope struct {
	RequestID string
	Result    fleet.NodeResult
}

// NewServer creates a relay server.
func NewServer(config ServerConfig, store fleet.Store, logger *slog.Logger) *Server {
	if config.MaxNodes <= 0 {
		config.MaxNodes = 1000
	}
	if config.PingInterval <= 0 {
		config.PingInterval = 15 * time.Second
	}
	return &Server{
		config:  config,
		logger:  logger,
		tunnels: make(map[fleet.NodeID]*Tunnel),
		store:   store,
	}
}

// RegisterTunnel is called when a node agent connects.
func (s *Server) RegisterTunnel(nodeID fleet.NodeID, remoteAddr string) (*Tunnel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.tunnels) >= s.config.MaxNodes {
		return nil, fmt.Errorf("max nodes (%d) reached", s.config.MaxNodes)
	}

	// Close existing tunnel if reconnecting
	if existing, ok := s.tunnels[nodeID]; ok {
		close(existing.Done)
		s.logger.Info("closing stale tunnel for reconnecting node", "node_id", nodeID)
	}

	tunnel := &Tunnel{
		NodeID:      nodeID,
		ConnectedAt: time.Now(),
		LastPing:    time.Now(),
		RemoteAddr:  remoteAddr,
		CommandCh:   make(chan *CommandEnvelope, 32),
		ResultCh:    make(chan *ResultEnvelope, 32),
		Done:        make(chan struct{}),
	}
	s.tunnels[nodeID] = tunnel

	s.logger.Info("tunnel registered", "node_id", nodeID, "remote_addr", remoteAddr)
	return tunnel, nil
}

// DeregisterTunnel removes a node's tunnel.
func (s *Server) DeregisterTunnel(nodeID fleet.NodeID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tunnels[nodeID]; ok {
		close(t.Done)
		delete(s.tunnels, nodeID)
		s.logger.Info("tunnel deregistered", "node_id", nodeID)
	}
}

// SendCommand routes a command to a node through its tunnel.
func (s *Server) SendCommand(ctx context.Context, nodeID fleet.NodeID, env *CommandEnvelope) (*ResultEnvelope, error) {
	s.mu.RLock()
	tunnel, ok := s.tunnels[nodeID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no tunnel for node %s", nodeID)
	}

	// Send command
	select {
	case tunnel.CommandCh <- env:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-tunnel.Done:
		return nil, fmt.Errorf("tunnel closed for node %s", nodeID)
	}

	// Wait for result
	select {
	case result := <-tunnel.ResultCh:
		if result.RequestID != env.RequestID {
			return nil, fmt.Errorf("result mismatch: expected %s, got %s", env.RequestID, result.RequestID)
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-tunnel.Done:
		return nil, fmt.Errorf("tunnel closed while waiting for result from node %s", nodeID)
	}
}

// ConnectedNodes returns the list of currently connected node IDs.
func (s *Server) ConnectedNodes() []fleet.NodeID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]fleet.NodeID, 0, len(s.tunnels))
	for id := range s.tunnels {
		ids = append(ids, id)
	}
	return ids
}

// ------------------------------------------------------------------
// Relay Client (runs on each fleet node)
// ------------------------------------------------------------------

// AgentConfig configures the node-side relay agent.
type AgentConfig struct {
	RelayAddr    string        `json:"relay_addr"` // e.g., "relay.example.com:9443"
	NodeID       fleet.NodeID  `json:"node_id"`
	AuthToken    string        `json:"auth_token"` // legacy, prefer mTLS
	TLSConfig    *tls.Config   `json:"tls_config,omitempty"`
	MTLS         *MTLSConfig   `json:"mtls,omitempty"` // mTLS config (replaces AuthToken)
	ReconnectInterval time.Duration `json:"reconnect_interval"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
}

// Agent runs on each fleet node, maintaining an outbound connection to the relay.
type Agent struct {
	config   AgentConfig
	logger   *slog.Logger
	executor LocalExecutor

	mu        sync.RWMutex
	connected bool
	stopCh    chan struct{}
}

// LocalExecutor executes commands locally on the fleet node.
type LocalExecutor interface {
	Execute(ctx context.Context, cmd fleet.TypedCommand) (*fleet.NodeResult, error)
}

// NewAgent creates a node relay agent.
func NewAgent(config AgentConfig, executor LocalExecutor, logger *slog.Logger) *Agent {
	if config.ReconnectInterval <= 0 {
		config.ReconnectInterval = 5 * time.Second
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	return &Agent{
		config:   config,
		logger:   logger,
		executor: executor,
		stopCh:   make(chan struct{}),
	}
}

// Run connects to the relay and processes commands. It reconnects automatically.
// This is the main loop for a fleet node agent.
func (a *Agent) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.stopCh:
			return nil
		default:
		}

		err := a.connectAndServe(ctx)
		if err != nil {
			a.logger.Error("relay connection failed, reconnecting",
				"error", err,
				"retry_in", a.config.ReconnectInterval,
			)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.stopCh:
			return nil
		case <-time.After(a.config.ReconnectInterval):
			// retry
		}
	}
}

// Stop gracefully stops the agent.
func (a *Agent) Stop() {
	close(a.stopCh)
}

// IsConnected returns whether the agent has an active relay connection.
func (a *Agent) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected
}

func (a *Agent) connectAndServe(ctx context.Context) error {
	a.logger.Info("connecting to relay", "addr", a.config.RelayAddr, "node_id", a.config.NodeID)

	// In a real implementation, this would:
	// 1. Establish a gRPC/WebSocket connection to the relay server
	// 2. Authenticate with auth token or mTLS certificate
	// 3. Register the node and its capabilities
	// 4. Enter a loop: receive commands, execute locally, send results back
	// 5. Send periodic heartbeats
	//
	// For now, this is the structural skeleton.

	a.mu.Lock()
	a.connected = true
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.connected = false
		a.mu.Unlock()
	}()

	a.logger.Info("connected to relay", "node_id", a.config.NodeID)

	// Heartbeat loop
	heartbeat := time.NewTicker(a.config.HeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.stopCh:
			return nil
		case <-heartbeat.C:
			a.logger.Debug("sending heartbeat", "node_id", a.config.NodeID)
			// In real implementation: send heartbeat with resource snapshot
		}
	}
}

// ------------------------------------------------------------------
// TunnelRelayClient implements fleet.RelayClient using the relay server
// ------------------------------------------------------------------

// TunnelRelayClient routes commands through the relay server.
type TunnelRelayClient struct {
	server *Server
	logger *slog.Logger
}

// NewTunnelRelayClient creates a relay-based fleet client.
func NewTunnelRelayClient(server *Server, logger *slog.Logger) *TunnelRelayClient {
	return &TunnelRelayClient{server: server, logger: logger}
}

// Execute sends a command to a node through the relay tunnel.
func (c *TunnelRelayClient) Execute(ctx context.Context, node *fleet.Node, cmd fleet.TypedCommand) (*fleet.NodeResult, error) {
	env := &CommandEnvelope{
		RequestID: fmt.Sprintf("cmd-%d", time.Now().UnixNano()),
		Command:   cmd,
		Deadline:  time.Now().Add(30 * time.Second),
	}

	result, err := c.server.SendCommand(ctx, node.ID, env)
	if err != nil {
		return nil, err
	}

	return &result.Result, nil
}

// Ping checks if a node is reachable through the relay.
func (c *TunnelRelayClient) Ping(ctx context.Context, node *fleet.Node) error {
	c.server.mu.RLock()
	_, ok := c.server.tunnels[node.ID]
	c.server.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no active tunnel for node %s", node.ID)
	}
	return nil
}
