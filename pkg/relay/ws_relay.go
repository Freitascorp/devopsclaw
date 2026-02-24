package relay

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/freitascorp/devopsclaw/pkg/fleet"
)

// ------------------------------------------------------------------
// WebSocket Relay Server — real networking
// ------------------------------------------------------------------

// WSServer is a WebSocket-based relay server that brokers connections
// between the control plane (CLI/SDK) and fleet node agents.
// Nodes connect outbound via WSS — no inbound ports required on nodes.
type WSServer struct {
	config  ServerConfig
	logger  *slog.Logger
	store   fleet.Store

	mu       sync.RWMutex
	tunnels  map[fleet.NodeID]*WSTunnel
	httpSrv  *http.Server
}

// WSTunnel is a WebSocket connection from a node agent to the relay.
type WSTunnel struct {
	NodeID     fleet.NodeID
	Conn       *websocket.Conn
	ConnectedAt time.Time
	LastPing   time.Time
	RemoteAddr string

	mu        sync.Mutex
	pending   map[string]chan *ResultEnvelope // requestID → result channel
}

// WSMessage is the wire format for relay messages.
type WSMessage struct {
	Type      string          `json:"type"` // "register", "command", "result", "ping", "pong", "error"
	RequestID string          `json:"request_id,omitempty"`
	NodeID    string          `json:"node_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     string          `json:"error,omitempty"`
	Timestamp time.Time       `json:"ts"`
}

// NewWSServer creates a WebSocket relay server.
func NewWSServer(config ServerConfig, store fleet.Store, logger *slog.Logger) *WSServer {
	if config.MaxNodes <= 0 {
		config.MaxNodes = 1000
	}
	if config.PingInterval <= 0 {
		config.PingInterval = 15 * time.Second
	}
	return &WSServer{
		config:  config,
		logger:  logger,
		store:   store,
		tunnels: make(map[fleet.NodeID]*WSTunnel),
	}
}

// buildMux creates the HTTP mux with relay routes.
func (s *WSServer) buildMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/relay/agent", s.handleAgentConnect)
	mux.HandleFunc("/relay/health", s.handleHealth)
	return mux
}

// Start starts the WebSocket relay server.
func (s *WSServer) Start(ctx context.Context) error {
	mux := s.buildMux()

	s.httpSrv = &http.Server{
		Addr:    s.config.ListenAddr,
		Handler: mux,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	s.logger.Info("relay server starting", "addr", s.config.ListenAddr)

	// Start ping monitor
	go s.pingLoop(ctx)

	var err error

	// Priority: mTLS config → explicit TLSConfig → plain HTTP
	if s.config.MTLS != nil && s.config.MTLS.CACertFile != "" {
		// Build mTLS config from certificate files
		tlsCfg, tlsErr := ServerTLSConfig(*s.config.MTLS)
		if tlsErr != nil {
			return fmt.Errorf("mTLS setup: %w", tlsErr)
		}
		s.httpSrv.TLSConfig = tlsCfg
		s.logger.Info("relay server using mTLS",
			"ca_cert", s.config.MTLS.CACertFile,
			"require_client_cert", s.config.MTLS.RequireClientCert,
		)
		listener, lisErr := tls.Listen("tcp", s.config.ListenAddr, tlsCfg)
		if lisErr != nil {
			return lisErr
		}
		err = s.httpSrv.Serve(listener)
	} else if s.config.TLSConfig != nil {
		s.logger.Info("relay server using TLS (server-only, no mTLS)")
		listener, lisErr := tls.Listen("tcp", s.config.ListenAddr, s.config.TLSConfig)
		if lisErr != nil {
			return lisErr
		}
		err = s.httpSrv.Serve(listener)
	} else {
		// Warn when TLS is not configured for non-localhost addresses
		if !strings.HasPrefix(s.config.ListenAddr, "127.0.0.1") && !strings.HasPrefix(s.config.ListenAddr, "localhost") {
			s.logger.Warn("relay server starting WITHOUT TLS on non-localhost address — use TLS or mTLS in production",
				"addr", s.config.ListenAddr)
		}
		err = s.httpSrv.ListenAndServe()
	}

	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Stop gracefully shuts down the relay server.
func (s *WSServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	for _, t := range s.tunnels {
		t.Conn.Close(websocket.StatusGoingAway, "server shutting down")
	}
	s.tunnels = make(map[fleet.NodeID]*WSTunnel)
	s.mu.Unlock()

	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

// handleAgentConnect handles WebSocket upgrade for node agents.
func (s *WSServer) handleAgentConnect(w http.ResponseWriter, r *http.Request) {
	// --- Authentication ---
	// Prefer mTLS: if the connection has a verified client certificate, extract
	// the node identity from the cert's CN. This eliminates shared secrets.
	var mtlsIdentity *ClientIdentity
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		id, err := VerifyClientCert(r.TLS)
		if err != nil {
			s.logger.Warn("mTLS client cert verification failed", "error", err, "remote", r.RemoteAddr)
			http.Error(w, "certificate verification failed", http.StatusForbidden)
			return
		}
		mtlsIdentity = id
		s.logger.Info("mTLS authenticated", "node_id", id.NodeID, "fingerprint", id.Fingerprint)
	} else if s.config.AuthToken != "" {
		// Fallback: bearer token auth (for migration or non-mTLS setups)
		token := r.Header.Get("Authorization")
		expected := "Bearer " + s.config.AuthToken
		if len(token) != len(expected) || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	} else if s.config.MTLS != nil && s.config.MTLS.RequireClientCert {
		// mTLS is configured but no cert was presented and no token fallback
		http.Error(w, "client certificate required", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: false,
	})
	if err != nil {
		s.logger.Error("websocket accept failed", "error", err)
		return
	}

	// Read registration message
	ctx := r.Context()
	var regMsg WSMessage
	if err := wsjson.Read(ctx, conn, &regMsg); err != nil {
		s.logger.Error("failed to read registration", "error", err)
		conn.Close(websocket.StatusProtocolError, "registration failed")
		return
	}

	if regMsg.Type != "register" {
		conn.Close(websocket.StatusProtocolError, "expected register message")
		return
	}

	nodeID := fleet.NodeID(regMsg.NodeID)
	if nodeID == "" {
		// If mTLS authenticated, use the cert CN as the node ID
		if mtlsIdentity != nil {
			nodeID = fleet.NodeID(mtlsIdentity.NodeID)
		} else {
			conn.Close(websocket.StatusProtocolError, "node_id required")
			return
		}
	}

	// If mTLS is present, verify the registration node_id matches the cert CN
	if mtlsIdentity != nil && string(nodeID) != mtlsIdentity.NodeID {
		s.logger.Warn("node_id mismatch with mTLS cert",
			"registration_id", nodeID,
			"cert_cn", mtlsIdentity.NodeID,
		)
		conn.Close(websocket.StatusProtocolError, "node_id does not match certificate CN")
		return
	}

	// Check capacity
	s.mu.Lock()
	if len(s.tunnels) >= s.config.MaxNodes {
		s.mu.Unlock()
		conn.Close(websocket.StatusTryAgainLater, "max nodes reached")
		return
	}

	// Close existing tunnel if reconnecting
	if existing, ok := s.tunnels[nodeID]; ok {
		existing.Conn.Close(websocket.StatusGoingAway, "reconnecting")
		s.logger.Info("replacing stale tunnel", "node_id", nodeID)
	}

	tunnel := &WSTunnel{
		NodeID:      nodeID,
		Conn:        conn,
		ConnectedAt: time.Now(),
		LastPing:    time.Now(),
		RemoteAddr:  r.RemoteAddr,
		pending:     make(map[string]chan *ResultEnvelope),
	}
	s.tunnels[nodeID] = tunnel
	s.mu.Unlock()

	s.logger.Info("agent connected",
		"node_id", nodeID,
		"remote_addr", r.RemoteAddr,
	)

	// Send ack
	wsjson.Write(ctx, conn, WSMessage{
		Type:      "registered",
		NodeID:    string(nodeID),
		Timestamp: time.Now(),
	})

	// Register node in store if applicable
	if s.store != nil {
		var regNode fleet.Node
		if regMsg.Payload != nil {
			json.Unmarshal(regMsg.Payload, &regNode)
		}
		regNode.ID = nodeID
		if regNode.Hostname == "" {
			regNode.Hostname = string(nodeID)
		}
		if regNode.Address == "" {
			regNode.Address = r.RemoteAddr // capture the agent's IP from the connection
		}
		regNode.Status = fleet.NodeStatusOnline
		regNode.LastSeen = time.Now()
		regNode.RegisteredAt = time.Now()
		regNode.TunnelID = string(nodeID)
		s.store.RegisterNode(ctx, &regNode)
	}

	// Enter message processing loop
	s.processAgentMessages(ctx, tunnel)

	// Cleanup on disconnect
	s.mu.Lock()
	if current, ok := s.tunnels[nodeID]; ok && current == tunnel {
		delete(s.tunnels, nodeID)
	}
	s.mu.Unlock()

	if s.store != nil {
		s.store.UpdateNodeStatus(context.Background(), nodeID, fleet.NodeStatusOffline)
	}

	s.logger.Info("agent disconnected", "node_id", nodeID)
}

// processAgentMessages reads results from the node agent.
func (s *WSServer) processAgentMessages(ctx context.Context, tunnel *WSTunnel) {
	for {
		var msg WSMessage
		err := wsjson.Read(ctx, tunnel.Conn, &msg)
		if err != nil {
			if websocket.CloseStatus(err) != -1 {
				s.logger.Debug("agent connection closed", "node_id", tunnel.NodeID)
			} else {
				s.logger.Error("error reading from agent", "node_id", tunnel.NodeID, "error", err)
			}
			return
		}

		switch msg.Type {
		case "result":
			var result fleet.NodeResult
			if msg.Payload != nil {
				json.Unmarshal(msg.Payload, &result)
			}
			tunnel.mu.Lock()
			if ch, ok := tunnel.pending[msg.RequestID]; ok {
				ch <- &ResultEnvelope{RequestID: msg.RequestID, Result: result}
				delete(tunnel.pending, msg.RequestID)
			}
			tunnel.mu.Unlock()

		case "pong":
			tunnel.LastPing = time.Now()
			if s.store != nil {
				s.store.UpdateNodeHeartbeat(ctx, tunnel.NodeID, fleet.NodeResources{})
			}

		default:
			s.logger.Debug("unknown message type from agent", "type", msg.Type, "node_id", tunnel.NodeID)
		}
	}
}

// SendCommandWS sends a command to a node through its WebSocket tunnel.
func (s *WSServer) SendCommandWS(ctx context.Context, nodeID fleet.NodeID, env *CommandEnvelope) (*ResultEnvelope, error) {
	s.mu.RLock()
	tunnel, ok := s.tunnels[nodeID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no active tunnel for node %s", nodeID)
	}

	// Create result channel
	resultCh := make(chan *ResultEnvelope, 1)
	tunnel.mu.Lock()
	tunnel.pending[env.RequestID] = resultCh
	tunnel.mu.Unlock()

	// Send command
	payload, _ := json.Marshal(env.Command)
	msg := WSMessage{
		Type:      "command",
		RequestID: env.RequestID,
		NodeID:    string(nodeID),
		Payload:   payload,
		Timestamp: time.Now(),
	}

	if err := wsjson.Write(ctx, tunnel.Conn, msg); err != nil {
		tunnel.mu.Lock()
		delete(tunnel.pending, env.RequestID)
		tunnel.mu.Unlock()
		return nil, fmt.Errorf("send command to %s: %w", nodeID, err)
	}

	// Wait for result
	select {
	case result := <-resultCh:
		return result, nil
	case <-ctx.Done():
		tunnel.mu.Lock()
		delete(tunnel.pending, env.RequestID)
		tunnel.mu.Unlock()
		return nil, ctx.Err()
	}
}

// ConnectedNodeIDs returns the list of currently connected node IDs.
func (s *WSServer) ConnectedNodeIDs() []fleet.NodeID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]fleet.NodeID, 0, len(s.tunnels))
	for id := range s.tunnels {
		ids = append(ids, id)
	}
	return ids
}

// handleHealth responds to health check requests.
func (s *WSServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	count := len(s.tunnels)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":          "ok",
		"connected_nodes": count,
		"max_nodes":       s.config.MaxNodes,
		"timestamp":       time.Now(),
	})
}

// pingLoop sends periodic pings to all connected agents.
func (s *WSServer) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.RLock()
			for nodeID, tunnel := range s.tunnels {
				msg := WSMessage{Type: "ping", Timestamp: time.Now()}
				if err := wsjson.Write(ctx, tunnel.Conn, msg); err != nil {
					s.logger.Warn("ping failed", "node_id", nodeID, "error", err)
				}
			}
			s.mu.RUnlock()
		}
	}
}

// ------------------------------------------------------------------
// WSRelayClient implements fleet.RelayClient using the WS relay server
// ------------------------------------------------------------------

// WSRelayClient routes commands through the WebSocket relay server.
type WSRelayClient struct {
	server *WSServer
	logger *slog.Logger
}

// NewWSRelayClient creates a relay client backed by the WS server.
func NewWSRelayClient(server *WSServer, logger *slog.Logger) *WSRelayClient {
	return &WSRelayClient{server: server, logger: logger}
}

// Execute sends a command to a node through the relay tunnel.
func (c *WSRelayClient) Execute(ctx context.Context, node *fleet.Node, cmd fleet.TypedCommand) (*fleet.NodeResult, error) {
	env := &CommandEnvelope{
		RequestID: fmt.Sprintf("cmd-%d", time.Now().UnixNano()),
		Command:   cmd,
		Deadline:  time.Now().Add(30 * time.Second),
	}

	result, err := c.server.SendCommandWS(ctx, node.ID, env)
	if err != nil {
		return nil, err
	}
	return &result.Result, nil
}

// Ping checks if a node is connected to the relay.
func (c *WSRelayClient) Ping(ctx context.Context, node *fleet.Node) error {
	s := c.server
	s.mu.RLock()
	_, ok := s.tunnels[node.ID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no active tunnel for node %s", node.ID)
	}
	return nil
}

// ------------------------------------------------------------------
// WebSocket Agent Client — runs on each fleet node
// ------------------------------------------------------------------

// WSAgent runs on each fleet node, connecting outbound to the relay server.
type WSAgent struct {
	config   AgentConfig
	logger   *slog.Logger
	executor LocalExecutor

	mu        sync.RWMutex
	connected bool
	stopCh    chan struct{}
}

// NewWSAgent creates a WebSocket-based node agent.
func NewWSAgent(config AgentConfig, executor LocalExecutor, logger *slog.Logger) *WSAgent {
	if config.ReconnectInterval <= 0 {
		config.ReconnectInterval = 5 * time.Second
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	return &WSAgent{
		config:   config,
		logger:   logger,
		executor: executor,
		stopCh:   make(chan struct{}),
	}
}

// Run connects to the relay and processes commands with automatic reconnection.
func (a *WSAgent) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.stopCh:
			return nil
		default:
		}

		err := a.connectAndServeWS(ctx)
		if err != nil {
			a.logger.Error("relay connection lost, reconnecting",
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
		}
	}
}

// Stop gracefully stops the agent.
func (a *WSAgent) Stop() {
	close(a.stopCh)
}

// IsConnected returns whether the agent has an active relay connection.
func (a *WSAgent) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected
}

func (a *WSAgent) connectAndServeWS(ctx context.Context) error {
	a.logger.Info("connecting to relay", "addr", a.config.RelayAddr, "node_id", a.config.NodeID)

	// Build WebSocket URL
	wsURL := a.config.RelayAddr
	if !strings.HasPrefix(wsURL, "ws://") && !strings.HasPrefix(wsURL, "wss://") {
		wsURL = "wss://" + wsURL
	}
	if !strings.Contains(wsURL, "/relay/agent") {
		wsURL += "/relay/agent"
	}

	// Connect
	dialOpts := &websocket.DialOptions{}

	// Prefer mTLS client config if available
	if a.config.MTLS != nil && a.config.MTLS.ClientCertFile != "" {
		tlsCfg, tlsErr := ClientTLSConfig(*a.config.MTLS)
		if tlsErr != nil {
			return fmt.Errorf("mTLS client setup: %w", tlsErr)
		}
		dialOpts.HTTPClient = &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		}
		a.logger.Info("using mTLS authentication", "cert", a.config.MTLS.ClientCertFile)
	} else if a.config.TLSConfig != nil {
		dialOpts.HTTPClient = &http.Client{
			Transport: &http.Transport{TLSClientConfig: a.config.TLSConfig},
		}
	}

	// Bearer token auth (legacy fallback, used when mTLS is not configured)
	if a.config.AuthToken != "" {
		dialOpts.HTTPHeader = http.Header{
			"Authorization": []string{"Bearer " + a.config.AuthToken},
		}
	}

	conn, _, err := websocket.Dial(ctx, wsURL, dialOpts)
	if err != nil {
		return fmt.Errorf("dial relay: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "agent stopping")

	// Send registration — include hostname and local address for fleet visibility
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = string(a.config.NodeID)
	}
	regPayload, _ := json.Marshal(map[string]any{
		"hostname":     hostname,
		"capabilities": []string{"shell", "file"},
	})
	regMsg := WSMessage{
		Type:      "register",
		NodeID:    string(a.config.NodeID),
		Payload:   regPayload,
		Timestamp: time.Now(),
	}
	if err := wsjson.Write(ctx, conn, regMsg); err != nil {
		return fmt.Errorf("send registration: %w", err)
	}

	// Read ack
	var ackMsg WSMessage
	if err := wsjson.Read(ctx, conn, &ackMsg); err != nil {
		return fmt.Errorf("read registration ack: %w", err)
	}
	if ackMsg.Type != "registered" {
		return fmt.Errorf("unexpected ack type: %s", ackMsg.Type)
	}

	a.mu.Lock()
	a.connected = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.connected = false
		a.mu.Unlock()
	}()

	a.logger.Info("connected to relay", "node_id", a.config.NodeID)

	// Process messages
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.processRelayMessages(ctx, conn)
	}()

	// Heartbeat loop
	heartbeat := time.NewTicker(a.config.HeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.stopCh:
			return nil
		case err := <-errCh:
			return err
		case <-heartbeat.C:
			pong := WSMessage{Type: "pong", NodeID: string(a.config.NodeID), Timestamp: time.Now()}
			if err := wsjson.Write(ctx, conn, pong); err != nil {
				return fmt.Errorf("send heartbeat: %w", err)
			}
		}
	}
}

func (a *WSAgent) processRelayMessages(ctx context.Context, conn *websocket.Conn) error {
	for {
		var msg WSMessage
		err := wsjson.Read(ctx, conn, &msg)
		if err != nil {
			return err
		}

		switch msg.Type {
		case "command":
			go a.handleCommand(ctx, conn, msg)
		case "ping":
			pong := WSMessage{Type: "pong", NodeID: string(a.config.NodeID), Timestamp: time.Now()}
			wsjson.Write(ctx, conn, pong)
		default:
			a.logger.Debug("unknown message from relay", "type", msg.Type)
		}
	}
}

func (a *WSAgent) handleCommand(ctx context.Context, conn *websocket.Conn, msg WSMessage) {
	var cmd fleet.TypedCommand
	if err := json.Unmarshal(msg.Payload, &cmd); err != nil {
		errMsg := WSMessage{
			Type:      "result",
			RequestID: msg.RequestID,
			NodeID:    string(a.config.NodeID),
			Error:     fmt.Sprintf("unmarshal command: %v", err),
			Timestamp: time.Now(),
		}
		wsjson.Write(ctx, conn, errMsg)
		return
	}

	result, err := a.executor.Execute(ctx, cmd)
	if err != nil {
		result = &fleet.NodeResult{
			NodeID: a.config.NodeID,
			Error:  err.Error(),
			Status: "failure",
		}
	}

	payload, _ := json.Marshal(result)
	resultMsg := WSMessage{
		Type:      "result",
		RequestID: msg.RequestID,
		NodeID:    string(a.config.NodeID),
		Payload:   payload,
		Timestamp: time.Now(),
	}

	wsjson.Write(ctx, conn, resultMsg)
}
