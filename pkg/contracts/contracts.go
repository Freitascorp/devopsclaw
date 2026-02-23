// Package contracts provides typed, validated tool definitions that replace
// the existing map[string]interface{} tool contracts with compile-time safe
// schemas. This is the bridge between DevOpsClaw's existing tool system and
// production-grade typed execution.
//
// Each tool has:
//   - A typed Request struct (validated before execution)
//   - A typed Response struct (guaranteed schema for consumers)
//   - JSON Schema generation for LLM tool calling
//   - Version tracking for backward compatibility
package contracts

import (
	"encoding/json"
	"fmt"
	"time"
)

// ------------------------------------------------------------------
// Contract system core
// ------------------------------------------------------------------

// ToolContract defines a typed, versioned tool with validated I/O.
type ToolContract[Req any, Resp any] struct {
	ToolName    string `json:"name"`
	ToolVersion string `json:"version"`
	Description string `json:"description"`
	Category    string `json:"category"` // "filesystem", "shell", "deploy", "docker", "k8s", "browser", "fleet"

	// Validate is called before execution. Return error to reject.
	Validate func(req *Req) error

	// Execute runs the tool with typed input and produces typed output.
	Execute func(req *Req) (*Resp, error)
}

// ToolMeta is the untyped metadata for registration/discovery.
type ToolMeta struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Parameters  map[string]any    `json:"parameters"`  // JSON Schema
	Returns     map[string]any    `json:"returns"`      // JSON Schema for output
	Examples    []ToolExample     `json:"examples,omitempty"`
	Deprecated  bool              `json:"deprecated,omitempty"`
	Supersedes  string            `json:"supersedes,omitempty"` // old tool name this replaces
}

// ToolExample documents expected I/O for a tool.
type ToolExample struct {
	Description string `json:"description"`
	Input       any    `json:"input"`
	Output      any    `json:"output"`
}

// ------------------------------------------------------------------
// Typed shell execution contract
// ------------------------------------------------------------------

