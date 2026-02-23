package runbook

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseRunbook_Valid(t *testing.T) {
	yaml := `
name: test-runbook
description: A test runbook
tags: [test, ci]
steps:
  - name: Check disk
    run: df -h
  - name: Check memory
    run: free -m
`
	rb, err := ParseRunbook([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseRunbook: %v", err)
	}
	if rb.Name != "test-runbook" {
		t.Errorf("Name = %q, want test-runbook", rb.Name)
	}
	if rb.Description != "A test runbook" {
		t.Errorf("Description = %q", rb.Description)
	}
	if len(rb.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(rb.Tags))
	}
	if len(rb.Steps) != 2 {
		t.Errorf("Steps len = %d, want 2", len(rb.Steps))
	}
	if rb.Steps[0].Name != "Check disk" {
		t.Errorf("Step[0].Name = %q", rb.Steps[0].Name)
	}
}

func TestParseRunbook_NoName(t *testing.T) {
	yaml := `
steps:
  - name: step1
    run: echo hi
`
	_, err := ParseRunbook([]byte(yaml))
	if err == nil {
		t.Error("expected error for runbook without name")
	}
}

func TestParseRunbook_NoSteps(t *testing.T) {
	yaml := `
name: empty
`
	_, err := ParseRunbook([]byte(yaml))
	if err == nil {
		t.Error("expected error for runbook without steps")
	}
}

func TestParseRunbook_BrowseStep(t *testing.T) {
	yaml := `
name: browse-test
steps:
  - name: Check dashboard
    browse:
      url: https://example.com
      task: get metrics
`
	rb, err := ParseRunbook([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseRunbook: %v", err)
	}
	if rb.Steps[0].Browse == nil {
		t.Fatal("expected Browse to be set")
	}
	if rb.Steps[0].Browse.URL != "https://example.com" {
		t.Errorf("Browse.URL = %q", rb.Steps[0].Browse.URL)
	}
}

func TestParseRunbook_InvalidYAML(t *testing.T) {
	_, err := ParseRunbook([]byte(":::not yaml"))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestEngine_ListEmpty(t *testing.T) {
	dir := t.TempDir()
	engine := NewEngine(dir)

	runbooks, err := engine.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(runbooks) != 0 {
		t.Fatalf("expected 0 runbooks, got %d", len(runbooks))
	}
}

func TestEngine_ListAndGet(t *testing.T) {
	dir := t.TempDir()

	yamlContent := `
name: incident-db
description: Handle DB incidents
steps:
  - name: Check connections
    run: echo "checking"
`
	os.WriteFile(filepath.Join(dir, "incident-db.yaml"), []byte(yamlContent), 0o644)

	engine := NewEngine(dir)

	// List
	runbooks, err := engine.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(runbooks) != 1 {
		t.Fatalf("expected 1 runbook, got %d", len(runbooks))
	}
	if runbooks[0].Name != "incident-db" {
		t.Errorf("Name = %q", runbooks[0].Name)
	}

	// Get
	rb, err := engine.Get("incident-db")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rb.Name != "incident-db" {
		t.Errorf("Name = %q", rb.Name)
	}
}

func TestEngine_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	engine := NewEngine(dir)

	_, err := engine.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent runbook")
	}
}

func TestEngine_RunShellStep(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: shell-test
steps:
  - name: Echo hello
    run: echo "hello world"
`
	os.WriteFile(filepath.Join(dir, "shell-test.yaml"), []byte(yamlContent), 0o644)

	engine := NewEngine(dir)
	rb, _ := engine.Get("shell-test")

	result, err := engine.Run(context.Background(), rb, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("Status = %q, want success", result.Status)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step result, got %d", len(result.Steps))
	}
	if result.Steps[0].Status != "success" {
		t.Errorf("Step.Status = %q, want success", result.Steps[0].Status)
	}
	if result.Steps[0].Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestEngine_RunDryRun(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: dry-run-test
steps:
  - name: Dangerous command
    run: rm -rf /tmp/test
`
	os.WriteFile(filepath.Join(dir, "dry-run-test.yaml"), []byte(yamlContent), 0o644)

	engine := NewEngine(dir)
	rb, _ := engine.Get("dry-run-test")

	result, err := engine.Run(context.Background(), rb, true)
	if err != nil {
		t.Fatalf("Run dry: %v", err)
	}
	if !result.DryRun {
		t.Error("expected DryRun=true")
	}
	if result.Steps[0].Status != "skipped" {
		t.Errorf("Step.Status = %q, want skipped", result.Steps[0].Status)
	}
}

func TestEngine_RunFailedCommand(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: fail-test
steps:
  - name: Will fail
    run: exit 1
  - name: Should not run
    run: echo "nope"
`
	os.WriteFile(filepath.Join(dir, "fail-test.yaml"), []byte(yamlContent), 0o644)

	engine := NewEngine(dir)
	rb, _ := engine.Get("fail-test")

	result, _ := engine.Run(context.Background(), rb, false)
	if result.Status != "failure" {
		t.Errorf("Status = %q, want failure", result.Status)
	}
	// Only the first step should have run
	if len(result.Steps) != 1 {
		t.Errorf("expected 1 step result (stopped on failure), got %d", len(result.Steps))
	}
}

func TestEngine_ContinueOnError(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: continue-test
steps:
  - name: Fail but continue
    run: exit 1
    continue_on_error: true
  - name: Should still run
    run: echo "ok"
`
	os.WriteFile(filepath.Join(dir, "continue-test.yaml"), []byte(yamlContent), 0o644)

	engine := NewEngine(dir)
	rb, _ := engine.Get("continue-test")

	result, _ := engine.Run(context.Background(), rb, false)
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.Steps))
	}
	if result.Steps[0].Status != "failure" {
		t.Errorf("Step[0].Status = %q, want failure", result.Steps[0].Status)
	}
	if result.Steps[1].Status != "success" {
		t.Errorf("Step[1].Status = %q, want success", result.Steps[1].Status)
	}
	if result.Status != "partial" {
		t.Errorf("RunResult.Status = %q, want partial", result.Status)
	}
}

