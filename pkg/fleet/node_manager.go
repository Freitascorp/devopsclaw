package fleet

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// NodeManager handles node registration, health tracking, and group management.
type NodeManager struct {
	store  Store
	logger *slog.Logger

	mu         sync.RWMutex
	watchers   []NodeWatcher
	gcInterval time.Duration // how often to check for stale nodes
	gcTimeout  time.Duration // consider offline after this silence
}

// NodeWatcher receives node lifecycle events.
type NodeWatcher interface {
	OnNodeRegistered(node *Node)
	OnNodeDeregistered(id NodeID)
	OnNodeStatusChanged(id NodeID, old, new NodeStatus)
}

// NewNodeManager creates a fleet node manager.
func NewNodeManager(store Store, logger *slog.Logger) *NodeManager {
	return &NodeManager{
		store:      store,
		logger:     logger,
		gcInterval: 30 * time.Second,
		gcTimeout:  2 * time.Minute,
	}
}

// Register adds a node to the fleet roster.
func (nm *NodeManager) Register(ctx context.Context, node *Node) error {
	node.RegisteredAt = time.Now()
	node.LastSeen = time.Now()
	if node.Status == "" {
		node.Status = NodeStatusOnline
	}
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	if err := nm.store.RegisterNode(ctx, node); err != nil {
		return fmt.Errorf("failed to register node %s: %w", node.ID, err)
	}

	nm.logger.Info("node registered",
		"node_id", node.ID,
		"hostname", node.Hostname,
		"groups", node.Groups,
		"capabilities", node.Capabilities,
	)

	nm.mu.RLock()
	for _, w := range nm.watchers {
		w.OnNodeRegistered(node)
	}
	nm.mu.RUnlock()
	return nil
}

// Deregister removes a node from the fleet.
func (nm *NodeManager) Deregister(ctx context.Context, id NodeID) error {
	if err := nm.store.DeregisterNode(ctx, id); err != nil {
		return fmt.Errorf("failed to deregister node %s: %w", id, err)
	}

	nm.logger.Info("node deregistered", "node_id", id)
	nm.mu.RLock()
	for _, w := range nm.watchers {
		w.OnNodeDeregistered(id)
	}
	nm.mu.RUnlock()
	return nil
}

// Heartbeat updates a node's last-seen timestamp and resource snapshot.
func (nm *NodeManager) Heartbeat(ctx context.Context, id NodeID, resources NodeResources) error {
	node, err := nm.store.GetNode(ctx, id)
	if err != nil {
		return fmt.Errorf("node %s not found: %w", id, err)
	}

	oldStatus := node.Status
	if err := nm.store.UpdateNodeHeartbeat(ctx, id, resources); err != nil {
		return err
	}

	// Transition back to online if was unreachable
	if oldStatus == NodeStatusUnreachable || oldStatus == NodeStatusOffline {
		if err := nm.store.UpdateNodeStatus(ctx, id, NodeStatusOnline); err != nil {
			return err
		}
		nm.logger.Info("node back online", "node_id", id)
		nm.mu.RLock()
		for _, w := range nm.watchers {
			w.OnNodeStatusChanged(id, oldStatus, NodeStatusOnline)
		}
		nm.mu.RUnlock()
	}

	return nil
}

// Drain marks a node as draining (won't receive new commands).
func (nm *NodeManager) Drain(ctx context.Context, id NodeID) error {
	node, err := nm.store.GetNode(ctx, id)
	if err != nil {
		return err
	}
	old := node.Status
	if err := nm.store.UpdateNodeStatus(ctx, id, NodeStatusDraining); err != nil {
		return err
	}
	nm.mu.RLock()
	for _, w := range nm.watchers {
		w.OnNodeStatusChanged(id, old, NodeStatusDraining)
	}
	nm.mu.RUnlock()
	return nil
}

// AddWatcher registers a node lifecycle event listener.
func (nm *NodeManager) AddWatcher(w NodeWatcher) {
	nm.mu.Lock()
	nm.watchers = append(nm.watchers, w)
	nm.mu.Unlock()
}

// RunGC periodically marks stale nodes as unreachable.
func (nm *NodeManager) RunGC(ctx context.Context) {
	ticker := time.NewTicker(nm.gcInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nm.gcCycle(ctx)
		}
	}
}

func (nm *NodeManager) gcCycle(ctx context.Context) {
	nodes, err := nm.store.ListNodes(ctx)
	if err != nil {
		nm.logger.Error("gc: failed to list nodes", "error", err)
		return
	}

	cutoff := time.Now().Add(-nm.gcTimeout)
	for _, n := range nodes {
		if n.Status == NodeStatusOnline && n.LastSeen.Before(cutoff) {
			nm.logger.Warn("marking node unreachable", "node_id", n.ID, "last_seen", n.LastSeen)
			if err := nm.store.UpdateNodeStatus(ctx, n.ID, NodeStatusUnreachable); err != nil {
				nm.logger.Error("gc: failed to update status", "node_id", n.ID, "error", err)
				continue
			}
			nm.mu.RLock()
			for _, w := range nm.watchers {
				w.OnNodeStatusChanged(n.ID, n.Status, NodeStatusUnreachable)
			}
			nm.mu.RUnlock()
		}
	}
}

// Summary returns a quick fleet status overview.
func (nm *NodeManager) Summary(ctx context.Context) (*FleetSummary, error) {
	nodes, err := nm.store.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	summary := &FleetSummary{
		TotalNodes:  len(nodes),
		GroupCounts: make(map[GroupName]int),
	}

	for _, n := range nodes {
		switch n.Status {
		case NodeStatusOnline:
			summary.Online++
		case NodeStatusOffline:
			summary.Offline++
		case NodeStatusDegraded:
			summary.Degraded++
		case NodeStatusDraining:
			summary.Draining++
		case NodeStatusUnreachable:
			summary.Unreachable++
		}
		for _, g := range n.Groups {
			summary.GroupCounts[g]++
		}
	}

	return summary, nil
}

// FleetSummary is a quick status of the entire fleet.
type FleetSummary struct {
	TotalNodes  int                  `json:"total_nodes"`
	Online      int                  `json:"online"`
	Offline     int                  `json:"offline"`
	Degraded    int                  `json:"degraded"`
	Draining    int                  `json:"draining"`
	Unreachable int                  `json:"unreachable"`
	GroupCounts map[GroupName]int    `json:"group_counts"`
}