// ShellExecRequest is the typed input for shell command execution.
type ShellExecRequest struct {
	Command    string            `json:"command" validate:"required"`
	WorkDir    string            `json:"work_dir,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	TimeoutSec int              `json:"timeout_sec,omitempty" validate:"gte=0,lte=3600"`
	Shell      string            `json:"shell,omitempty"` // default: /bin/sh
	DenyCheck  bool              `json:"deny_check,omitempty"` // run deny pattern check
}

// ShellExecResponse is the typed output of shell command execution.
type ShellExecResponse struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	Killed   bool          `json:"killed"` // true if killed by timeout
}

// ------------------------------------------------------------------
// Typed file operation contracts
// ------------------------------------------------------------------

// FileReadRequest reads file content.
type FileReadRequest struct {
	Path      string `json:"path" validate:"required"`
	MaxBytes  int    `json:"max_bytes,omitempty"` // 0 = full file
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

// FileReadResponse is the typed file read output.
type FileReadResponse struct {
	Content    string `json:"content"`
	Size       int64  `json:"size"`
	Lines      int    `json:"lines"`
	Truncated  bool   `json:"truncated"`
	ModifiedAt time.Time `json:"modified_at"`
}

// FileWriteRequest writes file content.
type FileWriteRequest struct {
	Path    string `json:"path" validate:"required"`
	Content string `json:"content" validate:"required"`
	Mode    string `json:"mode,omitempty"` // e.g., "0644"
	Append  bool   `json:"append,omitempty"`
	Backup  bool   `json:"backup,omitempty"` // create .bak before overwriting
}

// FileWriteResponse is the typed file write output.
type FileWriteResponse struct {
	BytesWritten int    `json:"bytes_written"`
	Path         string `json:"path"`
	BackupPath   string `json:"backup_path,omitempty"`
}

// FileEditRequest does search/replace in a file.
type FileEditRequest struct {
	Path      string `json:"path" validate:"required"`
	OldText   string `json:"old_text" validate:"required"`
	NewText   string `json:"new_text"`
	AllOccurrences bool `json:"all_occurrences,omitempty"`
}

// FileEditResponse is the typed file edit output.
type FileEditResponse struct {
	Replacements int    `json:"replacements"`
	Path         string `json:"path"`
}

// ------------------------------------------------------------------
// Typed deploy contracts
// ------------------------------------------------------------------

// DeployRequest triggers a service deployment.
type DeployRequest struct {
	Service     string   `json:"service" validate:"required"`
	Version     string   `json:"version" validate:"required"`
	Strategy    string   `json:"strategy" validate:"required,oneof=rolling blue-green canary"`
	Environment string   `json:"environment" validate:"required"`
	Replicas    int      `json:"replicas,omitempty" validate:"gte=0"`
	HealthURL   string   `json:"health_url,omitempty" validate:"omitempty,url"`
	RollbackOn  string   `json:"rollback_on,omitempty" validate:"omitempty,oneof=failure health-check manual"`
	DryRun      bool     `json:"dry_run,omitempty"`
	Targets     []string `json:"targets,omitempty"` // node IDs or group names
}

// DeployResponse is the typed deployment result.
type DeployResponse struct {
	DeployID    string        `json:"deploy_id"`
	Status      string        `json:"status"` // "success", "failed", "rolled-back", "dry-run"
	LogURL      string        `json:"log_url,omitempty"`
	Duration    time.Duration `json:"duration"`
	NodesAffected int        `json:"nodes_affected"`
	RolledBack  bool          `json:"rolled_back"`
	Error       string        `json:"error,omitempty"`
}

// ------------------------------------------------------------------
// Typed Docker contracts
// ------------------------------------------------------------------

// DockerExecRequest manages containers.
type DockerExecRequest struct {
	Action    string            `json:"action" validate:"required,oneof=run stop restart logs ps pull inspect"`
	Image     string            `json:"image,omitempty"`
	Container string            `json:"container,omitempty"`
	Ports     []string          `json:"ports,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Volumes   []string          `json:"volumes,omitempty"`
	TailLines int               `json:"tail_lines,omitempty"`
	Command   []string          `json:"command,omitempty"` // for run
}

// DockerExecResponse is the typed Docker operation result.
type DockerExecResponse struct {
	ContainerID string `json:"container_id,omitempty"`
	Output      string `json:"output"`
	Status      string `json:"status"` // "running", "stopped", "created", etc.
	ExitCode    int    `json:"exit_code,omitempty"`
}

// ------------------------------------------------------------------
// Typed Kubernetes contracts
// ------------------------------------------------------------------