func TestEngine_VariableCapture(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: capture-test
steps:
  - name: Get hostname
    run: echo "myhost"
    capture: host
  - name: Use hostname
    run: echo "Host is {{ host }}"
`
	os.WriteFile(filepath.Join(dir, "capture-test.yaml"), []byte(yamlContent), 0o644)

	engine := NewEngine(dir)
	rb, _ := engine.Get("capture-test")

	result, err := engine.Run(context.Background(), rb, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Steps[1].Output == "" {
		t.Fatal("expected output")
	}
	// The second step should contain "Host is myhost"
	if result.Steps[1].Output != "Host is myhost\n" {
		t.Errorf("Step[1].Output = %q, want 'Host is myhost\\n'", result.Steps[1].Output)
	}
}

func TestEngine_RequiresApproval(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: approval-test
steps:
  - name: Needs approval
    run: echo "dangerous"
    requires_approval: true
  - name: After approval
    run: echo "ok"
`
	os.WriteFile(filepath.Join(dir, "approval-test.yaml"), []byte(yamlContent), 0o644)

	engine := NewEngine(dir)
	rb, _ := engine.Get("approval-test")

	result, _ := engine.Run(context.Background(), rb, false)
	if result.Steps[0].Status != "pending_approval" {
		t.Errorf("Step[0].Status = %q, want pending_approval", result.Steps[0].Status)
	}
	// Should stop after approval gate
	if len(result.Steps) != 1 {
		t.Errorf("expected 1 step result (stopped at approval), got %d", len(result.Steps))
	}
}

func TestEngine_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: cancel-test
steps:
  - name: Long running
    run: sleep 10
    timeout_sec: 1
`
	os.WriteFile(filepath.Join(dir, "cancel-test.yaml"), []byte(yamlContent), 0o644)

	engine := NewEngine(dir)
	rb, _ := engine.Get("cancel-test")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := engine.Run(ctx, rb, false)
	if err == nil {
		t.Error("expected context cancellation error")
	}
	if result.Status != "failure" {
		t.Errorf("Status = %q, want failure", result.Status)
	}
}

func TestFormatResult(t *testing.T) {
	result := &RunResult{
		RunbookName: "test",
		Status:      "success",
		DryRun:      false,
		Steps: []StepResult{
			{StepName: "step1", Status: "success"},
			{StepName: "step2", Status: "failure", Error: "oops"},
		},
	}
	output := FormatResult(result)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatResultJSON(t *testing.T) {
	result := &RunResult{
		RunbookName: "test",
		Status:      "success",
	}
	output, err := FormatResultJSON(result)
	if err != nil {
		t.Fatalf("FormatResultJSON: %v", err)
	}
	if output == "" {
		t.Error("expected non-empty JSON output")
	}
}

func TestInterpolate(t *testing.T) {
	engine := &Engine{
		variables: map[string]string{
			"host":    "prod-1",
			"version": "v2.0",
		},
	}

	tests := []struct {
		input string
		want  string
	}{
		{"echo {{ host }}", "echo prod-1"},
		{"deploy {{version}}", "deploy v2.0"},
		{"{{ host }} running {{version}}", "prod-1 running v2.0"},
		{"no variables here", "no variables here"},
	}

	for _, tt := range tests {
		got := engine.interpolate(tt.input)
		if got != tt.want {
			t.Errorf("interpolate(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEngine_NotifyStep(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: notify-test
steps:
  - name: Send notification
    notify: slack
    message: "Deploy completed"
`
	os.WriteFile(filepath.Join(dir, "notify-test.yaml"), []byte(yamlContent), 0o644)

	engine := NewEngine(dir)
	rb, _ := engine.Get("notify-test")

	result, err := engine.Run(context.Background(), rb, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Steps[0].Status != "success" {
		t.Errorf("Step.Status = %q, want success", result.Steps[0].Status)
	}
}

func TestEngine_StepWithEnv(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: env-test
steps:
  - name: Use env vars
    run: echo "$MY_VAR"
    env:
      MY_VAR: hello-from-env
`
	os.WriteFile(filepath.Join(dir, "env-test.yaml"), []byte(yamlContent), 0o644)

	engine := NewEngine(dir)
	rb, _ := engine.Get("env-test")

	result, err := engine.Run(context.Background(), rb, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Steps[0].Output != "hello-from-env\n" {
		t.Errorf("Output = %q, want 'hello-from-env\\n'", result.Steps[0].Output)
	}
}

func TestEngine_ListSkipsInvalidFiles(t *testing.T) {
	dir := t.TempDir()

	// Valid runbook
	os.WriteFile(filepath.Join(dir, "good.yaml"), []byte("name: good\nsteps:\n  - name: s1\n    run: echo hi"), 0o644)
	// Invalid runbook (no name)
	os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("steps:\n  - name: s1\n    run: echo"), 0o644)
	// Not a YAML file
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a runbook"), 0o644)

	engine := NewEngine(dir)
	runbooks, err := engine.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(runbooks) != 1 {
		t.Fatalf("expected 1 valid runbook, got %d", len(runbooks))
	}
}

func TestEngine_YmlExtension(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.yml"), []byte("name: yml-test\nsteps:\n  - name: s1\n    run: echo hi"), 0o644)

	engine := NewEngine(dir)
	rb, err := engine.Get("test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rb.Name != "yml-test" {
		t.Errorf("Name = %q, want yml-test", rb.Name)
	}
}
