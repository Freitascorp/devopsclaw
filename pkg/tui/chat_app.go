// Package tui – chat_app.go
// Full-screen Bubble Tea chat TUI — faithful Go replica of claudechic.
//
// Layout (mirrors claudechic/screens/chat.py + styles.tcss):
//
//	┌──────────────────────────────────────────┐
//	│  (scrollable chat view)                  │
//	│  ┃ user message (thick orange border)    │
//	│  │ assistant reply (wide blue border)    │
//	│  │ tool call (wide gray border)          │
//	│  ┃ error (thick red border)              │
//	│                                          │
//	│  ⠋ thinking…                             │
//	│  ┌────────────────────────────────────┐  │
//	│  │ > input area                       │  │
//	│  └────────────────────────────────────┘  │
//	│  model · Auto-edit: off    ░░░░ 12%      │
//	└──────────────────────────────────────────┘
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// ─── Message types ─────────────────────────────────────────────────────

// ChatMsg represents a message in the chat history.
type ChatMsg struct {
	Role    string         // "user", "assistant", "tool", "error", "system", "system-warn", "summary", "confirm"
	Content string         // text content or markdown
	ToolName string        // for tool messages
	ToolArgs map[string]any // for tool messages
	IsError  bool          // for tool results
	Time     time.Time
}

// AppendChatMsg is a tea.Msg that adds a message to the chat.
type AppendChatMsg struct{ Msg ChatMsg }

// ThinkingMsg toggles the thinking indicator.
type ThinkingMsg struct{ Active bool }

// ContextUpdateMsg updates context usage in the footer.
type ContextUpdateMsg struct{ Pct float64 }

// UsageMsg shows token usage.
type UsageMsg struct{ Prompt, Completion, Total int }

// PlanUpdateMsg updates the visible task plan.
type PlanUpdateMsg struct{ Steps []PlanDisplayStep }

// PlanDisplayStep is a UI-friendly version of a plan step.
type PlanDisplayStep struct {
	ID     int
	Title  string
	Status string // "not-started", "in-progress", "completed"
}

// ResponseDoneMsg signals the agent finished responding.
type ResponseDoneMsg struct{}

// ConfirmRequestMsg asks the user for tool confirmation.
type ConfirmRequestMsg struct {
	ToolName string
	Preview  string
	Respond  func(ConfirmChoice)
}

// ConfirmChoice is the user's answer to a confirm prompt.
type ConfirmChoice int

const (
	ConfirmYes ConfirmChoice = iota
	ConfirmNo
	ConfirmAlwaysSession
)

// SendPromptMsg is emitted when the user presses Enter.
type SendPromptMsg struct{ Text string }

// tickMsg drives the spinner animation.
type spinTickMsg time.Time

// ─── Main model ────────────────────────────────────────────────────────

// ChatApp is the Bubble Tea model for the agent chat interface.
// It closely mirrors claudechic's ChatScreen + ChatApp composition.
type ChatApp struct {
	// Dimensions
	width, height int

	// Chat history (rendered blocks)
	messages []ChatMsg
	chatView viewport.Model // scrollable viewport

	// Input
	input    textarea.Model
	focused  bool

	// State
	thinking      bool
	spinnerFrame  int
	contextPct    float64
	model         string
	permMode      string // "default", "acceptEdits", "plan"
	quitting      bool

	// Confirm state
	confirmActive  bool
	confirmIdx     int // 0=yes, 1=always, 2=no — navigated with arrows
	confirmName    string
	confirmPreview string
	confirmCb      func(ConfirmChoice)

	// Tool detail view: false = collapsed (default), true = expanded
	toolsExpanded bool

	// Plan tracking (Copilot-style task plan)
	planSteps    []PlanDisplayStep
	planExpanded bool // collapsible plan panel

	// Rendering
	md *glamour.TermRenderer

	// Prompt channel (user input goes here for external consumer)
	promptCh chan<- string

	// Callbacks
	OnSend func(text string) // called when user submits input
}

