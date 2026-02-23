// Package tui – styles.go
// Shared color palette & lipgloss styles.
// Exact port of claudechic/theme.py CHIC_THEME + claudechic/styles.tcss.
package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// ─── Chic color palette ────────────────────────────────────────────────
// Mirror of claudechic/theme.py CHIC_THEME
var (
	ColorPrimary   = lipgloss.Color("#cc7700") // orange – user messages, accents
	ColorSecondary = lipgloss.Color("#5599dd") // sky blue – assistant, strings
	ColorAccent    = lipgloss.Color("#445566") // muted blue-gray – badges, tasks
	ColorPanel     = lipgloss.Color("#555555") // gray – tool borders, separators
	ColorSurface   = lipgloss.Color("#111111") // near-black – subtle backgrounds
	ColorMuted     = lipgloss.Color("#888888") // muted text – timestamps, hints
	ColorWarn      = lipgloss.Color("#aaaa00") // yellow – warnings, caution
	ColorError     = lipgloss.Color("#cc3333") // red – errors, high usage
	ColorText      = lipgloss.Color("#dddddd") // off-white – normal text
	ColorBg        = lipgloss.Color("#000000") // pure black – background
)

// ─── Border types ──────────────────────────────────────────────────────
// claudechic uses Textual border-left widths:
//
//	thick = heavy vertical  ┃  – user messages, errors, system
//	wide  = light vertical  │  – assistant, tools
//	tall  = right half-block ▐  – confirm prompts, todos
var (
	ThickBorder = lipgloss.Border{Left: "┃"}
	WideBorder  = lipgloss.Border{Left: "│"}
	TallBorder  = lipgloss.Border{Left: "▐"}
)

// ─── Block styles ──────────────────────────────────────────────────────
// Each mirrors the CSS from claudechic/styles.tcss exactly.

// UserBlockStyle – ChatMessage.user-message { border-left: thick $primary; margin: 2 0 2 0; }
var UserBlockStyle = lipgloss.NewStyle().
	Border(ThickBorder).
	BorderLeft(true).BorderTop(false).BorderBottom(false).BorderRight(false).
	BorderForeground(ColorPrimary).
	PaddingLeft(1)

// AssistantBlockStyle – ChatMessage.assistant-message { border-left: wide $secondary; margin: 1 0 1 0; }
var AssistantBlockStyle = lipgloss.NewStyle().
	Border(WideBorder).
	BorderLeft(true).BorderTop(false).BorderBottom(false).BorderRight(false).
	BorderForeground(ColorSecondary).
	PaddingLeft(1)

// SummaryBlockStyle – ChatMessage.summary { border-left: thick $secondary; margin: 2 0 1 0; }
var SummaryBlockStyle = lipgloss.NewStyle().
	Border(ThickBorder).
	BorderLeft(true).BorderTop(false).BorderBottom(false).BorderRight(false).
	BorderForeground(ColorSecondary).
	PaddingLeft(1)

// ToolBlockStyle – BaseToolWidget { border-left: wide $panel; margin: 0; }
var ToolBlockStyle = lipgloss.NewStyle().
	Border(WideBorder).
	BorderLeft(true).BorderTop(false).BorderBottom(false).BorderRight(false).
	BorderForeground(ColorPanel).
	PaddingLeft(1)

// ErrorBlockStyle – ErrorMessage { border-left: thick $error; background: $error 10%; }
var ErrorBlockStyle = lipgloss.NewStyle().
	Border(ThickBorder).
	BorderLeft(true).BorderTop(false).BorderBottom(false).BorderRight(false).
	BorderForeground(ColorError).
	PaddingLeft(1)

// SystemBlockStyle – SystemInfo { border-left: thick $panel; color: $panel; }
var SystemBlockStyle = lipgloss.NewStyle().
	Border(ThickBorder).
	BorderLeft(true).BorderTop(false).BorderBottom(false).BorderRight(false).
	BorderForeground(ColorPanel).
	PaddingLeft(1).
	Foreground(ColorPanel)

