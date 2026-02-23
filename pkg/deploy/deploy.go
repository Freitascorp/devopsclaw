// Package deploy provides deployment strategy orchestration for fleet-wide deployments.
//
// It implements rolling, canary, blue-green, all-at-once, and serial deployment
// strategies with health checking, automatic rollback, and progress tracking.
package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/freitascorp/devopsclaw/pkg/fleet"
)

// Strategy defines the deployment strategy type.
type Strategy string

const (
	StrategyRolling   Strategy = "rolling"
	StrategyCanary    Strategy = "canary"
	StrategyBlueGreen Strategy = "blue-green"
	StrategyAllAtOnce Strategy = "all-at-once"
	StrategySerial    Strategy = "serial"
)

// Spec defines a deployment specification.
type Spec struct {
	Service          string            `json:"service"`
	Version          string            `json:"version"`
	Strategy         Strategy          `json:"strategy"`
	Target           fleet.TargetSelector `json:"target"`
	HealthCheckURL   string            `json:"health_check_url,omitempty"`
	HealthTimeout    time.Duration     `json:"health_timeout,omitempty"`
	RollbackOnFail   bool              `json:"rollback_on_failure"`
	MaxUnavailable   int               `json:"max_unavailable,omitempty"`   // for rolling
	CanaryPercent    []int             `json:"canary_percent,omitempty"`    // e.g., [5, 25, 100]
	SerialDelay      time.Duration     `json:"serial_delay,omitempty"`     // for serial
	DeployCommand    string            `json:"deploy_command"`             // shell command to run
	RollbackCommand  string            `json:"rollback_command,omitempty"` // shell command for rollback
	Requester        string            `json:"requester"`
}

// State tracks deployment progress.
type State string

const (
	StatePending    State = "pending"
	StateRunning    State = "running"
	StateHealthCheck State = "health_check"
	StateRollback   State = "rollback"
	StateComplete   State = "complete"
	StateFailed     State = "failed"
)

// Result is the outcome of a deployment.
type Result struct {
	ID          string              `json:"id"`
	Spec        Spec                `json:"spec"`
	State       State               `json:"state"`
	StartedAt   time.Time           `json:"started_at"`
	FinishedAt  time.Time           `json:"finished_at,omitempty"`
	Duration    time.Duration       `json:"duration"`
	Batches     []BatchResult       `json:"batches"`
	RolledBack  bool                `json:"rolled_back"`
	Error       string              `json:"error,omitempty"`
}

// BatchResult is the outcome of a single deployment batch.
type BatchResult struct {
	BatchIndex  int                 `json:"batch_index"`
	Nodes       []fleet.NodeResult  `json:"nodes"`
	HealthOK    bool                `json:"health_ok"`
	StartedAt   time.Time           `json:"started_at"`
	FinishedAt  time.Time           `json:"finished_at"`
}

// Deployer orchestrates deployments across fleet nodes.
type Deployer struct {
	executor *fleet.Executor
	store    fleet.Store
	logger   *slog.Logger
	mu       sync.Mutex
	active   map[string]*Result // deploy ID → active result
}

// NewDeployer creates a deployment orchestrator.
func NewDeployer(executor *fleet.Executor, store fleet.Store, logger *slog.Logger) *Deployer {
	return &Deployer{
		executor: executor,
		store:    store,
		logger:   logger,
		active:   make(map[string]*Result),
	}
}

// Deploy executes a deployment according to the given spec.
func (d *Deployer) Deploy(ctx context.Context, spec Spec) (*Result, error) {
	if spec.Service == "" {
		return nil, fmt.Errorf("service name is required")
	}
	if spec.Version == "" {
		return nil, fmt.Errorf("version is required")
	}
	if spec.DeployCommand == "" {
		return nil, fmt.Errorf("deploy_command is required")
	}

	start := time.Now()
	result := &Result{
		ID:        fmt.Sprintf("deploy_%d", time.Now().UnixNano()),
		Spec:      spec,
		State:     StateRunning,
		StartedAt: start,
	}

	d.mu.Lock()
	d.active[result.ID] = result
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.active, result.ID)
		d.mu.Unlock()
	}()

	d.logger.Info("starting deployment",
		"id", result.ID,
		"service", spec.Service,
		"version", spec.Version,
		"strategy", spec.Strategy,
	)

	// Resolve target nodes
	roster, err := d.store.ListNodes(ctx)
	if err != nil {
		return d.fail(result, fmt.Errorf("list nodes: %w", err))
	}
	targets := spec.Target.Resolve(roster)
	if len(targets) == 0 {
		return d.fail(result, fmt.Errorf("no nodes matched target selector"))
	}

	// Execute deployment strategy
	var deployErr error
	switch spec.Strategy {
	case StrategyRolling:
		deployErr = d.deployRolling(ctx, spec, targets, result)
	case StrategyCanary:
		deployErr = d.deployCanary(ctx, spec, targets, result)
	case StrategyBlueGreen:
		deployErr = d.deployBlueGreen(ctx, spec, targets, result)
	case StrategyAllAtOnce:
		deployErr = d.deployAllAtOnce(ctx, spec, targets, result)
	case StrategySerial:
		deployErr = d.deploySerial(ctx, spec, targets, result)
	default:
		deployErr = fmt.Errorf("unknown strategy: %s", spec.Strategy)
	}

	if deployErr != nil {
		if spec.RollbackOnFail && spec.RollbackCommand != "" {
			d.rollback(ctx, spec, targets, result)
		}
		return d.fail(result, deployErr)
	}

	result.State = StateComplete
	result.FinishedAt = time.Now()
	result.Duration = time.Since(start)

	d.logger.Info("deployment complete",
		"id", result.ID,
		"duration", result.Duration,
		"batches", len(result.Batches),
	)

	return result, nil
}

