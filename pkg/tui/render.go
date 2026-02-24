// Package tui – render.go
// Shared rendering helpers used by both the print-based ChatRenderer
// (agent_chat.go) and the Bubble Tea ChatApp (chat_app.go).
package tui

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// ─── OSC 8 clickable hyperlinks ────────────────────────────────────────

// urlRe matches http/https URLs in text for OSC 8 wrapping.
var urlRe = regexp.MustCompile(`https?://[^\s\)\]>"'` + "`" + `]+`)

// Linkify wraps bare http/https URLs in OSC 8 escape sequences so they
// become clickable in terminals that support hyperlinks (iTerm2, Ghostty,
// WezTerm, Windows Terminal, GNOME Terminal ≥ 3.26, etc.).
// Format: ESC ] 8 ; params ; URI ST  text  ESC ] 8 ; ; ST
func Linkify(s string) string {
	return urlRe.ReplaceAllStringFunc(s, func(u string) string {
		return "\x1b]8;;" + u + "\x1b\\" + u + "\x1b]8;;\x1b\\"
	})
}

// ─── ANSI state propagation ────────────────────────────────────────────

// ansiSeqRe matches ANSI CSI sequences (e.g. \x1b[38;2;204;119;0m).
var ansiSeqRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// PropagateANSI ensures every line in a multi-line string is
// ANSI-self-contained. The viewport splits content by \n and displays
// an arbitrary window of lines. Without propagation, lines that depend
// on an ANSI open code from a previous (now off-screen) line appear
// as plain/missing text when scrolled to.
//
// Algorithm: walk lines, track the cumulative ANSI state, prepend it
// to each line, and append a reset so the next line starts clean.
func PropagateANSI(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 1 {
		return s
	}

	const reset = "\x1b[0m"
	var activeState string // accumulated ANSI SGR sequences

	for i, line := range lines {
		// Prepend the carried-over state to this line (if any)
		if activeState != "" {
			lines[i] = activeState + line
		}

		// Walk this line's ANSI sequences to update activeState.
		// A reset (\x1b[0m) clears the state; everything else accumulates.
		seqs := ansiSeqRe.FindAllString(line, -1)
		for _, seq := range seqs {
			if seq == reset {
				activeState = ""
			} else {
				activeState += seq
			}
		}

		// Append reset so the viewport can safely slice at any boundary
		if activeState != "" {
			lines[i] += reset
		}
	}

	return strings.Join(lines, "\n")
}

// ─── Confirm prompt ────────────────────────────────────────────────────

// ConfirmOption index constants.
const (
	ConfirmOptYes    = 0
	ConfirmOptAlways = 1
	ConfirmOptNo     = 2
	confirmOptCount  = 3
)

// confirmOptionDef describes a single selectable option.
type confirmOptionDef struct {
	Key   string
	Label string
	Color lipgloss.Color
}

var confirmOptions = [confirmOptCount]confirmOptionDef{
	{Key: "y", Label: "Yes, allow this time", Color: ColorSecondary},
	{Key: "a", Label: "Always allow in this session", Color: ColorPrimary},
	{Key: "n", Label: "No, deny", Color: ColorError},
}

