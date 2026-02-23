package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/freitascorp/devopsclaw/pkg/fleet"
)

// ShellExecutor implements LocalExecutor by running shell commands locally.
type ShellExecutor struct {
	WorkDir string
	DenyPatterns []string
}

// NewShellExecutor creates a local shell command executor.
func NewShellExecutor(workDir string) *ShellExecutor {
	return &ShellExecutor{WorkDir: workDir}
}

// Execute runs a typed command locally on this node.
func (e *ShellExecutor) Execute(ctx context.Context, cmd fleet.TypedCommand) (*fleet.NodeResult, error) {
	switch cmd.Type {
	case "shell":
		return e.executeShell(ctx, cmd.Data)
	case "file":
		return e.executeFile(ctx, cmd.Data)
	default:
		return &fleet.NodeResult{
			Error:    fmt.Sprintf("unsupported command type: %s", cmd.Type),
			Status:   "failure",
			ExitCode: -1,
		}, nil
	}
}

func (e *ShellExecutor) executeShell(ctx context.Context, data json.RawMessage) (*fleet.NodeResult, error) {
	var sc fleet.ShellCommand
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("unmarshal shell command: %w", err)
	}

	timeout := 30 * time.Second
	if sc.TimeoutSec > 0 {
		timeout = time.Duration(sc.TimeoutSec) * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	shell := sc.Shell
	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := exec.CommandContext(cmdCtx, shell, "-c", sc.Command)
	if sc.WorkDir != "" {
		cmd.Dir = sc.WorkDir
	} else if e.WorkDir != "" {
		cmd.Dir = e.WorkDir
	}

	for k, v := range sc.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := &fleet.NodeResult{
		Output:   stdout.String(),
		Duration: duration,
	}

	if stderr.Len() > 0 {
		result.Output += "\n" + stderr.String()
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		result.Error = err.Error()
		result.Status = "failure"

		if cmdCtx.Err() != nil {
			result.Status = "timeout"
		}
	} else {
		result.ExitCode = 0
		result.Status = "success"
	}

	return result, nil
}

func (e *ShellExecutor) executeFile(ctx context.Context, data json.RawMessage) (*fleet.NodeResult, error) {
	var fc fleet.FileCommand
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("unmarshal file command: %w", err)
	}

	start := time.Now()

	switch fc.Action {
	case "read":
		content, err := readFileContent(fc.Path)
		if err != nil {
			return &fleet.NodeResult{Error: err.Error(), Status: "failure", ExitCode: 1, Duration: time.Since(start)}, nil
		}
		return &fleet.NodeResult{Output: content, Status: "success", Duration: time.Since(start)}, nil

	case "write":
		if err := writeFileContent(fc.Path, fc.Content, fc.Mode); err != nil {
			return &fleet.NodeResult{Error: err.Error(), Status: "failure", ExitCode: 1, Duration: time.Since(start)}, nil
		}
		return &fleet.NodeResult{Output: "written", Status: "success", Duration: time.Since(start)}, nil

	default:
		return &fleet.NodeResult{Error: fmt.Sprintf("unknown file action: %s", fc.Action), Status: "failure", ExitCode: 1, Duration: time.Since(start)}, nil
	}
}

func readFileContent(path string) (string, error) {
	cmd := exec.Command("cat", path)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func writeFileContent(path, content, mode string) error {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("cat > %s", path))
	cmd.Stdin = bytes.NewBufferString(content)
	return cmd.Run()
}
