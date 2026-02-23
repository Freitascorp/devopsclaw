// Package fleet provides multi-machine orchestration for DevOpsClaw.
//
// It transforms DevOpsClaw from a single-machine assistant into a production
// fleet management platform by introducing:
//   - Node registration and discovery
//   - Group targeting and label-based selection
//   - Fan-out command execution with result aggregation
//   - Health monitoring across the fleet
//   - Distributed state via a pluggable store backend
package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// NodeID is a globally unique identifier for a fleet node.
type NodeID string

// GroupName is a named collection of nodes (e.g., "prod-web", "staging-db").
type GroupName string

// NodeStatus represents the operational state of a node.
type NodeStatus string

const (
	NodeStatusOnline      NodeStatus = "online"
	NodeStatusOffline     NodeStatus = "offline"
	NodeStatusDegraded    NodeStatus = "degraded"
	NodeStatusDraining    NodeStatus = "draining"
	NodeStatusUnreachable NodeStatus = "unreachable"
)

// Node represents a registered machine in the fleet.
type Node struct {
	ID          NodeID            `json:"id"`
	Hostname    string            `json:"hostname"`
	Address     string            `json:"address"` // relay tunnel address or direct IP
	Labels      map[string]string `json:"labels"`
	Groups      []GroupName       `json:"groups"`
	Status      NodeStatus        `json:"status"`
	Capabilities []string         `json:"capabilities"` // e.g., "shell", "docker", "k8s", "browser"
	Resources   NodeResources     `json:"resources"`
	LastSeen    time.Time         `json:"last_seen"`
	RegisteredAt time.Time        `json:"registered_at"`
	Version     string            `json:"version"` // devopsclaw version on this node
	TunnelID    string            `json:"tunnel_id,omitempty"` // relay tunnel identifier
}

// NodeResources captures the resource profile of a node.
type NodeResources struct {
	CPUCores    int    `json:"cpu_cores"`
	MemoryMB    int    `json:"memory_mb"`
	DiskMB      int    `json:"disk_mb"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	GoVersion   string `json:"go_version"`
}

// TargetSelector specifies which nodes a command should execute on.
type TargetSelector struct {
	// Exactly one of these must be set.
	NodeIDs  []NodeID          `json:"node_ids,omitempty"`
	Groups   []GroupName       `json:"groups,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	All      bool              `json:"all,omitempty"`

	// Limits
	MaxConcurrency int `json:"max_concurrency,omitempty"` // 0 = unlimited
	MaxNodes       int `json:"max_nodes,omitempty"`       // 0 = all matching
}

// Resolve returns the effective node list by filtering the full roster.
func (ts *TargetSelector) Resolve(roster []*Node) []*Node {
	if ts.All {
		return filterOnline(roster, ts.MaxNodes)
	}
	var matched []*Node
	if len(ts.NodeIDs) > 0 {
		idSet := make(map[NodeID]bool, len(ts.NodeIDs))
		for _, id := range ts.NodeIDs {
			idSet[id] = true
		}
		for _, n := range roster {
			if idSet[n.ID] && (n.Status == NodeStatusOnline || n.Status == NodeStatusDegraded) {
				matched = append(matched, n)
			}
		}
	}
	if len(ts.Groups) > 0 {
		groupSet := make(map[GroupName]bool, len(ts.Groups))
		for _, g := range ts.Groups {
			groupSet[g] = true
		}
		for _, n := range roster {
			if n.Status != NodeStatusOnline {
				continue
			}
			for _, ng := range n.Groups {
				if groupSet[ng] {
					matched = append(matched, n)
					break
				}
			}
		}
	}
	if len(ts.Labels) > 0 {
		for _, n := range roster {
			if n.Status != NodeStatusOnline && !alreadyIn(matched, n) {
				continue
			}
			if matchLabels(n.Labels, ts.Labels) {
				matched = append(matched, n)
			}
		}
	}
	return dedup(filterOnline(matched, ts.MaxNodes))
}

func filterOnline(nodes []*Node, max int) []*Node {
	var out []*Node
	for _, n := range nodes {
		if n.Status == NodeStatusOnline || n.Status == NodeStatusDegraded {
			out = append(out, n)
			if max > 0 && len(out) >= max {
				break
			}
		}
	}
	return out
}

func alreadyIn(nodes []*Node, n *Node) bool {
	for _, existing := range nodes {
		if existing.ID == n.ID {
			return true
		}
	}
	return false
}

func matchLabels(nodeLabels, required map[string]string) bool {
	for k, v := range required {
		if nodeLabels[k] != v {
			return false
		}
	}
	return true
}

func dedup(nodes []*Node) []*Node {
	seen := make(map[NodeID]bool, len(nodes))
	var out []*Node
	for _, n := range nodes {
		if !seen[n.ID] {
			seen[n.ID] = true
			out = append(out, n)
		}
	}
	return out
}

// ------------------------------------------------------------------
// Command execution types
// ------------------------------------------------------------------

// ExecRequest is a typed, validated command sent to fleet nodes.
type ExecRequest struct {
	ID        string         `json:"id"`
	Target    TargetSelector `json:"target"`
	Command   TypedCommand   `json:"command"`
	Timeout   time.Duration  `json:"timeout"`
	DryRun    bool           `json:"dry_run"`
	Requester string         `json:"requester"` // user/role who initiated
	CreatedAt time.Time      `json:"created_at"`
}

// TypedCommand is a discriminated union for command types.
// Each variant has its own strongly-typed schema.
type TypedCommand struct {
	Type string          `json:"type"` // "shell", "deploy", "docker", "k8s", "file", "browser"
	Data json.RawMessage `json:"data"`
}

