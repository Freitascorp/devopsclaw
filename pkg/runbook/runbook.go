// Package runbook provides YAML-defined, versioned, multi-step workflow execution.
//
// Runbooks replace shared Google Docs, Notion pages, and tribal knowledge with
// executable, audited, version-controlled procedures. Each runbook is a YAML file
// with typed steps that can run shell commands, fleet executions, browser tasks,
// notifications, and approval gates.
package runbook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Runbook is a YAML-defined workflow.
type Runbook struct {
	Name        string   `yaml:"name"        json:"name"`
	Description string   `yaml:"description" json:"description"`
	Tags        []string `yaml:"tags"        json:"tags"`
	Steps       []Step   `yaml:"steps"       json:"steps"`
}

// Step is a single action in a runbook.
type Step struct {
	Name             string            `yaml:"name"              json:"name"`
	Run              string            `yaml:"run,omitempty"     json:"run,omitempty"`
	Browse           *BrowseStep       `yaml:"browse,omitempty"  json:"browse,omitempty"`
	Notify           string            `yaml:"notify,omitempty"  json:"notify,omitempty"`
	Message          string            `yaml:"message,omitempty" json:"message,omitempty"`
	Target           *StepTarget       `yaml:"target,omitempty"  json:"target,omitempty"`
	Capture          string            `yaml:"capture,omitempty" json:"capture,omitempty"`
	RequiresApproval bool              `yaml:"requires_approval,omitempty" json:"requires_approval,omitempty"`
	ContinueOnError  bool              `yaml:"continue_on_error,omitempty" json:"continue_on_error,omitempty"`
	TimeoutSec       int               `yaml:"timeout_sec,omitempty" json:"timeout_sec,omitempty"`
	Env              map[string]string `yaml:"env,omitempty"     json:"env,omitempty"`
}

// BrowseStep defines a browser automation step within a runbook.
type BrowseStep struct {
	URL     string `yaml:"url,omitempty"     json:"url,omitempty"`
	Session string `yaml:"session,omitempty" json:"session,omitempty"`
	Task    string `yaml:"task"              json:"task"`
}

// StepTarget defines where a step should execute.
type StepTarget struct {
	Tag  string `yaml:"tag,omitempty"  json:"tag,omitempty"`
	Env  string `yaml:"env,omitempty"  json:"env,omitempty"`
	Node string `yaml:"node,omitempty" json:"node,omitempty"`
}