// NewChatApp creates a fully initialized chat TUI model.
func NewChatApp(modelName string) ChatApp {
	// Textarea input — bottom-docked, like claudechic #input
	ti := textarea.New()
	ti.Placeholder = "Type a message · /help · ctrl-c to quit"
	ti.CharLimit = 0
	ti.SetHeight(1)
	ti.ShowLineNumbers = false
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ti.FocusedStyle.Base = lipgloss.NewStyle().Foreground(ColorText)
	ti.BlurredStyle.Base = lipgloss.NewStyle().Foreground(ColorMuted)
	ti.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(ColorMuted)
	ti.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(ColorMuted)
	ti.Focus()

	// Viewport for chat messages
	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle()

	// Glamour markdown renderer — use dynamic content width
	cw := MaxContentWidth(80) - 6 // border + padding
	if cw < 40 {
		cw = 40
	}
	md, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(cw),
	)

	return ChatApp{
		width:    80,
		height:   24,
		input:    ti,
		focused:  true,
		chatView: vp,
		model:    modelName,
		permMode: "default",
		md:       md,
	}
}

func (m ChatApp) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		tickSpinner(),
	)
}

func tickSpinner() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return spinTickMsg(t)
	})
}

// ─── Update ────────────────────────────────────────────────────────────

func (m ChatApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Recreate glamour renderer with new width so markdown wraps correctly
		newCW := MaxContentWidth(m.width) - 6
		if newCW < 40 {
			newCW = 40
		}
		if newMD, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(newCW),
		); err == nil {
			m.md = newMD
		}
		m = m.recalcLayout()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinTickMsg:
		if m.thinking {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(SpinnerFrameSet)
		}
		return m, tickSpinner()

	case AppendChatMsg:
		m.messages = append(m.messages, msg.Msg)
		m = m.rebuildChatContent()
		return m, nil

	case ThinkingMsg:
		m.thinking = msg.Active
		if !msg.Active {
			m.spinnerFrame = 0
		}
		// Recalc layout: thinking line is 0 or 1 row, affects viewport height.
		m = m.recalcLayout()
		return m, nil

	case ContextUpdateMsg:
		m.contextPct = msg.Pct
		return m, nil

	case PlanUpdateMsg:
		m.planSteps = msg.Steps
		if len(msg.Steps) > 0 {
			m.planExpanded = true
		}
		// Check if all steps are completed — auto-collapse when done
		allDone := len(msg.Steps) > 0
		for _, s := range msg.Steps {
			if s.Status != "completed" {
				allDone = false
				break
			}
		}
		if allDone {
			m.planExpanded = false
		}
		m = m.recalcLayout()
		return m, nil

	case UsageMsg:
		usage := ChatMsg{
			Role:    "system",
			Content: FmtUsage(msg.Prompt, msg.Completion, msg.Total),
			Time:    time.Now(),
		}
		m.messages = append(m.messages, usage)
		m = m.rebuildChatContent()
		return m, nil

	case ResponseDoneMsg:
		m.thinking = false
		return m, nil

	case ConfirmRequestMsg:
		m.confirmActive = true
		m.confirmIdx = ConfirmOptYes // start on "Yes"
		m.confirmName = msg.ToolName
		m.confirmPreview = msg.Preview
		m.confirmCb = msg.Respond
		return m, nil
	}

	// Delegate to textarea
	if m.focused && !m.confirmActive {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m ChatApp) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Confirm mode key handling — arrow navigation + Enter to select
	if m.confirmActive {
		switch key {
		case "up", "k":
			if m.confirmIdx > 0 {
				m.confirmIdx--
			}
		case "down", "j":
			if m.confirmIdx < confirmOptCount-1 {
				m.confirmIdx++
			}
		case "enter":
			m.confirmActive = false
			if m.confirmCb != nil {
				switch m.confirmIdx {
				case ConfirmOptYes:
					m.confirmCb(ConfirmYes)
				case ConfirmOptAlways:
					m.confirmCb(ConfirmAlwaysSession)
				case ConfirmOptNo:
					m.confirmCb(ConfirmNo)
				}
			}
		// Quick-select shortcut keys
		case "y":
			m.confirmActive = false
			if m.confirmCb != nil {
				m.confirmCb(ConfirmYes)
			}
		case "a":
			m.confirmActive = false
			if m.confirmCb != nil {
				m.confirmCb(ConfirmAlwaysSession)
			}
		case "n", "esc":
			m.confirmActive = false
			if m.confirmCb != nil {
				m.confirmCb(ConfirmNo)
			}
		}
		return m, nil
	}

	switch key {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		m.input.Reset()
		m.input.SetHeight(1)
		// Add user message
		m.messages = append(m.messages, ChatMsg{
			Role:    "user",
			Content: text,
			Time:    time.Now(),
		})
		m = m.rebuildChatContent()
		// Send to prompt channel (for agent loop consumer)
		if m.promptCh != nil {
			m.promptCh <- text
		}
		// Notify callback (if set)
		if m.OnSend != nil {
			go m.OnSend(text)
		}
		return m, nil

	case "pgup", "shift+pgup":
		m.chatView.HalfViewUp()
		return m, nil

	case "pgdown", "shift+pgdown":
		m.chatView.HalfViewDown()
		return m, nil

	case "tab":
		// Toggle tool/step detail blocks expanded/collapsed
		m.toolsExpanded = !m.toolsExpanded
		m = m.rebuildChatContent()
		// Force full screen repaint — the content change is too large
		// for the diff renderer, which leaves ghost footer lines.
		return m, tea.ClearScreen

	case "ctrl+p":
		// Toggle plan panel expanded/collapsed
		if len(m.planSteps) > 0 {
			m.planExpanded = !m.planExpanded
			m = m.recalcLayout()
			return m, tea.ClearScreen
		}
		return m, nil

	default:
		// Grow textarea up to 10 lines
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		lines := strings.Count(m.input.Value(), "\n") + 1
		if lines > 10 {
			lines = 10
		}
		if lines < 1 {
			lines = 1
		}
		m.input.SetHeight(lines)
		return m, cmd
	}
}