// ShellCommand runs a shell command on target nodes.
type ShellCommand struct {
	Command    string            `json:"command"`
	WorkDir    string            `json:"work_dir,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	TimeoutSec int              `json:"timeout_sec,omitempty"`
	Shell      string            `json:"shell,omitempty"` // default: /bin/sh
}

// DeployCommand triggers a deployment on target nodes.
type DeployCommand struct {
	Service    string `json:"service"`
	Version    string `json:"version"`
	Strategy   string `json:"strategy"` // "rolling", "blue-green", "canary"
	Replicas   int    `json:"replicas,omitempty"`
	HealthURL  string `json:"health_url,omitempty"`
	RollbackOn string `json:"rollback_on,omitempty"` // "failure", "health-check", "manual"
}

// DockerCommand manages containers on target nodes.
type DockerCommand struct {
	Action     string            `json:"action"` // "run", "stop", "restart", "logs", "ps", "pull"
	Image      string            `json:"image,omitempty"`
	Container  string            `json:"container,omitempty"`
	Ports      []string          `json:"ports,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Volumes    []string          `json:"volumes,omitempty"`
	TailLines  int               `json:"tail_lines,omitempty"`
}

// K8sCommand manages Kubernetes resources.
type K8sCommand struct {
	Action    string `json:"action"` // "apply", "delete", "get", "rollout", "scale"
	Resource  string `json:"resource"`
	Namespace string `json:"namespace,omitempty"`
	Manifest  string `json:"manifest,omitempty"` // YAML content for apply
	Replicas  int    `json:"replicas,omitempty"` // for scale
}

// BrowserCommand runs browser automation via headless Chrome/Playwright.
type BrowserCommand struct {
	Action    string            `json:"action"` // "navigate", "click", "type", "screenshot", "evaluate"
	URL       string            `json:"url,omitempty"`
	Selector  string            `json:"selector,omitempty"`
	Value     string            `json:"value,omitempty"`
	Script    string            `json:"script,omitempty"`
	WaitFor   string            `json:"wait_for,omitempty"` // CSS selector or "networkidle"
	SessionID string            `json:"session_id,omitempty"` // reuse browser session
	Cookies   map[string]string `json:"cookies,omitempty"`
}

// FileCommand manages files on target nodes.
type FileCommand struct {
	Action  string `json:"action"` // "read", "write", "append", "delete", "stat", "list"
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Mode    string `json:"mode,omitempty"` // e.g., "0644"
}

// ------------------------------------------------------------------
// Execution results
// ------------------------------------------------------------------

// ExecResult is the aggregated result of a fleet-wide command.
type ExecResult struct {
	RequestID  string        `json:"request_id"`
	NodeResults []NodeResult `json:"node_results"`
	Summary    ExecSummary   `json:"summary"`
	Duration   time.Duration `json:"duration"`
}

// NodeResult is the outcome from a single node.
type NodeResult struct {
	NodeID   NodeID        `json:"node_id"`
	Hostname string        `json:"hostname"`
	Output   string        `json:"output"`
	ExitCode int           `json:"exit_code"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
	Status   string        `json:"status"` // "success", "failure", "timeout", "skipped"
}

// ExecSummary is a quick overview of fleet execution.
type ExecSummary struct {
	Total    int `json:"total"`
	Success  int `json:"success"`
	Failed   int `json:"failed"`
	Timeout  int `json:"timeout"`
	Skipped  int `json:"skipped"`
}

// ------------------------------------------------------------------
// Store interface (pluggable backend)
// ------------------------------------------------------------------

// Store is the persistence interface for fleet state.
// Implementations: MemoryStore (dev), SQLiteStore (single-node prod),
// PostgresStore (multi-node prod), EtcdStore (distributed).
type Store interface {
	// Node management
	RegisterNode(ctx context.Context, node *Node) error
	DeregisterNode(ctx context.Context, id NodeID) error
	UpdateNodeStatus(ctx context.Context, id NodeID, status NodeStatus) error
	UpdateNodeHeartbeat(ctx context.Context, id NodeID, resources NodeResources) error
	GetNode(ctx context.Context, id NodeID) (*Node, error)
	ListNodes(ctx context.Context) ([]*Node, error)
	ListNodesByGroup(ctx context.Context, group GroupName) ([]*Node, error)
	ListNodesByLabels(ctx context.Context, labels map[string]string) ([]*Node, error)

	// Execution audit
	RecordExecution(ctx context.Context, req *ExecRequest, result *ExecResult) error
	GetExecution(ctx context.Context, id string) (*ExecRequest, *ExecResult, error)
	ListExecutions(ctx context.Context, opts ListExecOptions) ([]*ExecRequest, error)

	// Distributed locking (for leader election / concurrency control)
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (Lock, error)
}

// Lock represents a distributed lock.
type Lock interface {
	Unlock(ctx context.Context) error
	Extend(ctx context.Context, ttl time.Duration) error
}

// ListExecOptions filters execution history.
type ListExecOptions struct {
	Requester string
	Since     time.Time
	Limit     int
	Offset    int
}

// Validate checks that an ExecRequest is well-formed.
func (r *ExecRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("exec request must have an ID")
	}
	if r.Command.Type == "" {
		return fmt.Errorf("command type is required")
	}
	if r.Timeout <= 0 {
		r.Timeout = 30 * time.Second
	}
	switch r.Command.Type {
	case "shell", "deploy", "docker", "k8s", "file", "browser":
		// valid
	default:
		return fmt.Errorf("unknown command type: %s", r.Command.Type)
	}
	return nil
}
