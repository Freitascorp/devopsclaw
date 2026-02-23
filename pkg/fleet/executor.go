package fleet

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Executor fans out commands across fleet nodes with concurrency control,
// timeout enforcement, and result aggregation.
type Executor struct {
	store  Store
	relay  RelayClient
	logger *slog.Logger

	mu       sync.RWMutex
	inflight map[string]context.CancelFunc // request ID → cancel
}

// RelayClient abstracts the connection to remote nodes.
// Implementations: DirectClient (same-network SSH), TunnelClient (NAT-traversal relay).
type RelayClient interface {
	// Execute sends a typed command to a specific node and returns the result.
	Execute(ctx context.Context, node *Node, cmd TypedCommand) (*NodeResult, error)

	// Ping checks connectivity to a node.
	Ping(ctx context.Context, node *Node) error
}

// NewExecutor creates a fleet command executor.
func NewExecutor(store Store, relay RelayClient, logger *slog.Logger) *Executor {
	return &Executor{
		store:    store,
		relay:    relay,
		logger:   logger,
		inflight: make(map[string]context.CancelFunc),
	}
}

// Execute fans out a command to targeted nodes with concurrency control.
func (e *Executor) Execute(ctx context.Context, req *ExecRequest) (*ExecResult, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	start := time.Now()

	// Resolve target nodes
	roster, err := e.store.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	targets := req.Target.Resolve(roster)
	if len(targets) == 0 {
		return nil, fmt.Errorf("no nodes matched target selector")
	}

	e.logger.Info("executing fleet command",
		"request_id", req.ID,
		"command_type", req.Command.Type,
		"target_count", len(targets),
		"dry_run", req.DryRun,
		"requester", req.Requester,
	)

	// Register inflight for cancellation
	execCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	e.mu.Lock()
	e.inflight[req.ID] = cancel
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.inflight, req.ID)
		e.mu.Unlock()
	}()

	// Fan-out with concurrency limiter
	concurrency := req.Target.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 10 // sensible default
	}
	sem := make(chan struct{}, concurrency)
	resultCh := make(chan NodeResult, len(targets))

	var wg sync.WaitGroup
	for _, node := range targets {
		wg.Add(1)
		go func(n *Node) {
			defer wg.Done()
			sem <- struct{}{} // acquire
			defer func() { <-sem }() // release

			nr := e.executeOnNode(execCtx, n, req)
			resultCh <- nr
		}(node)
	}

	// Wait for all results
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var results []NodeResult
	for nr := range resultCh {
		results = append(results, nr)
	}

	// Build summary
	summary := ExecSummary{Total: len(results)}
	for _, r := range results {
		switch r.Status {
		case "success":
			summary.Success++
		case "failure":
			summary.Failed++
		case "timeout":
			summary.Timeout++
		case "skipped":
			summary.Skipped++
		}
	}

	result := &ExecResult{
		RequestID:   req.ID,
		NodeResults: results,
		Summary:     summary,
		Duration:    time.Since(start),
	}

	// Audit trail
	if err := e.store.RecordExecution(ctx, req, result); err != nil {
		e.logger.Error("failed to record execution", "error", err, "request_id", req.ID)
	}

	e.logger.Info("fleet command completed",
		"request_id", req.ID,
		"duration", result.Duration,
		"total", summary.Total,
		"success", summary.Success,
		"failed", summary.Failed,
		"timeout", summary.Timeout,
	)

	return result, nil
}

// Cancel aborts an inflight execution.
func (e *Executor) Cancel(requestID string) bool {
	e.mu.RLock()
	cancel, ok := e.inflight[requestID]
	e.mu.RUnlock()
	if ok {
		cancel()
		return true
	}
	return false
}

func (e *Executor) executeOnNode(ctx context.Context, node *Node, req *ExecRequest) NodeResult {
	start := time.Now()

	if req.DryRun {
		return NodeResult{
			NodeID:   node.ID,
			Hostname: node.Hostname,
			Output:   "[dry-run] would execute on this node",
			ExitCode: 0,
			Duration: time.Since(start),
			Status:   "skipped",
		}
	}

	nr, err := e.relay.Execute(ctx, node, req.Command)
	if err != nil {
		if ctx.Err() != nil {
			return NodeResult{
				NodeID:   node.ID,
				Hostname: node.Hostname,
				Error:    "execution timed out",
				Duration: time.Since(start),
				Status:   "timeout",
				ExitCode: -1,
			}
		}
		return NodeResult{
			NodeID:   node.ID,
			Hostname: node.Hostname,
			Error:    err.Error(),
			Duration: time.Since(start),
			Status:   "failure",
			ExitCode: -1,
		}
	}

	nr.Duration = time.Since(start)
	if nr.ExitCode == 0 && nr.Error == "" {
		nr.Status = "success"
	} else {
		nr.Status = "failure"
	}
	return *nr
}