// StepResult is the outcome of running a single step.
type StepResult struct {
	StepName  string        `json:"step_name"`
	Status    string        `json:"status"` // "success", "failure", "skipped", "pending_approval"
	Output    string        `json:"output,omitempty"`
	Error     string        `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	Captured  string        `json:"captured,omitempty"` // value captured for variable interpolation
}

// RunResult is the outcome of an entire runbook execution.
type RunResult struct {
	RunbookName string        `json:"runbook_name"`
	StartedAt   time.Time     `json:"started_at"`
	FinishedAt  time.Time     `json:"finished_at"`
	Duration    time.Duration `json:"duration"`
	Status      string        `json:"status"` // "success", "failure", "partial"
	Steps       []StepResult  `json:"steps"`
	DryRun      bool          `json:"dry_run"`
}

// LoadRunbook loads a runbook from a YAML file.
func LoadRunbook(path string) (*Runbook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read runbook: %w", err)
	}
	return ParseRunbook(data)
}

// ParseRunbook parses YAML data into a Runbook.
func ParseRunbook(data []byte) (*Runbook, error) {
	var rb Runbook
	if err := yaml.Unmarshal(data, &rb); err != nil {
		return nil, fmt.Errorf("parse runbook YAML: %w", err)
	}
	if rb.Name == "" {
		return nil, fmt.Errorf("runbook must have a name")
	}
	if len(rb.Steps) == 0 {
		return nil, fmt.Errorf("runbook must have at least one step")
	}
	return &rb, nil
}

// Engine executes runbooks.
type Engine struct {
	runbookDir string
	variables  map[string]string // captured variables from steps
}

// NewEngine creates a runbook engine that loads runbooks from the given directory.
func NewEngine(runbookDir string) *Engine {
	os.MkdirAll(runbookDir, 0o755)
	return &Engine{
		runbookDir: runbookDir,
		variables:  make(map[string]string),
	}
}

// List returns all runbooks in the directory.
func (e *Engine) List() ([]*Runbook, error) {
	entries, err := os.ReadDir(e.runbookDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var runbooks []*Runbook
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		rb, err := LoadRunbook(filepath.Join(e.runbookDir, entry.Name()))
		if err != nil {
			continue // skip invalid runbooks
		}
		runbooks = append(runbooks, rb)
	}
	return runbooks, nil
}

// Get loads a specific runbook by name.
func (e *Engine) Get(name string) (*Runbook, error) {
	// Try exact name.yaml and name.yml
	for _, ext := range []string{".yaml", ".yml"} {
		path := filepath.Join(e.runbookDir, name+ext)
		if _, err := os.Stat(path); err == nil {
			return LoadRunbook(path)
		}
	}
	return nil, fmt.Errorf("runbook %q not found in %s", name, e.runbookDir)
}

// Run executes a runbook, returning the full result.
func (e *Engine) Run(ctx context.Context, rb *Runbook, dryRun bool) (*RunResult, error) {
	start := time.Now()
	result := &RunResult{
		RunbookName: rb.Name,
		StartedAt:   start,
		DryRun:      dryRun,
	}

	// Reset variables
	e.variables = make(map[string]string)

	allSuccess := true
	for _, step := range rb.Steps {
		select {
		case <-ctx.Done():
			result.Status = "failure"
			result.FinishedAt = time.Now()
			result.Duration = time.Since(start)
			return result, ctx.Err()
		default:
		}

		sr := e.executeStep(ctx, step, dryRun)
		result.Steps = append(result.Steps, sr)

		// Capture variable if specified
		if step.Capture != "" && sr.Output != "" {
			e.variables[step.Capture] = strings.TrimSpace(sr.Output)
		}

		if sr.Status == "pending_approval" {
			allSuccess = false
			break // stop execution, waiting for approval
		}

		if sr.Status == "failure" && !step.ContinueOnError {
			allSuccess = false
			break
		}
		if sr.Status == "failure" {
			allSuccess = false
		}
	}

	result.FinishedAt = time.Now()
	result.Duration = time.Since(start)
	if allSuccess {
		result.Status = "success"
	} else {
		// Check if any steps succeeded
		hasSuccess := false
		for _, s := range result.Steps {
			if s.Status == "success" {
				hasSuccess = true
				break
			}
		}
		if hasSuccess {
			result.Status = "partial"
		} else {
			result.Status = "failure"
		}
	}

	return result, nil
}

func (e *Engine) executeStep(ctx context.Context, step Step, dryRun bool) StepResult {
	start := time.Now()
	sr := StepResult{StepName: step.Name}

	if step.RequiresApproval && !dryRun {
		sr.Status = "pending_approval"
		sr.Output = "Step requires human approval before execution"
		sr.Duration = time.Since(start)
		return sr
	}

	if dryRun {
		sr.Status = "skipped"
		sr.Output = "[dry-run] "
		if step.Run != "" {
			sr.Output += fmt.Sprintf("would run: %s", step.Run)
		} else if step.Browse != nil {
			sr.Output += fmt.Sprintf("would browse: %s", step.Browse.Task)
		} else if step.Notify != "" {
			sr.Output += fmt.Sprintf("would notify: %s — %s", step.Notify, e.interpolate(step.Message))
		}
		sr.Duration = time.Since(start)
		return sr
	}

	// Shell execution step
	if step.Run != "" {
		return e.executeShellStep(ctx, step, start)
	}

	// Browse step (placeholder — will be wired when browser is integrated)
	if step.Browse != nil {
		sr.Status = "success"
		sr.Output = fmt.Sprintf("[browse] task: %s", step.Browse.Task)
		sr.Duration = time.Since(start)
		return sr
	}

	// Notify step (placeholder — will be wired to channels)
	if step.Notify != "" {
		sr.Status = "success"
		sr.Output = fmt.Sprintf("[notify:%s] %s", step.Notify, e.interpolate(step.Message))
		sr.Duration = time.Since(start)
		return sr
	}

	sr.Status = "failure"
	sr.Error = "step has no executable action (run, browse, or notify)"
	sr.Duration = time.Since(start)
	return sr
}

func (e *Engine) executeShellStep(ctx context.Context, step Step, start time.Time) StepResult {
	sr := StepResult{StepName: step.Name}

	timeout := 60 * time.Second
	if step.TimeoutSec > 0 {
		timeout = time.Duration(step.TimeoutSec) * time.Second
	}

	shellCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmdStr := e.interpolate(step.Run)
	cmd := exec.CommandContext(shellCtx, "sh", "-c", cmdStr)

	// Set environment variables
	if len(step.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range step.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, e.interpolate(v)))
		}
	}

	output, err := cmd.CombinedOutput()
	sr.Output = string(output)
	sr.Duration = time.Since(start)

	if err != nil {
		sr.Status = "failure"
		sr.Error = err.Error()
	} else {
		sr.Status = "success"
	}

	return sr
}

// interpolate replaces {{ variable }} placeholders with captured values.
func (e *Engine) interpolate(s string) string {
	result := s
	for k, v := range e.variables {
		result = strings.ReplaceAll(result, "{{ "+k+" }}", v)
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

// FormatResult returns a human-readable summary of a runbook run.
func FormatResult(r *RunResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Runbook: %s\n", r.RunbookName))
	b.WriteString(fmt.Sprintf("Status:  %s\n", r.Status))
	b.WriteString(fmt.Sprintf("Duration: %s\n", r.Duration.Round(time.Millisecond)))
	if r.DryRun {
		b.WriteString("Mode:    DRY RUN\n")
	}
	b.WriteString("\nSteps:\n")
	for i, s := range r.Steps {
		icon := "✓"
		switch s.Status {
		case "failure":
			icon = "✗"
		case "skipped":
			icon = "○"
		case "pending_approval":
			icon = "⏸"
		}
		b.WriteString(fmt.Sprintf("  %d. %s %s (%s) [%s]\n", i+1, icon, s.StepName, s.Duration.Round(time.Millisecond), s.Status))
		if s.Error != "" {
			b.WriteString(fmt.Sprintf("     Error: %s\n", s.Error))
		}
	}
	return b.String()
}

// FormatResultJSON returns the result as formatted JSON.
func FormatResultJSON(r *RunResult) (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