func (d *Deployer) fail(result *Result, err error) (*Result, error) {
	result.State = StateFailed
	result.Error = err.Error()
	result.FinishedAt = time.Now()
	result.Duration = result.FinishedAt.Sub(result.StartedAt)
	return result, err
}

// deployRolling deploys in batches, checking health between each batch.
func (d *Deployer) deployRolling(ctx context.Context, spec Spec, targets []*fleet.Node, result *Result) error {
	batchSize := spec.MaxUnavailable
	if batchSize <= 0 {
		batchSize = 1
	}

	batches := splitIntoBatches(targets, batchSize)
	for i, batch := range batches {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		d.logger.Info("deploying batch", "batch", i+1, "total", len(batches), "nodes", len(batch))

		br, err := d.executeBatch(ctx, spec, batch, i)
		result.Batches = append(result.Batches, br)
		if err != nil {
			return fmt.Errorf("batch %d failed: %w", i+1, err)
		}

		// Health check between batches (skip for last batch)
		if spec.HealthCheckURL != "" && i < len(batches)-1 {
			if err := d.healthCheck(ctx, spec, batch); err != nil {
				return fmt.Errorf("health check failed after batch %d: %w", i+1, err)
			}
		}
	}
	return nil
}

// deployCanary deploys to increasing percentages of nodes.
func (d *Deployer) deployCanary(ctx context.Context, spec Spec, targets []*fleet.Node, result *Result) error {
	percentages := spec.CanaryPercent
	if len(percentages) == 0 {
		percentages = []int{5, 25, 100} // default canary percentages
	}

	deployed := 0
	for i, pct := range percentages {
		target := (len(targets) * pct) / 100
		if target < 1 {
			target = 1
		}
		if target > len(targets) {
			target = len(targets)
		}

		batch := targets[deployed:target]
		if len(batch) == 0 {
			continue
		}

		d.logger.Info("canary batch", "batch", i+1, "percent", pct, "nodes", len(batch))

		br, err := d.executeBatch(ctx, spec, batch, i)
		result.Batches = append(result.Batches, br)
		if err != nil {
			return fmt.Errorf("canary batch %d (%d%%) failed: %w", i+1, pct, err)
		}

		deployed = target

		// Health check between canary steps
		if spec.HealthCheckURL != "" && pct < 100 {
			if err := d.healthCheck(ctx, spec, batch); err != nil {
				return fmt.Errorf("canary health check failed at %d%%: %w", pct, err)
			}
		}
	}
	return nil
}

// deployBlueGreen deploys to all nodes at once, then switches.
func (d *Deployer) deployBlueGreen(ctx context.Context, spec Spec, targets []*fleet.Node, result *Result) error {
	// Blue-green: deploy to all, health check all, then done
	br, err := d.executeBatch(ctx, spec, targets, 0)
	result.Batches = append(result.Batches, br)
	if err != nil {
		return fmt.Errorf("blue-green deploy failed: %w", err)
	}

	if spec.HealthCheckURL != "" {
		if err := d.healthCheck(ctx, spec, targets); err != nil {
			return fmt.Errorf("blue-green health check failed: %w", err)
		}
	}

	return nil
}

// deployAllAtOnce deploys to all nodes simultaneously.
func (d *Deployer) deployAllAtOnce(ctx context.Context, spec Spec, targets []*fleet.Node, result *Result) error {
	br, err := d.executeBatch(ctx, spec, targets, 0)
	result.Batches = append(result.Batches, br)
	return err
}

