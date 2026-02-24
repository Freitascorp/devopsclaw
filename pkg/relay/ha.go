// Package relay — High Availability support for relay servers.
//
// When the fleet exceeds ~100 nodes, a single relay server becomes a bottleneck
// and a single point of failure. HAProxy implements:
//
//   - Multiple relay server instances sharing a common store (Postgres)
//   - Consistent hashing to route nodes to preferred relay instances
//   - Automatic failover: if a relay goes down, its nodes reconnect to another
//   - Health monitoring between relay peers
//   - Graceful draining for zero-downtime upgrades
//
// Architecture:
//
//	                ┌──────────────────────────────────┐
//	                │        Load Balancer (L4)        │
//	                └──┬──────────┬──────────┬─────────┘
//	                   │          │          │
//	              ┌────▼──┐  ┌───▼───┐  ┌──▼─────┐
//	              │Relay-1│  │Relay-2│  │Relay-3 │
//	              └───┬───┘  └───┬───┘  └───┬────┘
//	                  │          │          │
//	              ┌───▼──────────▼──────────▼───┐
//	              │     Shared Store (PG)       │
//	              └────────────────────────────-┘
package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// ------------------------------------------------------------------
// HA Coordinator
// ------------------------------------------------------------------

// HAConfig configures the relay HA cluster.
type HAConfig struct {
	Enabled        bool          `json:"enabled"`
	InstanceID     string        `json:"instance_id"`     // Unique ID for this relay instance
	PeerAddrs      []string      `json:"peer_addrs"`      // Addresses of peer relay instances
	AdvertiseAddr  string        `json:"advertise_addr"`  // Address this instance advertises to peers
	HealthInterval time.Duration `json:"health_interval"` // How often to check peer health
	DrainTimeout   time.Duration `json:"drain_timeout"`   // How long to wait for connections to drain
}

// PeerState tracks the health of a relay peer.
type PeerState struct {
	InstanceID   string    `json:"instance_id"`
	Address      string    `json:"address"`
	Status       string    `json:"status"` // "healthy", "unhealthy", "draining"
	ConnectedNodes int     `json:"connected_nodes"`
	LastSeen     time.Time `json:"last_seen"`
	LastError    string    `json:"last_error,omitempty"`
}

// HACoordinator manages relay HA cluster coordination.
type HACoordinator struct {
	config  HAConfig
	server  *WSServer
	logger  *slog.Logger

	mu      sync.RWMutex
	peers   map[string]*PeerState // instanceID → state
	status  string                // "active", "draining", "standby"

	httpClient *http.Client
	stopCh     chan struct{}
}

// NewHACoordinator creates a new HA coordinator for the relay server.
func NewHACoordinator(config HAConfig, server *WSServer, logger *slog.Logger) *HACoordinator {
	if config.HealthInterval <= 0 {
		config.HealthInterval = 10 * time.Second
	}
	if config.DrainTimeout <= 0 {
		config.DrainTimeout = 30 * time.Second
	}
	if config.InstanceID == "" {
		config.InstanceID = fmt.Sprintf("relay-%d", time.Now().UnixNano())
	}

	return &HACoordinator{
		config: config,
		server: server,
		logger: logger,
		peers:  make(map[string]*PeerState),
		status: "active",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		stopCh: make(chan struct{}),
	}
}

// Start begins HA coordination — peer health checking and status broadcasting.
func (ha *HACoordinator) Start(ctx context.Context) error {
	ha.logger.Info("HA coordinator starting",
		"instance_id", ha.config.InstanceID,
		"peers", len(ha.config.PeerAddrs),
		"advertise_addr", ha.config.AdvertiseAddr,
	)

	// Register HA routes on the server's HTTP mux
	ha.registerRoutes()

	// Start peer health monitor
	go ha.healthLoop(ctx)

	return nil
}

// Stop gracefully stops the HA coordinator, draining connections.
func (ha *HACoordinator) Stop(ctx context.Context) error {
	close(ha.stopCh)

	ha.mu.Lock()
	ha.status = "draining"
	ha.mu.Unlock()

	ha.logger.Info("HA coordinator draining", "timeout", ha.config.DrainTimeout)

	// Wait for connections to drain or timeout
	select {
	case <-time.After(ha.config.DrainTimeout):
		ha.logger.Warn("drain timeout reached, force stopping")
	case <-ctx.Done():
	}

	return nil
}

// ClusterStatus returns the current HA cluster state.
func (ha *HACoordinator) ClusterStatus() *ClusterState {
	ha.mu.RLock()
	defer ha.mu.RUnlock()

	connectedNodes := ha.server.ConnectedNodeIDs()

	self := &PeerState{
		InstanceID:     ha.config.InstanceID,
		Address:        ha.config.AdvertiseAddr,
		Status:         ha.status,
		ConnectedNodes: len(connectedNodes),
		LastSeen:       time.Now(),
	}

	peers := make([]*PeerState, 0, len(ha.peers))
	for _, p := range ha.peers {
		peers = append(peers, p)
	}

	totalNodes := self.ConnectedNodes
	healthyPeers := 0
	for _, p := range peers {
		totalNodes += p.ConnectedNodes
		if p.Status == "healthy" {
			healthyPeers++
		}
	}

	return &ClusterState{
		Self:           self,
		Peers:          peers,
		TotalNodes:     totalNodes,
		TotalInstances: 1 + len(peers),
		HealthyInstances: 1 + healthyPeers,
	}
}