// SystemWarnBlockStyle – SystemInfo.system-warning { border-left: thick $warning; color: $warning; }
var SystemWarnBlockStyle = lipgloss.NewStyle().
	Border(ThickBorder).
	BorderLeft(true).BorderTop(false).BorderBottom(false).BorderRight(false).
	BorderForeground(ColorWarn).
	PaddingLeft(1).
	Foreground(ColorWarn)

// ConfirmBlockStyle – .base-prompt { background: $surface; border-left: tall $primary; padding: 1 2 1 1; max-width: 90; }
var ConfirmBlockStyle = lipgloss.NewStyle().
	Border(TallBorder).
	BorderLeft(true).BorderTop(false).BorderBottom(false).BorderRight(false).
	BorderForeground(ColorPrimary).
	PaddingLeft(1).PaddingRight(2).PaddingTop(1).PaddingBottom(1).
	Background(ColorSurface)

// PromptOptionStyle – .prompt-option { border-left: tall $panel; color: $text-muted; padding: 0 0 0 1; }
var PromptOptionStyle = lipgloss.NewStyle().
	Border(TallBorder).
	BorderLeft(true).BorderTop(false).BorderBottom(false).BorderRight(false).
	BorderForeground(ColorPanel).
	PaddingLeft(1).
	Foreground(ColorMuted)

// PromptOptionSelectedStyle – .prompt-option.selected { border-left: tall $primary; color: $text; background: $surface-lighten-1; }
var PromptOptionSelectedStyle = lipgloss.NewStyle().
	Border(TallBorder).
	BorderLeft(true).BorderTop(false).BorderBottom(false).BorderRight(false).
	BorderForeground(ColorPrimary).
	PaddingLeft(1).
	Foreground(ColorText).
	Background(lipgloss.Color("#222222")) // $surface-lighten-1

// PromptTitleStyle – .prompt-title { color: $text; padding: 0 0 1 0; }
var PromptTitleStyle = lipgloss.NewStyle().
	Foreground(ColorText).
	PaddingBottom(1)

// ─── Text styles ───────────────────────────────────────────────────────

var (
	PrimaryText   = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	SecondaryText = lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary)
	MutedText     = lipgloss.NewStyle().Foreground(ColorMuted)
	AccentText    = lipgloss.NewStyle().Foreground(ColorAccent)
	WarnText      = lipgloss.NewStyle().Bold(true).Foreground(ColorWarn)
	ErrorText     = lipgloss.NewStyle().Bold(true).Foreground(ColorError)
	NormalText    = lipgloss.NewStyle().Foreground(ColorText)
	PanelText     = lipgloss.NewStyle().Foreground(ColorPanel)
	ToolHdrText   = lipgloss.NewStyle().Foreground(ColorMuted)
)

// ─── Input box ─────────────────────────────────────────────────────────
// #input-container { border: solid $panel; background: $surface; }
// #input-container:focus-within { border: solid $primary; }

var InputBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.NormalBorder()).
	BorderForeground(ColorPanel).
	Background(ColorSurface).
	Padding(0, 1)

var InputBoxFocusedStyle = lipgloss.NewStyle().
	Border(lipgloss.NormalBorder()).
	BorderForeground(ColorPrimary).
	Background(ColorSurface).
	Padding(0, 1)

// ─── Footer bar ────────────────────────────────────────────────────────
// StatusFooter { dock: bottom; height: 1; background: $surface; }
var FooterStyle = lipgloss.NewStyle().
	Background(ColorSurface).
	Foreground(ColorMuted)

// ─── Spinner ───────────────────────────────────────────────────────────
// Braille spinner from claudechic/widgets/primitives/spinner.py
var SpinnerFrameSet = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ─── Dimension helpers ─────────────────────────────────────────────────

// TerminalWidth returns the current terminal width, defaulting to 80.
func TerminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// TerminalHeight returns the current terminal height, defaulting to 24.
func TerminalHeight() int {
	_, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || h <= 0 {
		return 24
	}
	return h
}

// MaxContentWidth caps content at 100 columns (claudechic #chat-column max-width: 100).
func MaxContentWidth(termW int) int {
	if termW > 100 {
		return 100
	}
	return termW
}