// deploySerial deploys to nodes one at a time with optional delay.
func (d *Deployer) deploySerial(ctx context.Context, spec Spec, targets []*fleet.Node, result *Result) error {
	delay := spec.SerialDelay
	if delay <= 0 {
		delay = 2 * time.Second
	}

	for i, node := range targets {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		br, err := d.executeBatch(ctx, spec, []*fleet.Node{node}, i)
		result.Batches = append(result.Batches, br)
		if err != nil {
			return fmt.Errorf("serial deploy to %s failed: %w", node.ID, err)
		}

		// Wait between nodes (skip for last)
		if i < len(targets)-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return nil
}

func (d *Deployer) executeBatch(ctx context.Context, spec Spec, nodes []*fleet.Node, batchIdx int) (BatchResult, error) {
	start := time.Now()
	br := BatchResult{
		BatchIndex: batchIdx,
		StartedAt:  start,
	}

	// Build the deploy command with env-var injection to avoid shell injection.
	// The deploy command can reference $DEPLOY_SERVICE and $DEPLOY_VERSION.
	deployCmd := spec.DeployCommand
	if !strings.Contains(deployCmd, "$DEPLOY_SERVICE") {
		// Legacy mode: append service and version as arguments (shell-safe via env vars)
		deployCmd = fmt.Sprintf("DEPLOY_SERVICE=%q DEPLOY_VERSION=%q %s \"$DEPLOY_SERVICE\" \"$DEPLOY_VERSION\"",
			spec.Service, spec.Version, spec.DeployCommand)
	} else {
		deployCmd = fmt.Sprintf("DEPLOY_SERVICE=%q DEPLOY_VERSION=%q %s",
			spec.Service, spec.Version, spec.DeployCommand)
	}
	cmdJSON, _ := json.Marshal(fleet.ShellCommand{
		Command: deployCmd,
	})

	req := &fleet.ExecRequest{
		ID:      fmt.Sprintf("deploy_%d_batch_%d", time.Now().UnixNano(), batchIdx),
		Target:  fleet.TargetSelector{NodeIDs: nodeIDs(nodes)},
		Command: fleet.TypedCommand{Type: "shell", Data: cmdJSON},
		Timeout: 5 * time.Minute,
		Requester: spec.Requester,
	}

	result, err := d.executor.Execute(ctx, req)
	br.FinishedAt = time.Now()

	if err != nil {
		return br, err
	}

	br.Nodes = result.NodeResults

	// Check for failures
	for _, nr := range result.NodeResults {
		if nr.Status == "failure" || nr.Status == "timeout" {
			return br, fmt.Errorf("node %s: %s", nr.NodeID, nr.Error)
		}
	}

	br.HealthOK = true
	return br, nil
}

func (d *Deployer) healthCheck(ctx context.Context, spec Spec, nodes []*fleet.Node) error {
	timeout := spec.HealthTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	d.logger.Info("running health check", "url", spec.HealthCheckURL, "nodes", len(nodes))

	healthCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use shell-quoted health check URL to prevent injection
	cmdJSON, _ := json.Marshal(fleet.ShellCommand{
		Command:    fmt.Sprintf("curl -sf %q", spec.HealthCheckURL),
		TimeoutSec: int(timeout.Seconds()),
	})

	req := &fleet.ExecRequest{
		ID:      fmt.Sprintf("health_%d", time.Now().UnixNano()),
		Target:  fleet.TargetSelector{NodeIDs: nodeIDs(nodes)},
		Command: fleet.TypedCommand{Type: "shell", Data: cmdJSON},
		Timeout: timeout,
	}

	result, err := d.executor.Execute(healthCtx, req)
	if err != nil {
		return fmt.Errorf("health check execution: %w", err)
	}

	for _, nr := range result.NodeResults {
		if nr.Status != "success" {
			return fmt.Errorf("health check failed on %s: %s", nr.NodeID, nr.Error)
		}
	}

	return nil
}

func (d *Deployer) rollback(ctx context.Context, spec Spec, targets []*fleet.Node, result *Result) {
	d.logger.Warn("rolling back deployment", "id", result.ID, "service", spec.Service)
	result.State = StateRollback
	result.RolledBack = true

	cmdJSON, _ := json.Marshal(fleet.ShellCommand{
		Command: spec.RollbackCommand,
	})

	req := &fleet.ExecRequest{
		ID:        fmt.Sprintf("rollback_%d", time.Now().UnixNano()),
		Target:    fleet.TargetSelector{NodeIDs: nodeIDs(targets)},
		Command:   fleet.TypedCommand{Type: "shell", Data: cmdJSON},
		Timeout:   5 * time.Minute,
		Requester: spec.Requester,
	}

	if _, err := d.executor.Execute(ctx, req); err != nil {
		d.logger.Error("rollback failed", "error", err)
	}
}

// ActiveDeployments returns currently running deployments.
func (d *Deployer) ActiveDeployments() []*Result {
	d.mu.Lock()
	defer d.mu.Unlock()
	results := make([]*Result, 0, len(d.active))
	for _, r := range d.active {
		results = append(results, r)
	}
	return results
}

func splitIntoBatches(nodes []*fleet.Node, batchSize int) [][]*fleet.Node {
	var batches [][]*fleet.Node
	for i := 0; i < len(nodes); i += batchSize {
		end := i + batchSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batches = append(batches, nodes[i:end])
	}
	return batches
}

func nodeIDs(nodes []*fleet.Node) []fleet.NodeID {
	ids := make([]fleet.NodeID, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}
