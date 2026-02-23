// Package tui provides terminal UI components for DevOpsClaw.
// agent_chat.go – Claude Code / Gemini CLI–inspired interactive chat renderer.
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// ─── Color palette ──────────────────────────────────────────────────

var (
	colorLobster    = lipgloss.Color("#FF6B6B")
	colorUser       = lipgloss.Color("#87CEEB")
	colorAgent      = lipgloss.Color("#B8BB26")
	colorTool       = lipgloss.Color("#D3869B")
	colorToolBorder = lipgloss.Color("#504945")
	colorDim        = lipgloss.Color("#7C6F64")
	colorSubtle     = lipgloss.Color("#928374")
	colorWarn       = lipgloss.Color("#FE8019")
	colorErr        = lipgloss.Color("#FB4934")
)

// ─── Styles ─────────────────────────────────────────────────────────

var (
	sLobster = lipgloss.NewStyle().Bold(true).Foreground(colorLobster)
	sUser    = lipgloss.NewStyle().Bold(true).Foreground(colorUser)
	sAgent   = lipgloss.NewStyle().Bold(true).Foreground(colorAgent)
	sToolBld = lipgloss.NewStyle().Bold(true).Foreground(colorTool)
	sDim     = lipgloss.NewStyle().Foreground(colorDim)
	sSub     = lipgloss.NewStyle().Foreground(colorSubtle)
	sWarn    = lipgloss.NewStyle().Bold(true).Foreground(colorWarn)
	sErr     = lipgloss.NewStyle().Bold(true).Foreground(colorErr)
	sBorder  = lipgloss.NewStyle().Foreground(colorToolBorder)

	sToolBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorToolBorder).
			Padding(0, 1).
			MarginLeft(4)

	sToolBoxErr = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorErr).
			Padding(0, 1).
			MarginLeft(4)

	spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
)

// ─── Terminal width ─────────────────────────────────────────────────

// TermWidth returns the current terminal width, defaulting to 80.
func TermWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

func thinLine() string {
	return sBorder.Render(strings.Repeat("─", TermWidth()-1))
}

// ─── ChatRenderer ───────────────────────────────────────────────────

// ChatRenderer handles styled output for the agent interactive chat.
type ChatRenderer struct {
	md    *glamour.TermRenderer
	width int
}

// NewChatRenderer creates a renderer with glamour markdown support.
func NewChatRenderer() *ChatRenderer {
	w := TermWidth()
	cw := w - 6
	if cw < 40 {
		cw = 40
	}

	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(cw),
	)
	return &ChatRenderer{md: r, width: w}
}

// ─── Banner ─────────────────────────────────────────────────────────

// RenderBanner returns the styled startup header.
func (c *ChatRenderer) RenderBanner(version, model string, tools, skills int) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(thinLine())
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(sLobster.Render("🦞 DevOpsClaw"))
	b.WriteString(" ")
	b.WriteString(sDim.Render(version))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(sDim.Render(fmt.Sprintf("model  %s", model)))
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(sDim.Render(fmt.Sprintf("tools  %d   skills  %d", tools, skills)))
	b.WriteString("\n\n")
	b.WriteString(thinLine())
	b.WriteString("\n")
	b.WriteString(sDim.Render("  tip: type a message · /help · ctrl-c to quit"))
	b.WriteString("\n\n")
	return b.String()
}

// ─── Messages ───────────────────────────────────────────────────────

// RenderUserMessage formats a user input label (content already on screen via readline).
func (c *ChatRenderer) RenderUserMessage(content string) string {
	ts := sDim.Render(time.Now().Format("15:04"))
	label := sUser.Render("❯ You")
	// Don't re-print the user's text — readline already showed it.
	return fmt.Sprintf("\n%s %s\n", label, ts)
}

// RenderAgentResponse formats and renders the agent's markdown response.
func (c *ChatRenderer) RenderAgentResponse(content string) string {
	ts := sDim.Render(time.Now().Format("15:04"))
	label := sAgent.Render("🦞 DevOpsClaw")

	body := content
	if c.md != nil {
		if rendered, err := c.md.Render(content); err == nil {
			// Glamour adds trailing newlines; trim to control spacing ourselves.
			body = strings.TrimRight(rendered, "\n")
		}
	}

	return fmt.Sprintf("\n%s %s\n%s\n", label, ts, body)
}

// ─── Tool calls ─────────────────────────────────────────────────────