// RenderConfirmBox builds a claudechic-style confirm prompt.
// selectedIdx (0-2) controls which row has the highlight; pass -1 for no highlight.
func RenderConfirmBox(name, preview string, w, selectedIdx int) string {
	if w > 90 {
		w = 90
	}
	optW := w - 8 // account for outer border + padding + pointer

	title := PromptTitleStyle.Render(BrandEmoji + " Allow " + name + "?")
	var previewLine string
	if preview != "" {
		previewLine = MutedText.Render("  " + preview)
	}

	var inner strings.Builder
	inner.WriteString(title)
	if previewLine != "" {
		inner.WriteString("\n" + previewLine)
	}
	inner.WriteString("\n")

	for i, opt := range confirmOptions {
		keyStyle := lipgloss.NewStyle().Bold(true).Foreground(opt.Color)
		if i == selectedIdx {
			pointer := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("▸ ")
			row := PromptOptionSelectedStyle.Width(optW).Render(
				keyStyle.Render(opt.Key) + "  " + NormalText.Render(opt.Label))
			inner.WriteString(pointer + row)
		} else {
			pointer := MutedText.Render("  ")
			row := PromptOptionStyle.Width(optW).Render(
				keyStyle.Render(opt.Key) + "  " + MutedText.Render(opt.Label))
			inner.WriteString(pointer + row)
		}
		if i < confirmOptCount-1 {
			inner.WriteString("\n")
		}
	}

	inner.WriteString("\n")
	inner.WriteString(MutedText.Render("  ↑↓ navigate · enter select · y/a/n shortcut"))

	return ConfirmBlockStyle.Width(w).Render(inner.String())
}

// ─── Standalone blocking confirm (for print-based / readline modes) ───

// RunConfirmPrompt enters raw terminal mode and renders an interactive
// confirm prompt the user can navigate with ↑/↓ arrows and select with Enter.
// Returns the selected option index (ConfirmOptYes / ConfirmOptAlways / ConfirmOptNo).
// Falls back to simple line-read if raw mode fails.
func RunConfirmPrompt(toolName, preview string) int {
	w := MaxContentWidth(TerminalWidth())
	selected := ConfirmOptYes // start on "Yes"

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Fallback: just print and read a byte
		fmt.Print(RenderConfirmBox(toolName, preview, w, selected))
		return readSimpleConfirmKey()
	}
	defer term.Restore(fd, oldState)

	// Count the lines our box occupies so we can redraw in-place.
	render := func() string {
		return "\n" + RenderConfirmBox(toolName, preview, w, selected) + "\n"
	}

	// Initial draw
	out := render()
	lines := strings.Count(out, "\n")
	fmt.Print(out)

	buf := make([]byte, 8)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			break
		}
		b := buf[:n]

		switch {
		// Enter
		case b[0] == '\r' || b[0] == '\n':
			// Move cursor below the box before returning
			fmt.Print("\r\n")
			return selected

		// Up arrow: ESC [ A
		case n >= 3 && b[0] == 0x1b && b[1] == '[' && b[2] == 'A':
			if selected > 0 {
				selected--
			}
		// Down arrow: ESC [ B
		case n >= 3 && b[0] == 0x1b && b[1] == '[' && b[2] == 'B':
			if selected < confirmOptCount-1 {
				selected++
			}
		// 'k' / 'j' (vim)
		case b[0] == 'k' || b[0] == 'K':
			if selected > 0 {
				selected--
			}
		case b[0] == 'j' || b[0] == 'J':
			if selected < confirmOptCount-1 {
				selected++
			}
		// Shortcut keys (select and return immediately)
		case b[0] == 'y' || b[0] == 'Y':
			fmt.Print("\r\n")
			return ConfirmOptYes
		case b[0] == 'a' || b[0] == 'A':
			fmt.Print("\r\n")
			return ConfirmOptAlways
		case b[0] == 'n' || b[0] == 'N':
			fmt.Print("\r\n")
			return ConfirmOptNo
		// Ctrl-C / Escape → deny
		case b[0] == 0x03 || b[0] == 0x1b && n == 1:
			fmt.Print("\r\n")
			return ConfirmOptNo

		default:
			continue
		}

		// Redraw: move cursor up, clear, reprint
		for i := 0; i < lines; i++ {
			fmt.Print("\033[A") // cursor up
		}
		fmt.Print("\r\033[J") // clear from cursor to end of screen
		out = render()
		lines = strings.Count(out, "\n")
		fmt.Print(out)
	}

	return selected
}

// readSimpleConfirmKey is the fallback when raw mode is unavailable.
func readSimpleConfirmKey() int {
	buf := make([]byte, 16)
	n, _ := os.Stdin.Read(buf)
	if n > 0 {
		switch buf[0] {
		case 'y', 'Y', '\n', '\r':
			return ConfirmOptYes
		case 'a', 'A':
			return ConfirmOptAlways
		case 'n', 'N':
			return ConfirmOptNo
		}
	}
	return ConfirmOptNo
}