// K8sRequest manages Kubernetes resources.
type K8sRequest struct {
	Action    string `json:"action" validate:"required,oneof=apply delete get rollout scale logs describe"`
	Resource  string `json:"resource" validate:"required"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Manifest  string `json:"manifest,omitempty"`
	Replicas  int    `json:"replicas,omitempty"`
	TailLines int    `json:"tail_lines,omitempty"`
	Context   string `json:"context,omitempty"` // kubeconfig context
}

// K8sResponse is the typed Kubernetes operation result.
type K8sResponse struct {
	Output    string `json:"output"`
	Resources []K8sResourceStatus `json:"resources,omitempty"`
	Status    string `json:"status"`
}

// K8sResourceStatus is status info for a K8s resource.
type K8sResourceStatus struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Ready     string `json:"ready,omitempty"`
	Age       string `json:"age,omitempty"`
}

// ------------------------------------------------------------------
// Typed browser automation contracts
// ------------------------------------------------------------------

// BrowserRequest runs browser automation actions.
type BrowserRequest struct {
	Action      string            `json:"action" validate:"required,oneof=navigate click type screenshot evaluate wait_for extract"`
	URL         string            `json:"url,omitempty"`
	Selector    string            `json:"selector,omitempty"`
	Value       string            `json:"value,omitempty"`
	Script      string            `json:"script,omitempty"`
	WaitFor     string            `json:"wait_for,omitempty"`
	SessionID   string            `json:"session_id,omitempty"`
	Cookies     map[string]string `json:"cookies,omitempty"`
	UserAgent   string            `json:"user_agent,omitempty"`
	Headless    bool              `json:"headless,omitempty"`
	Viewport    *Viewport         `json:"viewport,omitempty"`
}

// Viewport defines browser window dimensions.
type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// BrowserResponse is the typed browser automation result.
type BrowserResponse struct {
	SessionID   string `json:"session_id"`
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Content     string `json:"content,omitempty"` // extracted text or evaluate result
	Screenshot  string `json:"screenshot,omitempty"` // base64 PNG
	StatusCode  int    `json:"status_code,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ------------------------------------------------------------------
// Typed fleet operation contracts
// ------------------------------------------------------------------

// FleetExecRequest sends a command to fleet nodes.
type FleetExecRequest struct {
	Targets    []string `json:"targets" validate:"required"`      // node IDs or group names
	Command    string   `json:"command" validate:"required"`
	WorkDir    string   `json:"work_dir,omitempty"`
	TimeoutSec int     `json:"timeout_sec,omitempty"`
	MaxConcurrency int `json:"max_concurrency,omitempty"`
	DryRun     bool     `json:"dry_run,omitempty"`
}

// FleetExecResponse is the aggregated result of fleet execution.
type FleetExecResponse struct {
	RequestID string             `json:"request_id"`
	Results   []FleetNodeResult  `json:"results"`
	Total     int                `json:"total"`
	Success   int                `json:"success"`
	Failed    int                `json:"failed"`
	Duration  time.Duration      `json:"duration"`
}

// FleetNodeResult is the result from a single fleet node.
type FleetNodeResult struct {
	NodeID   string `json:"node_id"`
	Hostname string `json:"hostname"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

// ------------------------------------------------------------------
// Contract registry for runtime discovery
// ------------------------------------------------------------------

// Registry holds all registered tool contracts and provides
// type-safe execution plus LLM-compatible schema generation.
type Registry struct {
	tools map[string]ToolMeta
	executors map[string]func(json.RawMessage) (json.RawMessage, error)
}

// NewRegistry creates a typed tool contract registry.
func NewRegistry() *Registry {
	return &Registry{
		tools:     make(map[string]ToolMeta),
		executors: make(map[string]func(json.RawMessage) (json.RawMessage, error)),
	}
}

// Register adds a typed tool contract to the registry.
func Register[Req any, Resp any](r *Registry, contract ToolContract[Req, Resp], schema ToolMeta) {
	r.tools[contract.ToolName] = schema
	r.executors[contract.ToolName] = func(raw json.RawMessage) (json.RawMessage, error) {
		var req Req
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("invalid request for %s: %w", contract.ToolName, err)
		}
		if contract.Validate != nil {
			if err := contract.Validate(&req); err != nil {
				return nil, fmt.Errorf("validation failed for %s: %w", contract.ToolName, err)
			}
		}
		resp, err := contract.Execute(&req)
		if err != nil {
			return nil, err
		}
		return json.Marshal(resp)
	}
}

// Execute runs a tool by name with raw JSON input, returning raw JSON output.
// This is the bridge between the LLM tool-calling interface and typed contracts.
func (r *Registry) Execute(name string, input json.RawMessage) (json.RawMessage, error) {
	exec, ok := r.executors[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return exec(input)
}

// ListTools returns metadata for all registered tools.
func (r *Registry) ListTools() []ToolMeta {
	out := make([]ToolMeta, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// GetTool returns metadata for a specific tool.
func (r *Registry) GetTool(name string) (ToolMeta, bool) {
	t, ok := r.tools[name]
	return t, ok
}
