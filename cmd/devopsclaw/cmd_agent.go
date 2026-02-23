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
	"sync"
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
	//
	// spinnerActive is shared with the spinner goroutine so event output
	// can clear the spinner line before printing, preventing overlap.
	var spinnerActive atomic.Bool

	clearSpinnerLine := func() {
		if spinnerActive.Load() {
			// Erase the current line the spinner wrote on
			fmt.Printf("\r\033[K")
		}
	}

	agentLoop.SetEventCallback(func(event agent.AgentEvent) {
		switch event.Type {
		case agent.EventThinking:
			if event.Iteration > 1 {
				clearSpinnerLine()
				fmt.Printf("  %s\n", chat.RenderIterationBadge(event.Iteration, event.MaxIter))
			}
		case agent.EventToolCall:
			clearSpinnerLine()
			fmt.Println(chat.RenderToolCall(event.ToolName, event.ToolArgs))
		case agent.EventToolResult:
			if event.ToolOutput != "" {
				clearSpinnerLine()
				fmt.Println(chat.RenderToolOutput(event.ToolOutput, event.IsError))
			}
		case agent.EventToolDenied:
			clearSpinnerLine()
			fmt.Println(chat.RenderToolDenied(event.ToolName, event.DenyReason))
		case agent.EventResponse:
			if event.TotalTokens > 0 {
				clearSpinnerLine()
				fmt.Println(chat.RenderUsage(event.PromptTokens, event.CompletionTokens, event.TotalTokens))
			}
		case agent.EventError:
			clearSpinnerLine()
			fmt.Println(chat.RenderError(event.Content))
		}
	})

	// Set up human-in-the-loop confirmation with styled prompt
	agentLoop.SetConfirmCallback(func(toolName string, args map[string]any) agent.ConfirmResult {
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

		clearSpinnerLine()
		fmt.Print(chat.RenderConfirm(toolName, preview))

		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		answer := strings.TrimSpace(strings.ToLower(line))
		switch answer {
		case "", "y", "yes":
			return agent.ConfirmAllow
		case "a", "always":
			return agent.ConfirmAllowSession
		default:
			return agent.ConfirmDeny
		}
	})

	// Gather startup info (used for one-shot banner, not needed for TUI mode)
	startupInfo := agentLoop.GetStartupInfo()
	_ = startupInfo

	if message != "" {
		// One-shot mode — minimal UI (print-based)
		ctx := context.Background()
		response, err := agentLoop.ProcessDirect(ctx, message, sessionKey)
		if err != nil {
			fmt.Println(chat.RenderError(err.Error()))
			os.Exit(1)
		}
		fmt.Print(chat.RenderAgentResponse(response))
	} else {
		// Interactive mode — full-screen Bubble Tea TUI (claudechic replica)
		interactiveModeTUI(agentLoop, sessionKey, cfg.Agents.Defaults.Model)
	}
}

// interactiveModeTUI launches the full-screen Bubble Tea chat, replicating claudechic.
func interactiveModeTUI(agentLoop *agent.AgentLoop, sessionKey, modelName string) {
	p, promptCh := tui.RunChatApp(modelName)

	// Wire agent events → Bubble Tea messages
	agentLoop.SetEventCallback(func(event agent.AgentEvent) {
		switch event.Type {
		case agent.EventThinking:
			p.Send(tui.ThinkingMsg{Active: true})
			if event.Iteration > 1 {
				p.Send(tui.AppendChatMsg{Msg: tui.ChatMsg{
					Role:    "system",
					Content: fmt.Sprintf("── step %d/%d ──", event.Iteration, event.MaxIter),
					Time:    time.Now(),
				}})
			}
		case agent.EventToolCall:
			p.Send(tui.AppendChatMsg{Msg: tui.ChatMsg{
				Role:     "tool",
				ToolName: event.ToolName,
				ToolArgs: event.ToolArgs,
				Time:     time.Now(),
			}})
		case agent.EventToolResult:
			if event.ToolOutput != "" {
				p.Send(tui.AppendChatMsg{Msg: tui.ChatMsg{
					Role:    "tool",
					Content: event.ToolOutput,
					IsError: event.IsError,
					Time:    time.Now(),
				}})
			}
		case agent.EventToolDenied:
			p.Send(tui.AppendChatMsg{Msg: tui.ChatMsg{
				Role:    "error",
				Content: fmt.Sprintf("✗ %s – %s", event.ToolName, event.DenyReason),
				Time:    time.Now(),
			}})
		case agent.EventResponse:
			if event.TotalTokens > 0 {
				p.Send(tui.UsageMsg{
					Prompt:     event.PromptTokens,
					Completion: event.CompletionTokens,
					Total:      event.TotalTokens,
				})
			}
		case agent.EventError:
			p.Send(tui.AppendChatMsg{Msg: tui.ChatMsg{
				Role:    "error",
				Content: event.Content,
				Time:    time.Now(),
			}})
		}
	})

	// Wire confirmation requests → Bubble Tea confirm prompt
	agentLoop.SetConfirmCallback(func(toolName string, args map[string]any) agent.ConfirmResult {
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

		var result agent.ConfirmResult
		var wg sync.WaitGroup
		wg.Add(1)

		p.Send(tui.ConfirmRequestMsg{
			ToolName: toolName,
			Preview:  preview,
			Respond: func(choice tui.ConfirmChoice) {
				switch choice {
				case tui.ConfirmYes:
					result = agent.ConfirmAllow
				case tui.ConfirmNo:
					result = agent.ConfirmDeny
				case tui.ConfirmAlwaysSession:
					result = agent.ConfirmAllowSession
				}
				wg.Done()
			},
		})

		wg.Wait()
		return result
	})

	// Goroutine: read prompts from TUI and feed to agent loop
	go func() {
		for text := range promptCh {
			p.Send(tui.ThinkingMsg{Active: true})

			ctx := context.Background()
			response, err := agentLoop.ProcessDirect(ctx, text, sessionKey)

			p.Send(tui.ThinkingMsg{Active: false})

			if err != nil {
				p.Send(tui.AppendChatMsg{Msg: tui.ChatMsg{
					Role:    "error",
					Content: err.Error(),
					Time:    time.Now(),
				}})
				continue
			}

			p.Send(tui.AppendChatMsg{Msg: tui.ChatMsg{
				Role:    "assistant",
				Content: response,
				Time:    time.Now(),
			}})
		}
	}()

	// Run the Bubble Tea program (blocks until quit)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}

// interactiveModeReadline is the fallback readline-based interactive mode.
func interactiveModeReadline(agentLoop *agent.AgentLoop, sessionKey string, chat *tui.ChatRenderer, spinnerActive *atomic.Bool) {
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

		// Start a subtle spinner
		var spinnerDone atomic.Bool
		spinnerActive.Store(true)
		go func() {
			frame := 0
			time.Sleep(300 * time.Millisecond)
			for !spinnerDone.Load() {
				fmt.Printf("\r\033[K%s", chat.RenderThinking(frame))
				frame = tui.SpinnerTick(frame)
				time.Sleep(80 * time.Millisecond)
			}
			fmt.Printf("\r\033[K")
		}()

		ctx := context.Background()
		response, err := agentLoop.ProcessDirect(ctx, input, sessionKey)

		spinnerDone.Store(true)
		spinnerActive.Store(false)
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