// ─── Context bar ───────────────────────────────────────────────────────
// Mirrors claudechic ContextBar indicator.

// RenderCtxBar renders a 10-char context-usage bar with embedded percentage.
func RenderCtxBar(pct float64) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	barWidth := 10
	filled := int(pct * float64(barWidth))

	var fillColor, emptyColor lipgloss.Color
	switch {
	case pct < 0.5:
		fillColor = lipgloss.Color("#666666")
	case pct < 0.8:
		fillColor = ColorWarn
	default:
		fillColor = ColorError
	}
	emptyColor = lipgloss.Color("#333333")

	var bar strings.Builder
	pctStr := fmt.Sprintf("%2.0f%%", pct*100)
	start := (barWidth - len(pctStr)) / 2

	for i := 0; i < barWidth; i++ {
		bg := emptyColor
		if i < filled {
			bg = fillColor
		}
		fg := ColorText
		if start <= i && i < start+len(pctStr) {
			ch := string(pctStr[i-start])
			bar.WriteString(lipgloss.NewStyle().
				Foreground(fg).Background(bg).Render(ch))
		} else {
			bar.WriteString(lipgloss.NewStyle().
				Background(bg).Render(" "))
		}
	}
	return bar.String()
}

// ─── Formatting helpers ────────────────────────────────────────────────
// Ported from claudechic/formatting.py

// FormatToolHdr builds a one-line tool header like "▸ exec ls -la".
func FormatToolHdr(name string, args map[string]any) string {
	switch name {
	case "exec":
		if d, ok := args["description"].(string); ok && d != "" {
			return "▸ " + name + " " + TruncStr(d, 60)
		}
		if cmd, ok := args["command"].(string); ok {
			return "▸ " + name + " " + TruncStr(cmd, 60)
		}
	case "read_file", "write_file", "edit_file", "append_file", "list_dir":
		if p, ok := args["path"].(string); ok {
			return "▸ " + name + " " + p
		}
	case "web_search":
		if q, ok := args["query"].(string); ok {
			return "▸ " + name + " " + TruncStr(q, 60)
		}
	case "web_fetch", "browser":
		if u, ok := args["url"].(string); ok {
			return "▸ " + name + " " + TruncStr(u, 60)
		}
	case "message":
		if ct, ok := args["content"].(string); ok {
			return "▸ " + name + " " + TruncStr(ct, 40)
		}
	case "spawn":
		if d, ok := args["description"].(string); ok && d != "" {
			return "▸ " + name + " " + TruncStr(d, 50)
		}
	}
	return "▸ " + name
}

// FmtResultSummary returns a short "(N lines)" or "(error)" label.
func FmtResultSummary(content string, isError bool) string {
	if isError {
		return "(error)"
	}
	stripped := strings.TrimSpace(content)
	if stripped == "" {
		return "(no output)"
	}
	lines := strings.Split(stripped, "\n")
	return fmt.Sprintf("(%d lines)", len(lines))
}

// TruncateOutput truncates multi-line output to maxLines.
func TruncateOutput(output string, maxLines int) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}
	total := len(lines)
	text := strings.Join(lines[:maxLines], "\n")
	text += "\n" + MutedText.Render(fmt.Sprintf("… %d more lines", total-maxLines))
	return text
}

// TruncStr truncates a string to max characters with "…" suffix.
func TruncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// FmtTokCount formats a token count as "1.2k", "3.4M", etc.
func FmtTokCount(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// FmtUsage returns a compact token-usage summary.
func FmtUsage(prompt, completion, total int) string {
	return fmt.Sprintf(
		"  %s prompt · %s completion · %s total",
		FmtTokCount(prompt), FmtTokCount(completion), FmtTokCount(total),
	)
}