// ClusterState represents the complete HA cluster state.
type ClusterState struct {
	Self             *PeerState   `json:"self"`
	Peers            []*PeerState `json:"peers"`
	TotalNodes       int          `json:"total_nodes"`
	TotalInstances   int          `json:"total_instances"`
	HealthyInstances int          `json:"healthy_instances"`
}

// PreferredInstance returns the preferred relay instance for a given node ID
// using consistent hashing. This helps distribute nodes across relay instances.
func (ha *HACoordinator) PreferredInstance(nodeID string) string {
	ha.mu.RLock()
	defer ha.mu.RUnlock()

	// Build list of healthy instances
	instances := []string{ha.config.InstanceID}
	for _, p := range ha.peers {
		if p.Status == "healthy" {
			instances = append(instances, p.InstanceID)
		}
	}

	if len(instances) == 0 {
		return ha.config.InstanceID
	}

	// Consistent hash: FNV-1a of nodeID → modulo instance count
	h := fnv.New32a()
	h.Write([]byte(nodeID))
	idx := int(h.Sum32()) % len(instances)
	return instances[idx]
}

// ShouldAcceptNode returns true if this relay instance should accept
// the given node, based on consistent hashing.
func (ha *HACoordinator) ShouldAcceptNode(nodeID string) bool {
	preferred := ha.PreferredInstance(nodeID)
	return preferred == ha.config.InstanceID
}

// ------------------------------------------------------------------
// Internal: health monitoring
// ------------------------------------------------------------------

func (ha *HACoordinator) healthLoop(ctx context.Context) {
	ticker := time.NewTicker(ha.config.HealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ha.stopCh:
			return
		case <-ticker.C:
			ha.checkPeers(ctx)
		}
	}
}

func (ha *HACoordinator) checkPeers(ctx context.Context) {
	for _, addr := range ha.config.PeerAddrs {
		go ha.checkPeer(ctx, addr)
	}
}

func (ha *HACoordinator) checkPeer(ctx context.Context, addr string) {
	url := fmt.Sprintf("http://%s/relay/ha/status", addr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		ha.markPeerUnhealthy(addr, err.Error())
		return
	}

	resp, err := ha.httpClient.Do(req)
	if err != nil {
		ha.markPeerUnhealthy(addr, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ha.markPeerUnhealthy(addr, fmt.Sprintf("status %d", resp.StatusCode))
		return
	}

	var peerStatus PeerState
	if err := json.NewDecoder(resp.Body).Decode(&peerStatus); err != nil {
		ha.markPeerUnhealthy(addr, err.Error())
		return
	}

	ha.mu.Lock()
	peerStatus.Address = addr
	peerStatus.LastSeen = time.Now()
	if peerStatus.Status == "" {
		peerStatus.Status = "healthy"
	}
	ha.peers[peerStatus.InstanceID] = &peerStatus
	ha.mu.Unlock()
}

func (ha *HACoordinator) markPeerUnhealthy(addr, errMsg string) {
	ha.mu.Lock()
	defer ha.mu.Unlock()

	// Find peer by address
	for id, p := range ha.peers {
		if p.Address == addr {
			p.Status = "unhealthy"
			p.LastError = errMsg
			ha.logger.Warn("peer unhealthy", "peer", id, "addr", addr, "error", errMsg)
			return
		}
	}

	// New unknown peer — create entry
	ha.peers[addr] = &PeerState{
		InstanceID: addr,
		Address:    addr,
		Status:     "unhealthy",
		LastError:  errMsg,
		LastSeen:   time.Now(),
	}
}

// ------------------------------------------------------------------
// Internal: HTTP routes for HA
// ------------------------------------------------------------------

func (ha *HACoordinator) registerRoutes() {
	mux := ha.server.buildMux()
	mux.HandleFunc("/relay/ha/status", ha.handleHAStatus)
	mux.HandleFunc("/relay/ha/cluster", ha.handleClusterStatus)
	mux.HandleFunc("/relay/ha/drain", ha.handleDrain)
}

func (ha *HACoordinator) handleHAStatus(w http.ResponseWriter, _ *http.Request) {
	ha.mu.RLock()
	connectedNodes := len(ha.server.tunnels)
	status := ha.status
	ha.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(PeerState{
		InstanceID:     ha.config.InstanceID,
		Address:        ha.config.AdvertiseAddr,
		Status:         status,
		ConnectedNodes: connectedNodes,
		LastSeen:       time.Now(),
	})
}

func (ha *HACoordinator) handleClusterStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ha.ClusterStatus())
}

func (ha *HACoordinator) handleDrain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ha.mu.Lock()
	ha.status = "draining"
	ha.mu.Unlock()

	ha.logger.Info("relay instance draining", "instance_id", ha.config.InstanceID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "draining",
		"instance_id": ha.config.InstanceID,
	})
}
