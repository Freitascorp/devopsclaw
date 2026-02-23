package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/freitascorp/devopsclaw/pkg/fleet"
)

// relayDenyPatterns mirrors the deny patterns from the local exec tool.
// These block destructive or privilege-escalating commands from the fleet relay.
var relayDenyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+-[rf]{1,2}\b`),
	regexp.MustCompile(`\b(format|mkfs|diskpart)\b\s`),
	regexp.MustCompile(`\bdd\s+if=`),
	regexp.MustCompile(`>\s*/dev/sd[a-z]\b`),
	regexp.MustCompile(`\b(shutdown|reboot|poweroff)\b`),
	regexp.MustCompile(`:\(\)\s*\{.*\};\s*:`),
	regexp.MustCompile(`\|\s*(sh|bash)\b`),
	regexp.MustCompile(`\bsudo\b`),
	regexp.MustCompile(`\bcurl\b.*\|\s*(sh|bash)`),
	regexp.MustCompile(`\bwget\b.*\|\s*(sh|bash)`),
	regexp.MustCompile(`\beval\b`),
	regexp.MustCompile(`\bsource\s+.*\.sh\b`),
}

// ShellExecutor implements LocalExecutor by running shell commands locally.
type ShellExecutor struct {
	WorkDir      string
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

// guardRelayCommand checks a command against the relay deny patterns.
// Returns an error string if blocked, empty string if allowed.
func guardRelayCommand(command string) string {
	lower := strings.ToLower(strings.TrimSpace(command))
	for _, pattern := range relayDenyPatterns {
		if pattern.MatchString(lower) {
			return "command blocked by relay safety guard (dangerous pattern detected)"
		}
	}
	return ""
}

func (e *ShellExecutor) executeShell(ctx context.Context, data json.RawMessage) (*fleet.NodeResult, error) {
	var sc fleet.ShellCommand
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("unmarshal shell command: %w", err)
	}

	// Guard: deny dangerous commands
	if guardErr := guardRelayCommand(sc.Command); guardErr != "" {
		return &fleet.NodeResult{
			Error:    guardErr,
			Status:   "blocked",
			ExitCode: -1,
		}, nil
	}

	// Enforce maximum timeout of 120 seconds
	timeout := 30 * time.Second
	if sc.TimeoutSec > 0 {
		timeout = time.Duration(sc.TimeoutSec) * time.Second
	}
	maxTimeout := 120 * time.Second
	if timeout > maxTimeout {
		timeout = maxTimeout
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	shell := sc.Shell
	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := exec.CommandContext(cmdCtx, shell, "-c", sc.Command)

	// Validate working directory — prevent traversal
	workDir := e.WorkDir
	if sc.WorkDir != "" {
		if strings.Contains(sc.WorkDir, "..") {
			return &fleet.NodeResult{
				Error:    "command blocked by relay safety guard (path traversal in work_dir)",
				Status:   "blocked",
				ExitCode: -1,
			}, nil
		}
		workDir = sc.WorkDir
	}
	if workDir != "" {
		cmd.Dir = workDir
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

	// Truncate output to prevent memory exhaustion
	const maxOutput = 10000
	if len(result.Output) > maxOutput {
		result.Output = result.Output[:maxOutput] + fmt.Sprintf("\n... (truncated, %d more chars)", len(result.Output)-maxOutput)
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

	// Validate path: reject traversal and non-local paths
	if err := validateFilePath(fc.Path); err != nil {
		return &fleet.NodeResult{
			Error:    fmt.Sprintf("path blocked by safety guard: %v", err),
			Status:   "blocked",
			ExitCode: -1,
			Duration: 0,
		}, nil
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

// validateFilePath rejects path traversal, absolute paths to sensitive areas, etc.
func validateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	// Block traversal
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal not allowed")
	}
	// Block sensitive system paths
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	blockedPrefixes := []string{"/etc/shadow", "/etc/passwd", "/root", "/proc", "/sys"}
	for _, prefix := range blockedPrefixes {
		if strings.HasPrefix(absPath, prefix) {
			return fmt.Errorf("access to %s is not allowed", prefix)
		}
	}
	return nil
}

// readFileContent reads a file using os.ReadFile (no shell involved).
func readFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// Limit read size to 1MB
	const maxSize = 1024 * 1024
	if len(data) > maxSize {
		return string(data[:maxSize]), fmt.Errorf("file truncated at %d bytes", maxSize)
	}
	return string(data), nil
}

// writeFileContent writes a file using os.WriteFile (no shell involved).
// This eliminates the command injection vulnerability of the old shell-based approach.
func writeFileContent(path, content, mode string) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Parse file mode, default to 0644
	perm := os.FileMode(0o644)
	if mode != "" {
		parsed, err := strconv.ParseUint(mode, 8, 32)
		if err == nil {
			perm = os.FileMode(parsed)
		}
	}

	return os.WriteFile(path, []byte(content), perm)
}
