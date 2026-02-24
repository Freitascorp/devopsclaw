package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// PlanStatus represents the state of a plan step.
type PlanStatus string

const (
	PlanNotStarted PlanStatus = "not-started"
	PlanInProgress PlanStatus = "in-progress"
	PlanCompleted  PlanStatus = "completed"
)

// PlanStep is a single item in the agent's task plan.
type PlanStep struct {
	ID     int        `json:"id"`
	Title  string     `json:"title"`
	Status PlanStatus `json:"status"`
}

// PlanState holds the current plan. Thread-safe for concurrent reads.
type PlanState struct {
	mu    sync.RWMutex
	Steps []PlanStep
}

// Update replaces the plan with new steps.
func (ps *PlanState) Update(steps []PlanStep) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.Steps = steps
}

// Snapshot returns a copy of the current plan steps.
func (ps *PlanState) Snapshot() []PlanStep {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	out := make([]PlanStep, len(ps.Steps))
	copy(out, ps.Steps)
	return out
}

// Progress returns (completed, total) counts.
func (ps *PlanState) Progress() (int, int) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	done := 0
	for _, s := range ps.Steps {
		if s.Status == PlanCompleted {
			done++
		}
	}
	return done, len(ps.Steps)
}

// Clear empties the plan.
func (ps *PlanState) Clear() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.Steps = nil
}

// PlanCallback is called whenever the plan changes, to notify the UI.
type PlanCallback func(steps []PlanStep)

// PlanTool lets the LLM create and update a visible task plan.
// Works exactly like GitHub Copilot's manage_todo_list — the LLM
// sends the full list of steps each time it calls the tool.
type PlanTool struct {
	state    *PlanState
	onChange PlanCallback
}

// NewPlanTool creates a new plan tool with shared state.
func NewPlanTool(state *PlanState, onChange PlanCallback) *PlanTool {
	return &PlanTool{
		state:    state,
		onChange: onChange,
	}
}

func (t *PlanTool) Name() string { return "plan" }

func (t *PlanTool) Description() string {
	return `Manage a visible task plan to track progress. Use this tool to create, update, and track a step-by-step plan for complex tasks.

When to use:
- Multi-step tasks that require planning and tracking
- Before starting work: create the plan
- When starting a step: mark it as in-progress
- After finishing a step: mark it as completed (immediately, one at a time)

Rules:
- Send the COMPLETE list of ALL steps every time (existing + new)
- Only ONE step should be "in-progress" at a time
- Mark steps "completed" immediately after finishing each one
- Use short, action-oriented titles (3-8 words)
- NEVER stop working until ALL steps are completed
- If a step fails, adapt and retry — do NOT abandon the plan
- Do NOT ask the user for permission to continue to the next step

Status values: "not-started", "in-progress", "completed"`
}

func (t *PlanTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"steps": map[string]any{
				"type":        "array",
				"description": "Complete array of ALL plan steps. Must include every step — both existing and new.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{
							"type":        "integer",
							"description": "Unique step ID (sequential, starting from 1)",
						},
						"title": map[string]any{
							"type":        "string",
							"description": "Short action-oriented step title (3-8 words)",
						},
						"status": map[string]any{
							"type":        "string",
							"enum":        []string{"not-started", "in-progress", "completed"},
							"description": "Step status",
						},
					},
					"required": []string{"id", "title", "status"},
				},
			},
		},
		"required": []string{"steps"},
	}
}

func (t *PlanTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	stepsRaw, ok := args["steps"]
	if !ok {
		return ErrorResult("missing 'steps' parameter")
	}

	// Parse the steps from the LLM's arguments
	stepsJSON, err := json.Marshal(stepsRaw)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid steps format: %v", err))
	}

	var steps []PlanStep
	if err := json.Unmarshal(stepsJSON, &steps); err != nil {
		return ErrorResult(fmt.Sprintf("failed to parse steps: %v", err))
	}

	if len(steps) == 0 {
		t.state.Clear()
		if t.onChange != nil {
			t.onChange(nil)
		}
		return SilentResult("Plan cleared.")
	}

	// Validate: at most one in-progress
	inProgress := 0
	for _, s := range steps {
		if s.Status == PlanInProgress {
			inProgress++
		}
	}
	if inProgress > 1 {
		return ErrorResult("only one step can be 'in-progress' at a time")
	}

	// Update state
	t.state.Update(steps)

	// Notify UI
	if t.onChange != nil {
		t.onChange(steps)
	}

	// Build summary for LLM
	done, total := t.state.Progress()
	var lines []string
	for _, s := range steps {
		icon := "○"
		switch s.Status {
		case PlanInProgress:
			icon = "◉"
		case PlanCompleted:
			icon = "✓"
		}
		lines = append(lines, fmt.Sprintf("  %s %d. %s", icon, s.ID, s.Title))
	}

	summary := fmt.Sprintf("Plan updated (%d/%d completed):\n%s", done, total, strings.Join(lines, "\n"))
	return SilentResult(summary)
}
