// DevOpsClaw - Ultra-lightweight personal AI agent
// License: MIT

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chzyer/readline"

	"github.com/freitascorp/devopsclaw/pkg/agent"
	"github.com/freitascorp/devopsclaw/pkg/bus"
	"github.com/freitascorp/devopsclaw/pkg/logger"
	"github.com/freitascorp/devopsclaw/pkg/providers"
	"github.com/freitascorp/devopsclaw/pkg/tui"
)

func agentCmd() {
	message := ""
	sessionKey := "cli:default"
	modelOverride := ""
	debugMode := false

	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--debug", "-d":
			debugMode = true
			logger.SetLevel(logger.DEBUG)
		case "-m", "--message":
			if i+1 < len(args) {
				message = args[i+1]
				i++
			}
		case "-s", "--session":
			if i+1 < len(args) {
				sessionKey = args[i+1]
				i++
			}
		case "--model", "-model":
			if i+1 < len(args) {
				modelOverride = args[i+1]
				i++
			}
		}
	}

	// In interactive mode, suppress log noise unless --debug is set.
	// Logs go to stderr (via logger init), but we silence INFO/WARN
	// entirely so the terminal stays clean like Claude Code / Gemini CLI.
	if !debugMode {
		logger.SetLevel(logger.ERROR)
	}

	// Initialize renderer
	chat := tui.NewChatRenderer()

	cfg, err := loadConfig()
	if err != nil {
		fmt.Println(chat.RenderError(fmt.Sprintf("Config error: %v", err)))
		os.Exit(1)
	}

	if modelOverride != "" {
		cfg.Agents.Defaults.Model = modelOverride
	}

	provider, modelID, err := providers.CreateProvider(cfg)
	if err != nil {
		fmt.Println(chat.RenderError(fmt.Sprintf("Provider error: %v", err)))
		os.Exit(1)
	}
	if modelID != "" {
		cfg.Agents.Defaults.Model = modelID
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)

	// Set up real-time event rendering — this is what makes it feel like Claude Code.
	// Every tool call, result, and iteration is visible to the user as it happens.
	agentLoop.SetEventCallback(func(event agent.AgentEvent) {
		switch event.Type {
		case agent.EventThinking:
			if event.Iteration > 1 {
				// Show iteration counter for multi-step agentic work
				fmt.Printf("  %s\n", chat.RenderIterationBadge(event.Iteration, event.MaxIter))
			}
		case agent.EventToolCall:
			fmt.Println(chat.RenderToolCall(event.ToolName, event.ToolArgs))
		case agent.EventToolResult:
			if event.ToolOutput != "" {
				fmt.Println(chat.RenderToolOutput(event.ToolOutput, event.IsError))
			}
		case agent.EventToolDenied:
			fmt.Println(chat.RenderToolDenied(event.ToolName, event.DenyReason))
		case agent.EventResponse:
			// Rendered by the main flow after ProcessDirect returns.
			// But show usage stats if available.
			if event.TotalTokens > 0 {
				fmt.Println(chat.RenderUsage(event.PromptTokens, event.CompletionTokens, event.TotalTokens))
			}
		case agent.EventError:
			fmt.Println(chat.RenderError(event.Content))
		}
	})

	// Set up human-in-the-loop confirmation with styled prompt
	agentLoop.SetConfirmCallback(func(toolName string, args map[string]any) bool {
		preview := ""
		switch toolName {
		case "exec":
			if cmd, ok := args["command"].(string); ok {
				preview = cmd
			}
		case "write_file", "edit_file", "append_file":
			if path, ok := args["path"].(string); ok {
				preview = path
			}
		case "web_fetch", "browser":
			if u, ok := args["url"].(string); ok {
				preview = u
			}
		}

		fmt.Print(chat.RenderConfirm(toolName, preview))

		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		answer := strings.TrimSpace(strings.ToLower(line))
		return answer == "" || answer == "y" || answer == "yes"
	})

	// Gather startup info
	startupInfo := agentLoop.GetStartupInfo()
	toolCount := 0
	skillCount := 0
	if t, ok := startupInfo["tools"].(map[string]any); ok {
		if c, ok := t["count"].(int); ok {
			toolCount = c
		}
	}
	if s, ok := startupInfo["skills"].(map[string]any); ok {
		if c, ok := s["available"].(int); ok {
			skillCount = c
		}
	}

	if message != "" {
		// One-shot mode — minimal UI
		ctx := context.Background()
		response, err := agentLoop.ProcessDirect(ctx, message, sessionKey)
		if err != nil {
			fmt.Println(chat.RenderError(err.Error()))
			os.Exit(1)
		}
		fmt.Print(chat.RenderAgentResponse(response))
	} else {
		// Interactive mode — full styled UI
		fmt.Print(chat.RenderBanner(
			formatVersion(),
			cfg.Agents.Defaults.Model,
			toolCount,
			skillCount,
		))
		interactiveMode(agentLoop, sessionKey, chat)
	}
}

func interactiveMode(agentLoop *agent.AgentLoop, sessionKey string, chat *tui.ChatRenderer) {
	// Use ANSI escape for a colored prompt — lipgloss styles don't work with readline.
	prompt := "\033[38;2;135;206;235m❯\033[0m "

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          prompt,
		HistoryFile:     filepath.Join(os.TempDir(), ".devopsclaw_history"),
		HistoryLimit:    500,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Println(chat.RenderError("readline init failed, using simple mode"))
		simpleInteractiveMode(agentLoop, sessionKey, chat)
		return
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt || err == io.EOF {
				fmt.Print(chat.RenderGoodbye())
				return
			}
			fmt.Println(chat.RenderError(err.Error()))
			continue
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			fmt.Print(chat.RenderGoodbye())
			return
		}

		// Show user message
		fmt.Print(chat.RenderUserMessage(input))

		// Start a subtle spinner — it will be interrupted by event output
		var spinnerDone atomic.Bool
		go func() {
			frame := 0
			// Only show spinner before first event arrives
			time.Sleep(300 * time.Millisecond)
			for !spinnerDone.Load() {
				fmt.Printf("\r%s", chat.RenderThinking(frame))
				frame = tui.SpinnerTick(frame)
				time.Sleep(80 * time.Millisecond)
			}
			fmt.Printf("\r%s\r", strings.Repeat(" ", 40))
		}()

		ctx := context.Background()
		response, err := agentLoop.ProcessDirect(ctx, input, sessionKey)

		// Stop spinner
		spinnerDone.Store(true)
		time.Sleep(100 * time.Millisecond)

		if err != nil {
			fmt.Println(chat.RenderError(err.Error()))
			continue
		}

		fmt.Print(chat.RenderAgentResponse(response))
		fmt.Println(chat.RenderDivider())
	}
}

func simpleInteractiveMode(agentLoop *agent.AgentLoop, sessionKey string, chat *tui.ChatRenderer) {
	reader := bufio.NewReader(os.Stdin)
	prompt := "\033[38;2;135;206;235m❯\033[0m "
	for {
		fmt.Print(prompt)
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Print(chat.RenderGoodbye())
				return
			}
			fmt.Println(chat.RenderError(err.Error()))
			continue
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			fmt.Print(chat.RenderGoodbye())
			return
		}

		fmt.Print(chat.RenderUserMessage(input))

		ctx := context.Background()
		response, err := agentLoop.ProcessDirect(ctx, input, sessionKey)
		if err != nil {
			fmt.Println(chat.RenderError(err.Error()))
			continue
		}

		fmt.Print(chat.RenderAgentResponse(response))
		fmt.Println(chat.RenderDivider())
	}
}