// ─── View ──────────────────────────────────────────────────────────────

func (m ChatApp) View() string {
	if m.quitting {
		return MutedText.Render("  " + BrandEmoji + " Goodbye!") + "\n"
	}

	contentW := MaxContentWidth(m.width)

	// Build sections top-to-bottom
	var sections []string

	// 1. Chat viewport
	sections = append(sections, m.chatView.View())

	// 2. Plan panel (collapsible, above thinking indicator)
	if planPanel := m.renderPlanPanel(contentW); planPanel != "" {
		sections = append(sections, planPanel)
	}

	// 3. Thinking indicator (1 line, above input)
	thinkLine := m.renderThinking(contentW)
	sections = append(sections, thinkLine)

	// 3. Confirm prompt (if active, replaces input)
	if m.confirmActive {
		sections = append(sections, m.renderConfirmPrompt(contentW))
	} else {
		// 4. Input box
		sections = append(sections, m.renderInput(contentW))
	}

	// 5. Footer bar
	sections = append(sections, m.renderFooter())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// ─── Layout recalculation ──────────────────────────────────────────────

func (m ChatApp) recalcLayout() ChatApp {
	contentW := MaxContentWidth(m.width)

	// Input area height: 1-10 lines + 2 border
	inputLines := strings.Count(m.input.Value(), "\n") + 1
	if inputLines > 10 {
		inputLines = 10
	}
	inputH := inputLines + 2

	// Footer = 1, thinking indicator = 0 or 1, plan panel = variable
	planH := m.planPanelHeight()
	thinkH := 0
	if m.thinking {
		thinkH = 1
	}
	chatH := m.height - inputH - 1 - thinkH - planH
	if chatH < 3 {
		chatH = 3
	}

	m.chatView.Width = contentW
	m.chatView.Height = chatH

	m.input.SetWidth(contentW - 4) // account for border + padding

	m = m.rebuildChatContent()
	return m
}

// ─── Chat content rendering ────────────────────────────────────────────

func (m ChatApp) rebuildChatContent() ChatApp {
	contentW := MaxContentWidth(m.width) - 4 // border + padding
	if contentW < 30 {
		contentW = 30
	}

	var blocks []string

	for _, msg := range m.messages {
		block := m.renderMessage(msg, contentW)
		blocks = append(blocks, block)
	}

	content := strings.Join(blocks, "\n")
	// Propagate ANSI state so every line is self-contained.
	// Without this, scrolling up causes text to disappear because
	// the viewport slices lines and upper lines' ANSI codes are lost.
	content = PropagateANSI(content)
	m.chatView.SetContent(content)
	m.chatView.GotoBottom()
	return m
}

func (m ChatApp) renderMessage(msg ChatMsg, w int) string {
	switch msg.Role {
	case "user":
		ts := MutedText.Render(msg.Time.Format("15:04"))
		label := PrimaryText.Render("❯ You") + " " + ts
		body := NormalText.Width(w).Render(msg.Content)
		return "\n" + UserBlockStyle.Width(w + 2).Render(label+"\n"+body) + "\n"

	case "assistant":
		body := msg.Content
		if m.md != nil {
			if rendered, err := m.md.Render(msg.Content); err == nil {
				body = strings.TrimRight(rendered, "\n")
			}
		}
		body = Linkify(body)
		return AssistantBlockStyle.Width(w + 2).Render(body)

	case "summary":
		body := msg.Content
		if m.md != nil {
			if rendered, err := m.md.Render(msg.Content); err == nil {
				body = strings.TrimRight(rendered, "\n")
			}
		}
		return "\n" + SummaryBlockStyle.Width(w + 2).Render(body) + "\n"

	case "tool":
		return m.renderToolMessage(msg, w)

	case "error":
		if !m.toolsExpanded {
			// Collapsed: one-line error summary
			errSnippet := TruncStr(msg.Content, 60)
			return ErrorBlockStyle.Width(w + 2).Render(
				ErrorText.Render("✘ ") + MutedText.Render(errSnippet))
		}
		return ErrorBlockStyle.Width(w + 2).Render(
			ErrorText.Render("Error: " + msg.Content))

	case "system":
		if !m.toolsExpanded {
			// Collapsed: hide step indicators and usage lines
			return ""
		}
		return SystemBlockStyle.Width(w + 2).Render(msg.Content)

	case "system-warn":
		return SystemWarnBlockStyle.Width(w + 2).Render(msg.Content)

	case "confirm":
		return m.renderConfirmBlock(msg, w)

	default:
		return NormalText.Render(msg.Content)
	}
}

// renderToolMessage renders a tool call or tool result, collapsed or expanded.
func (m ChatApp) renderToolMessage(msg ChatMsg, w int) string {
	// Tool call (has ToolName, no Content) — the invocation header
	if msg.ToolName != "" && msg.Content == "" {
		header := FormatToolHdr(msg.ToolName, msg.ToolArgs)
		if !m.toolsExpanded {
			// Collapsed: single line with arrow indicator
			return ToolBlockStyle.Width(w + 2).Render(
				MutedText.Render(header))
		}
		// Expanded: full header + exec preview
		var inner strings.Builder
		inner.WriteString(ToolHdrText.Render(Linkify(header)))
		if msg.ToolName == "exec" {
			if cmd, ok := msg.ToolArgs["command"].(string); ok {
				inner.WriteString("\n")
				inner.WriteString(MutedText.Render("$ " + cmd))
			}
		}
		return ToolBlockStyle.Width(w + 2).Render(inner.String())
	}

	// Tool result (has Content) — the output
	if msg.Content != "" {
		resultTag := FmtResultSummary(msg.Content, msg.IsError)
		if !m.toolsExpanded {
			// Collapsed: tiny one-line result badge
			icon := SecondaryText.Render("✓")
			if msg.IsError {
				icon = ErrorText.Render("✘")
			}
			return ToolBlockStyle.Width(w + 2).Render(
				icon + " " + MutedText.Render(resultTag))
		}
		// Expanded: full output (same as before)
		text := TruncateOutput(msg.Content, 15)
		var inner strings.Builder
		inner.WriteString(PanelText.Render("───") + "\n")
		inner.WriteString(NormalText.Width(w).Render(Linkify(text)))
		if resultTag != "" {
			inner.WriteString("\n" + MutedText.Render(resultTag))
		}
		if msg.IsError {
			return ErrorBlockStyle.Width(w + 2).Render(inner.String())
		}
		return ToolBlockStyle.Width(w + 2).Render(inner.String())
	}

	return ""
}

func (m ChatApp) renderConfirmBlock(msg ChatMsg, w int) string {
	return "\n" + RenderConfirmBox(msg.ToolName, msg.Content, w, ConfirmOptYes)
}

// ─── Thinking indicator ────────────────────────────────────────────────

func (m ChatApp) renderThinking(w int) string {
	if !m.thinking {
		// Return empty string — no blank line reservation.
		// Reserving a full line of spaces caused layout jank
		// (ghost lines, double thinking indicators).
		return ""
	}
	frame := SpinnerFrameSet[m.spinnerFrame%len(SpinnerFrameSet)]
	return MutedText.Render(fmt.Sprintf("  %s thinking…", frame))
}

// ─── Plan panel ────────────────────────────────────────────────────────

// planPanelHeight returns the height consumed by the plan panel (0 if hidden).
func (m ChatApp) planPanelHeight() int {
	if len(m.planSteps) == 0 {
		return 0
	}
	if !m.planExpanded {
		return 1 // collapsed: just the header line
	}
	// Header + each step + blank line below
	return 1 + len(m.planSteps) + 1
}

// renderPlanPanel renders the collapsible Copilot-style plan tracker.
func (m ChatApp) renderPlanPanel(w int) string {
	if len(m.planSteps) == 0 {
		return ""
	}

	done := 0
	for _, s := range m.planSteps {
		if s.Status == "completed" {
			done++
		}
	}
	total := len(m.planSteps)

	if !m.planExpanded {
		// Collapsed: single line summary
		chevron := "▸"
		header := fmt.Sprintf(" %s Plan (%d/%d)", chevron, done, total)
		return MutedText.Render(header)
	}

	// Expanded: header + steps
	chevron := "▾"
	header := SecondaryText.Render(fmt.Sprintf(" %s Plan (%d/%d)", chevron, done, total))

	var lines []string
	lines = append(lines, header)
	for _, s := range m.planSteps {
		icon := "○"
		style := MutedText
		switch s.Status {
		case "in-progress":
			icon = "◉"
			style = PrimaryText
		case "completed":
			icon = "✓"
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#66aa66"))
		}
		lines = append(lines, style.Render(fmt.Sprintf("   %s %s", icon, s.Title)))
	}

	return PlanPanelStyle.Width(w).Render(strings.Join(lines, "\n"))
}

// ─── Input box ─────────────────────────────────────────────────────────

func (m ChatApp) renderInput(contentW int) string {
	style := InputBoxFocusedStyle.Width(contentW - 2)
	if !m.focused {
		style = InputBoxStyle.Width(contentW - 2)
	}
	return style.Render(m.input.View())
}

// ─── Confirm prompt (overlay) ──────────────────────────────────────────

func (m ChatApp) renderConfirmPrompt(contentW int) string {
	return RenderConfirmBox(m.confirmName, m.confirmPreview, contentW, m.confirmIdx)
}

// ─── Footer ────────────────────────────────────────────────────────────
// claudechic StatusFooter: model · permission-mode · (spacer) · context-bar · branch

func (m ChatApp) renderFooter() string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	brand := PrimaryText.Render(BrandEmoji)
	modelLabel := MutedText.Render(m.model)
	sep := PanelText.Render("·")

	permLabel := "Auto-edit: off"
	permStyle := MutedText
	switch m.permMode {
	case "acceptEdits":
		permLabel = "Auto-edit: on"
		permStyle = lipgloss.NewStyle().Foreground(ColorPrimary)
	case "plan":
		permLabel = "Plan mode"
		permStyle = lipgloss.NewStyle().Foreground(ColorSecondary)
	}
	permRendered := permStyle.Render(permLabel)

	detailLabel := "Details: off"
	detailStyle := MutedText
	if m.toolsExpanded {
		detailLabel = "Details: on"
		detailStyle = lipgloss.NewStyle().Foreground(ColorSecondary)
	}
	detailRendered := detailStyle.Render(detailLabel)

	// Plan indicator
	planRendered := ""
	if len(m.planSteps) > 0 {
		done := 0
		for _, s := range m.planSteps {
			if s.Status == "completed" {
				done++
			}
		}
		planLabel := fmt.Sprintf("Plan: %d/%d", done, len(m.planSteps))
		planStyle := lipgloss.NewStyle().Foreground(ColorSecondary)
		if done == len(m.planSteps) {
			planStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#66aa66"))
		}
		planRendered = sep + " " + planStyle.Render(planLabel) + " "
	}

	contextBar := RenderCtxBar(m.contextPct)

	left := fmt.Sprintf(" %s %s %s %s %s %s %s %s", brand, sep, modelLabel, sep, permRendered, sep, detailRendered, planRendered)
	right := fmt.Sprintf("%s ", contextBar)

	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	return FooterStyle.Width(w).MaxHeight(1).Render(
		left + strings.Repeat(" ", gap) + right,
	)
}

// ─── Context bar ───────────────────────────────────────────────────────
// Mirrors claudechic ContextBar indicator



// ─── Formatting helpers ────────────────────────────────────────────────
// Ported from claudechic/formatting.py



// ─── Public API for external callers ───────────────────────────────────

// RunChatApp starts the full-screen Bubble Tea chat TUI.
// Returns the tea.Program for sending messages, and a channel
// that emits user-submitted prompts.
func RunChatApp(modelName string) (*tea.Program, <-chan string) {
	promptCh := make(chan string, 10)
	app := NewChatApp(modelName)
	app.promptCh = promptCh
	// Mouse tracking is disabled — SGR escape sequences leak into the textarea
	// as garbled text on fast scrolling. Use PgUp/PgDn for chat viewport scroll.
	p := tea.NewProgram(app, tea.WithAltScreen())
	return p, promptCh
}
