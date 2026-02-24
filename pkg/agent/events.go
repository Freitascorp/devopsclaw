// DevOpsClaw - Ultra-lightweight personal AI agent
// License: MIT

package agent

import "github.com/freitascorp/devopsclaw/pkg/tools"

// EventType classifies agent loop events for real-time UI rendering.
// This enables a Claude Code–style experience where the user can see
// tool calls, intermediate results, and reasoning as they happen.
type EventType int

const (
	// EventThinking signals that the LLM is processing (new iteration starting).
	EventThinking EventType = iota
	// EventToolCall signals the LLM has requested a tool invocation.
	EventToolCall
	// EventToolResult signals a tool has finished executing.
	EventToolResult
	// EventToolDenied signals a tool call was denied (RBAC or user rejection).
	EventToolDenied
	// EventResponse signals the LLM has returned a final text response (no more tool calls).
	EventResponse
	// EventError signals an error during processing.
	EventError
	// EventPlanUpdate signals the agent's task plan has changed.
	EventPlanUpdate
)

// AgentEvent represents a single event during the agentic loop.
// The CLI or any subscriber uses these to render real-time updates.
type AgentEvent struct {
	Type      EventType
	Iteration int    // Current loop iteration (1-based)
	MaxIter   int    // Maximum iterations allowed
	Model     string // Model used for this iteration

	// Tool call fields (EventToolCall, EventToolResult, EventToolDenied)
	ToolName string
	ToolArgs map[string]any
	ToolID   string // Tool call ID for correlation

	// Tool result fields (EventToolResult)
	ToolOutput string
	IsError    bool

	// Denial reason (EventToolDenied)
	DenyReason string

	// Response/error content (EventResponse, EventError)
	Content string

	// Plan fields (EventPlanUpdate)
	PlanSteps []tools.PlanStep

	// Token usage — cumulative across all iterations
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// EventCallback is invoked during the agent loop to stream events to the UI.
// It is called synchronously from the agent loop goroutine.
// Implementations should return quickly to avoid blocking the loop.
type EventCallback func(event AgentEvent)
