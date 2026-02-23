package contracts

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(r.ListTools()) != 0 {
		t.Fatal("expected empty tool list")
	}
}

func TestRegisterAndExecute(t *testing.T) {
	r := NewRegistry()

	contract := ToolContract[ShellExecRequest, ShellExecResponse]{
		ToolName:    "shell_exec",
		ToolVersion: "1.0",
		Description: "Execute a shell command",
		Category:    "shell",
		Validate: func(req *ShellExecRequest) error {
			if req.Command == "" {
				return errors.New("command is required")
			}
			return nil
		},
		Execute: func(req *ShellExecRequest) (*ShellExecResponse, error) {
			return &ShellExecResponse{
				Stdout:   "hello world",
				ExitCode: 0,
			}, nil
		},
	}

	meta := ToolMeta{
		Name:        "shell_exec",
		Version:     "1.0",
		Description: "Execute a shell command",
		Category:    "shell",
	}

	Register(r, contract, meta)

	// Verify registration
	tools := r.ListTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "shell_exec" {
		t.Errorf("expected tool name shell_exec, got %s", tools[0].Name)
	}

	// GetTool
	tool, ok := r.GetTool("shell_exec")
	if !ok {
		t.Fatal("expected to find tool")
	}
	if tool.Category != "shell" {
		t.Errorf("expected category shell, got %s", tool.Category)
	}

	// Execute with valid input
	input, _ := json.Marshal(ShellExecRequest{Command: "echo hello"})
	output, err := r.Execute("shell_exec", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp ShellExecResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Stdout != "hello world" {
		t.Errorf("expected stdout 'hello world', got %s", resp.Stdout)
	}
	if resp.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", resp.ExitCode)
	}
}

