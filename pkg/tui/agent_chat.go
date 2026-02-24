// Package tui – agent_chat.go
// Lightweight print-based renderer for one-shot / non-interactive mode.
// Uses shared styles from styles.go (claudechic palette).
// For the full interactive TUI, see chat_app.go.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// ChatRenderer handles styled output for the agent interactive chat (print mode).
type ChatRenderer struct {
	md    *glamour.TermRenderer
	width int
}

// NewChatRenderer creates a renderer with glamour markdown support.
func NewChatRenderer() *ChatRenderer {
	w := MaxContentWidth(TerminalWidth())
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

func thinSep(w int) string {
	sw := w - 4
	if sw < 10 {
		sw = 10
	}
	return PanelText.Render(strings.Repeat("─", sw))
}

// RenderBanner returns the styled startup header with the 🦞 brand logo.
func (c *ChatRenderer) RenderBanner(version, model string, tools, skills int) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(BrandLogo(version))
	b.WriteString("\n")
	b.WriteString(MutedText.Render(fmt.Sprintf("  model  %s", model)))
	b.WriteString("\n")
	b.WriteString(MutedText.Render(fmt.Sprintf("  tools  %d   skills  %d", tools, skills)))
	b.WriteString("\n\n")
	b.WriteString(thinSep(c.width))
	b.WriteString("\n")
	b.WriteString(MutedText.Render("  tip: type a message · /help · ctrl-c to quit"))
	b.WriteString("\n\n")
	return b.String()
}

// RenderUserMessage – thick orange left border, 2-line top margin.
func (c *ChatRenderer) RenderUserMessage(content string) string {
	ts := MutedText.Render(time.Now().Format("15:04"))
	label := PrimaryText.Render("❯ You") + " " + ts
	return "\n\n" + UserBlockStyle.Render(label) + "\n"
}

// RenderAgentResponse – wide blue left border, 1-line margin.
func (c *ChatRenderer) RenderAgentResponse(content string) string {
	body := content
	if c.md != nil {
		if rendered, err := c.md.Render(content); err == nil {
			body = strings.TrimRight(rendered, "\n")
		}
	}
	return "\n" + AssistantBlockStyle.Render(Linkify(body)) + "\n"
}

// RenderSummaryMessage – thick blue left border, 2-line top margin.
func (c *ChatRenderer) RenderSummaryMessage(content string) string {
	body := content
	if c.md != nil {
		if rendered, err := c.md.Render(content); err == nil {
			body = strings.TrimRight(rendered, "\n")
		}
	}
	return "\n\n" + SummaryBlockStyle.Render(body) + "\n"
}

// RenderSystemInfo – thick panel border, panel-colored text.
func (c *ChatRenderer) RenderSystemInfo(msg string) string {
	return "\n" + SystemBlockStyle.Render(msg) + "\n"
}

// RenderSystemWarning – thick warning border, warning-colored text.
func (c *ChatRenderer) RenderSystemWarning(msg string) string {
	return "\n" + SystemWarnBlockStyle.Render(msg) + "\n"
}

// RenderToolCall – wide gray left border, header with triangle-right prefix.
func (c *ChatRenderer) RenderToolCall(name string, args map[string]any) string {
	header := FormatToolHdr(name, args)

	var inner strings.Builder
	inner.WriteString(ToolHdrText.Render(header))

	if name == "exec" {
		if cmd, ok := args["command"].(string); ok {
			inner.WriteString("\n")
			inner.WriteString(MutedText.Render("$ " + cmd))
		}
	}

	return ToolBlockStyle.Render(inner.String())
}

// RenderToolOutput – gray or red border, truncated to 15 lines.
func (c *ChatRenderer) RenderToolOutput(output string, isError bool) string {
	text := TruncateOutput(output, 15)

	summary := FmtResultSummary(output, isError)
	if summary != "" {
		text += "\n" + MutedText.Render(summary)
	}

	text = Linkify(text)

	if isError {
		return ErrorBlockStyle.Render(text)
	}
	return ToolBlockStyle.Render(text)
}

// RenderToolDenied – red left border.
func (c *ChatRenderer) RenderToolDenied(name, reason string) string {
	inner := ErrorText.Render("✗ "+name) + " " + MutedText.Render(reason)
	return ErrorBlockStyle.Render(inner)
}

// RenderConfirm – claudechic base-prompt style: tall primary border, surface bg,
// with individual prompt-option rows for each choice.
func (c *ChatRenderer) RenderConfirm(name, preview string) string {
	w := MaxContentWidth(TerminalWidth())
	return "\n" + RenderConfirmBox(name, preview, w, ConfirmOptYes) + "\n"
}

// RenderThinking – braille spinner "thinking…" in muted text.
func (c *ChatRenderer) RenderThinking(frame int) string {
	f := SpinnerFrameSet[frame%len(SpinnerFrameSet)]
	return MutedText.Render(fmt.Sprintf("  %s thinking…", f))
}

// RenderIterationBadge – step N/M in muted text.
func (c *ChatRenderer) RenderIterationBadge(iter, max int) string {
	return MutedText.Render(fmt.Sprintf("── step %d/%d ──", iter, max))
}

// RenderUsage – compact token-usage summary line.
func (c *ChatRenderer) RenderUsage(prompt, completion, total int) string {
	return MutedText.Render(FmtUsage(prompt, completion, total))
}

// RenderError – red left border with "Error: " prefix.
func (c *ChatRenderer) RenderError(msg string) string {
	return ErrorBlockStyle.Render(ErrorText.Render("Error: " + msg))
}

// RenderDivider – subtle horizontal rule in panel color.
func (c *ChatRenderer) RenderDivider() string {
	return thinSep(c.width)
}

// RenderGoodbye – exit message.
func (c *ChatRenderer) RenderGoodbye() string {
	return "\n" + MutedText.Render("  " + BrandEmoji + " Goodbye!") + "\n\n"
}

// RenderFooter – claudechic StatusFooter: brand + model + context bar.
func (c *ChatRenderer) RenderFooter(model string, contextPct float64) string {
	w := MaxContentWidth(TerminalWidth())
	brand := PrimaryText.Render(BrandEmoji)
	sep := PanelText.Render("·")
	left := brand + " " + sep + " " + MutedText.Render(model)
	right := RenderCtxBar(contextPct)
	gap := w - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}
	return FooterStyle.Width(w).Render(
		"  " + left + strings.Repeat(" ", gap) + right + "  ",
	)
}

// SpinnerTick returns the next spinner frame index (0-9 braille cycle).
func SpinnerTick(current int) int {
	return (current + 1) % len(SpinnerFrameSet)
}
