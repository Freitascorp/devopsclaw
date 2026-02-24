package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// GeminiTool lets the agent invoke Gemini CLI (`gemini`) through the terminal.
// Designed for AI-to-AI delegation: the devopsclaw agent (running on its own LLM)
// sends prompts to Gemini CLI and gets structured responses back.
// Primary use-case: BMAD method workflows where Gemini acts as a specialist sub-agent.
type GeminiTool struct {
	workingDir string
	timeout    time.Duration
	model      string
}

// geminiCLIResponse is the JSON envelope returned by `gemini --output-format json`.
type geminiCLIResponse struct {
	SessionID string `json:"session_id"`
	Response  string `json:"response"`
	Stats     any    `json:"stats,omitempty"`
	Error     *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// NewGeminiTool creates a GeminiTool that calls `gemini` CLI from the given working directory.
func NewGeminiTool(workingDir string) *GeminiTool {
	return &GeminiTool{
		workingDir: workingDir,
		timeout:    0, // No timeout — BMAD workflows can run for a long time. 0 means no limit.
	}
}

func (t *GeminiTool) Name() string {
	return "gemini"
}

func (t *GeminiTool) Description() string {
	return `Invoke Gemini CLI to delegate tasks to Google's Gemini AI through the terminal.
Send a prompt and get a structured response. Gemini runs non-interactively with full autonomy (yolo mode).
Use this for AI-to-AI delegation — e.g. BMAD workflows where Gemini acts as a specialist (Analyst, PM, Architect, Dev, QA).
The prompt should contain all context the Gemini agent needs — it has no memory of prior calls.`
}

func (t *GeminiTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "The full prompt to send to Gemini CLI. Include all context, instructions, and expected output format. Gemini has no memory between calls.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional Gemini model to use (e.g. 'gemini-2.5-flash', 'gemini-2.5-pro'). If empty, uses Gemini CLI's default.",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "Optional working directory for the command. Gemini CLI reads GEMINI.md from this directory for system instructions.",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *GeminiTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	prompt, ok := args["prompt"].(string)
	if !ok || strings.TrimSpace(prompt) == "" {
		return ErrorResult("prompt is required and must be non-empty")
	}

	// Resolve working directory
	cwd := t.workingDir
	if wd, ok := args["working_dir"].(string); ok && wd != "" {
		cwd = wd
	}
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}

	// Resolve model
	model := t.model
	if m, ok := args["model"].(string); ok && m != "" {
		model = m
	}

	// Check gemini CLI is available
	geminiPath, err := exec.LookPath("gemini")
	if err != nil {
		return ErrorResult("gemini CLI not found in PATH. Install with: npm install -g @google/gemini-cli")
	}

	// Build command args
	cmdArgs := []string{
		"-p", prompt,
		"--output-format", "json",
		"--approval-mode", "yolo",
	}
	if model != "" {
		cmdArgs = append(cmdArgs, "-m", model)
	}

	// Set up command with timeout
	var cmdCtx context.Context
	var cancel context.CancelFunc
	if t.timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, t.timeout)
	} else {
		cmdCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, geminiPath, cmdArgs...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	// Inherit environment so Gemini CLI picks up API keys, PATH, etc.
	cmd.Env = os.Environ()

	// Platform-specific process group handling for clean termination
	prepareCommandForTermination(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return ErrorResult(fmt.Sprintf("failed to start gemini CLI: %v", err))
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var execErr error
	select {
	case execErr = <-done:
	case <-cmdCtx.Done():
		_ = terminateProcessTree(cmd)
		select {
		case execErr = <-done:
		case <-time.After(3 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			execErr = <-done
		}
	}

	rawOutput := stdout.String()
	stderrOutput := stderr.String()

	// Handle timeout
	if execErr != nil && cmdCtx.Err() == context.DeadlineExceeded {
		msg := fmt.Sprintf("Gemini CLI timed out after %v", t.timeout)
		if stderrOutput != "" {
			msg += "\nSTDERR: " + stderrOutput
		}
		return &ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	// Try to parse JSON response
	var resp geminiCLIResponse
	if err := json.Unmarshal([]byte(rawOutput), &resp); err == nil {
		// Successfully parsed JSON
		if resp.Error != nil {
			errMsg := fmt.Sprintf("Gemini CLI error: %s (code: %s)", resp.Error.Message, resp.Error.Code)
			return &ToolResult{ForLLM: errMsg, ForUser: errMsg, IsError: true}
		}

		response := resp.Response
		if response == "" {
			response = "(empty response from Gemini)"
		}

		// Truncate very long responses
		maxLen := 50000
		if len(response) > maxLen {
			response = response[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(response)-maxLen)
		}

		// Append continuation hint so the agent knows to keep going
		response += "\n\n[GEMINI TOOL NOTE: If the workflow above is not fully complete, call the gemini tool again to continue. Do not ask the user — proceed autonomously.]"

		// Show the full Gemini response to the user so they can watch the AI-to-AI interaction
		userDisplay := "🤖 **Gemini CLI:**\n\n" + resp.Response

		return &ToolResult{ForLLM: response, ForUser: userDisplay, IsError: false}
	}

	// JSON parse failed — return raw output
	output := rawOutput
	if stderrOutput != "" {
		output += "\nSTDERR:\n" + stderrOutput
	}

	if execErr != nil {
		output += fmt.Sprintf("\nExit code: %v", execErr)
	}

	if output == "" {
		output = "(no output from gemini CLI)"
	}

	// Truncate
	maxLen := 50000
	if len(output) > maxLen {
		output = output[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(output)-maxLen)
	}

	isErr := execErr != nil
	return &ToolResult{ForLLM: output, ForUser: output, IsError: isErr}
}

// SetTimeout configures the maximum duration for a gemini CLI invocation.
func (t *GeminiTool) SetTimeout(timeout time.Duration) {
	t.timeout = timeout
}

// SetModel sets the default model for gemini CLI calls.
func (t *GeminiTool) SetModel(model string) {
	t.model = model
}

// geminiCLIAvailable checks if gemini CLI is installed and accessible.
func geminiCLIAvailable() bool {
	_, err := exec.LookPath("gemini")
	return err == nil
}

// prepareCommandForTermination and terminateProcessTree are defined in
// shell_process_unix.go / shell_process_windows.go (same package).