func TestExecute_ValidationFailure(t *testing.T) {
	r := NewRegistry()

	contract := ToolContract[ShellExecRequest, ShellExecResponse]{
		ToolName: "shell_exec",
		Validate: func(req *ShellExecRequest) error {
			if req.Command == "" {
				return errors.New("command is required")
			}
			return nil
		},
		Execute: func(req *ShellExecRequest) (*ShellExecResponse, error) {
			return &ShellExecResponse{}, nil
		},
	}

	Register(r, contract, ToolMeta{Name: "shell_exec"})

	// Empty command should fail validation
	input, _ := json.Marshal(ShellExecRequest{Command: ""})
	_, err := r.Execute("shell_exec", input)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestExecute_UnknownTool(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute("nonexistent", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestExecute_InvalidJSON(t *testing.T) {
	r := NewRegistry()

	contract := ToolContract[ShellExecRequest, ShellExecResponse]{
		ToolName: "shell_exec",
		Execute: func(req *ShellExecRequest) (*ShellExecResponse, error) {
			return &ShellExecResponse{}, nil
		},
	}

	Register(r, contract, ToolMeta{Name: "shell_exec"})

	_, err := r.Execute("shell_exec", json.RawMessage(`{invalid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestExecute_NoValidator(t *testing.T) {
	r := NewRegistry()

	contract := ToolContract[FileReadRequest, FileReadResponse]{
		ToolName: "file_read",
		// No Validate function
		Execute: func(req *FileReadRequest) (*FileReadResponse, error) {
			return &FileReadResponse{
				Content: "file content",
				Size:    12,
				Lines:   1,
			}, nil
		},
	}

	Register(r, contract, ToolMeta{Name: "file_read"})

	input, _ := json.Marshal(FileReadRequest{Path: "/tmp/test.txt"})
	output, err := r.Execute("file_read", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp FileReadResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.Content != "file content" {
		t.Errorf("unexpected content: %s", resp.Content)
	}
}

func TestExecute_ExecutionError(t *testing.T) {
	r := NewRegistry()

	contract := ToolContract[ShellExecRequest, ShellExecResponse]{
		ToolName: "shell_exec",
		Execute: func(req *ShellExecRequest) (*ShellExecResponse, error) {
			return nil, errors.New("execution failed")
		},
	}

	Register(r, contract, ToolMeta{Name: "shell_exec"})

	input, _ := json.Marshal(ShellExecRequest{Command: "fail"})
	_, err := r.Execute("shell_exec", input)
	if err == nil {
		t.Fatal("expected execution error")
	}
}

func TestGetTool_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.GetTool("nonexistent")
	if ok {
		t.Fatal("expected tool not found")
	}
}

func TestMultipleToolRegistration(t *testing.T) {
	r := NewRegistry()

	shellContract := ToolContract[ShellExecRequest, ShellExecResponse]{
		ToolName: "shell_exec",
		Execute: func(req *ShellExecRequest) (*ShellExecResponse, error) {
			return &ShellExecResponse{Stdout: "shell output"}, nil
		},
	}

	fileContract := ToolContract[FileReadRequest, FileReadResponse]{
		ToolName: "file_read",
		Execute: func(req *FileReadRequest) (*FileReadResponse, error) {
			return &FileReadResponse{Content: "file content"}, nil
		},
	}

	Register(r, shellContract, ToolMeta{Name: "shell_exec", Category: "shell"})
	Register(r, fileContract, ToolMeta{Name: "file_read", Category: "filesystem"})

	tools := r.ListTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// Both should be executable
	shellInput, _ := json.Marshal(ShellExecRequest{Command: "ls"})
	shellOut, err := r.Execute("shell_exec", shellInput)
	if err != nil {
		t.Fatalf("shell exec failed: %v", err)
	}
	var shellResp ShellExecResponse
	json.Unmarshal(shellOut, &shellResp)
	if shellResp.Stdout != "shell output" {
		t.Errorf("unexpected shell output: %s", shellResp.Stdout)
	}

	fileInput, _ := json.Marshal(FileReadRequest{Path: "/test"})
	fileOut, err := r.Execute("file_read", fileInput)
	if err != nil {
		t.Fatalf("file read failed: %v", err)
	}
	var fileResp FileReadResponse
	json.Unmarshal(fileOut, &fileResp)
	if fileResp.Content != "file content" {
		t.Errorf("unexpected file content: %s", fileResp.Content)
	}
}

func TestToolMetaSerialization(t *testing.T) {
	meta := ToolMeta{
		Name:        "test_tool",
		Version:     "2.0",
		Description: "A test tool",
		Category:    "testing",
		Deprecated:  true,
		Supersedes:  "old_tool",
		Examples: []ToolExample{
			{
				Description: "Basic usage",
				Input:       map[string]string{"key": "value"},
				Output:      map[string]string{"result": "ok"},
			},
		},
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ToolMeta
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Name != "test_tool" {
		t.Errorf("expected name test_tool, got %s", decoded.Name)
	}
	if decoded.Version != "2.0" {
		t.Errorf("expected version 2.0, got %s", decoded.Version)
	}
	if !decoded.Deprecated {
		t.Error("expected deprecated to be true")
	}
	if decoded.Supersedes != "old_tool" {
		t.Errorf("expected supersedes old_tool, got %s", decoded.Supersedes)
	}
	if len(decoded.Examples) != 1 {
		t.Errorf("expected 1 example, got %d", len(decoded.Examples))
	}
}

func TestDeployRequestSerialization(t *testing.T) {
	req := DeployRequest{
		Service:     "api-server",
		Version:     "1.2.3",
		Strategy:    "blue-green",
		Environment: "production",
		Replicas:    3,
		HealthURL:   "https://api.example.com/health",
		RollbackOn:  "health-check",
		DryRun:      true,
		Targets:     []string{"node-1", "node-2"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded DeployRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Service != "api-server" {
		t.Errorf("wrong service: %s", decoded.Service)
	}
	if decoded.Strategy != "blue-green" {
		t.Errorf("wrong strategy: %s", decoded.Strategy)
	}
	if decoded.Replicas != 3 {
		t.Errorf("wrong replicas: %d", decoded.Replicas)
	}
	if len(decoded.Targets) != 2 {
		t.Errorf("wrong targets count: %d", len(decoded.Targets))
	}
}

func TestDockerExecRequestSerialization(t *testing.T) {
	req := DockerExecRequest{
		Action:    "run",
		Image:     "nginx:latest",
		Container: "my-nginx",
		Ports:     []string{"8080:80"},
		Env:       map[string]string{"ENV": "production"},
		Volumes:   []string{"/data:/data"},
		Command:   []string{"nginx", "-g", "daemon off;"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded DockerExecRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Action != "run" {
		t.Errorf("wrong action: %s", decoded.Action)
	}
	if decoded.Image != "nginx:latest" {
		t.Errorf("wrong image: %s", decoded.Image)
	}
	if len(decoded.Ports) != 1 {
		t.Errorf("wrong ports count: %d", len(decoded.Ports))
	}
}

func TestK8sRequestSerialization(t *testing.T) {
	req := K8sRequest{
		Action:    "apply",
		Resource:  "deployment",
		Name:      "web-app",
		Namespace: "production",
		Replicas:  5,
		Context:   "prod-cluster",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded K8sRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Namespace != "production" {
		t.Errorf("wrong namespace: %s", decoded.Namespace)
	}
	if decoded.Replicas != 5 {
		t.Errorf("wrong replicas: %d", decoded.Replicas)
	}
}

func TestBrowserRequestSerialization(t *testing.T) {
	req := BrowserRequest{
		Action:   "navigate",
		URL:      "https://example.com",
		Headless: true,
		Viewport: &Viewport{Width: 1920, Height: 1080},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded BrowserRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.URL != "https://example.com" {
		t.Errorf("wrong URL: %s", decoded.URL)
	}
	if decoded.Viewport == nil || decoded.Viewport.Width != 1920 {
		t.Error("wrong viewport")
	}
}

func TestFleetExecRequestSerialization(t *testing.T) {
	req := FleetExecRequest{
		Targets:        []string{"node-1", "group:web"},
		Command:        "uptime",
		WorkDir:        "/home/admin",
		TimeoutSec:     30,
		MaxConcurrency: 10,
		DryRun:         false,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded FleetExecRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded.Targets) != 2 {
		t.Errorf("wrong targets count: %d", len(decoded.Targets))
	}
	if decoded.MaxConcurrency != 10 {
		t.Errorf("wrong max concurrency: %d", decoded.MaxConcurrency)
	}
}

func TestRegisterOverwrite(t *testing.T) {
	r := NewRegistry()

	contract1 := ToolContract[ShellExecRequest, ShellExecResponse]{
		ToolName: "shell_exec",
		Execute: func(req *ShellExecRequest) (*ShellExecResponse, error) {
			return &ShellExecResponse{Stdout: "v1"}, nil
		},
	}

	contract2 := ToolContract[ShellExecRequest, ShellExecResponse]{
		ToolName: "shell_exec",
		Execute: func(req *ShellExecRequest) (*ShellExecResponse, error) {
			return &ShellExecResponse{Stdout: "v2"}, nil
		},
	}

	Register(r, contract1, ToolMeta{Name: "shell_exec", Version: "1.0"})
	Register(r, contract2, ToolMeta{Name: "shell_exec", Version: "2.0"})

	// Should use the latest registration
	input, _ := json.Marshal(ShellExecRequest{Command: "test"})
	output, err := r.Execute("shell_exec", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp ShellExecResponse
	json.Unmarshal(output, &resp)
	if resp.Stdout != "v2" {
		t.Errorf("expected v2, got %s", resp.Stdout)
	}

	// Metadata should also be updated
	meta, _ := r.GetTool("shell_exec")
	if meta.Version != "2.0" {
		t.Errorf("expected version 2.0, got %s", meta.Version)
	}
}