var toolIcons = map[string]string{
	"exec": "⚙", "read_file": "📄", "write_file": "✏️", "edit_file": "✏️",
	"append_file": "✏️", "list_dir": "📂", "web_search": "🔍", "web_fetch": "🌐",
	"browser": "🖥", "message": "💬", "spawn": "🔀", "find_skills": "📚",
	"install_skill": "📦", "i2c": "🔌", "spi": "🔌",
}

// RenderToolCall formats a tool invocation line.
func (c *ChatRenderer) RenderToolCall(name string, args map[string]any) string {
	icon, ok := toolIcons[name]
	if !ok {
		icon = "⚡"
	}
	n := sToolBld.Render(name)
	if preview := toolPreview(name, args); preview != "" {
		return fmt.Sprintf("  %s %s %s", icon, n, sDim.Render(preview))
	}
	return fmt.Sprintf("  %s %s", icon, n)
}

// RenderToolOutput wraps tool output in a bordered box, truncated to 15 lines.
func (c *ChatRenderer) RenderToolOutput(output string, isError bool) string {
	const maxLines = 15
	lines := strings.Split(output, "\n")
	truncated := false
	if len(lines) > maxLines {
		truncated = true
		lines = lines[:maxLines]
	}
	text := strings.Join(lines, "\n")
	if truncated {
		total := len(strings.Split(output, "\n"))
		text += sDim.Render(fmt.Sprintf("\n… %d more lines", total-maxLines))
	}

	if isError {
		return sToolBoxErr.Render(text)
	}
	return sToolBox.Render(text)
}

// RenderToolDenied formats a tool denial message.
func (c *ChatRenderer) RenderToolDenied(name, reason string) string {
	return fmt.Sprintf("  %s %s %s", sErr.Render("✗"), sToolBld.Render(name), sDim.Render(reason))
}

// ─── Confirmation ───────────────────────────────────────────────────

// RenderConfirm formats the human-in-the-loop confirmation prompt.
func (c *ChatRenderer) RenderConfirm(name, preview string) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(sWarn.Render("  ⚠  Allow "))
	b.WriteString(sWarn.Render(name))
	b.WriteString(sWarn.Render("?"))
	if preview != "" {
		b.WriteString("\n")
		b.WriteString(sDim.Render("     " + preview))
	}
	b.WriteString("\n")
	b.WriteString(sDim.Render("  [Y/n] "))
	return b.String()
}

// ─── Status / meta ──────────────────────────────────────────────────

// RenderThinking returns a spinner line for the "thinking…" indicator.
func (c *ChatRenderer) RenderThinking(frame int) string {
	f := spinnerFrames[frame%len(spinnerFrames)]
	return sDim.Render(fmt.Sprintf("  %s thinking…", f))
}

// RenderIterationBadge shows the current agentic loop step.
func (c *ChatRenderer) RenderIterationBadge(iter, max int) string {
	return sSub.Render(fmt.Sprintf("── step %d/%d ──", iter, max))
}

// RenderUsage formats a compact token-usage summary line.
func (c *ChatRenderer) RenderUsage(prompt, completion, total int) string {
	return sDim.Render(fmt.Sprintf(
		"  %s prompt · %s completion · %s total",
		fmtTok(prompt), fmtTok(completion), fmtTok(total),
	))
}

// RenderError formats an error message.
func (c *ChatRenderer) RenderError(msg string) string {
	return sErr.Render("  ✗ " + msg)
}

// RenderDivider returns a subtle horizontal rule.
func (c *ChatRenderer) RenderDivider() string {
	return thinLine()
}

// RenderGoodbye formats the exit message.
func (c *ChatRenderer) RenderGoodbye() string {
	return "\n" + sDim.Render("  👋 Goodbye!") + "\n\n"
}

// ─── Helpers ────────────────────────────────────────────────────────

func toolPreview(name string, args map[string]any) string {
	switch name {
	case "exec":
		if v, ok := args["command"].(string); ok {
			return trunc(v, 60)
		}
	case "read_file", "write_file", "edit_file", "append_file":
		if v, ok := args["path"].(string); ok {
			return v
		}
	case "web_search":
		if v, ok := args["query"].(string); ok {
			return trunc(v, 50)
		}
	case "web_fetch", "browser":
		if v, ok := args["url"].(string); ok {
			return trunc(v, 60)
		}
	case "list_dir":
		if v, ok := args["path"].(string); ok {
			return v
		}
	case "message":
		if v, ok := args["content"].(string); ok {
			return trunc(v, 40)
		}
	}
	return ""
}

func trunc(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func fmtTok(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// SpinnerTick returns the next spinner frame index.
func SpinnerTick(current int) int {
	return (current + 1) % len(spinnerFrames)
}
