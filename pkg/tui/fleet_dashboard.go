// Package tui provides terminal UI components for DevOpsClaw using Bubble Tea.
// Fleet dashboard styled with the claudechic visual language:
//   - Left border bars for content sections
//   - Chic color palette (primary=#cc7700, secondary=#5599dd, panel=#555555)
//   - Minimal chrome, dark background
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/freitascorp/devopsclaw/pkg/fleet"
)

// ------------------------------------------------------------------
// Styles – use shared palette from styles.go
// ------------------------------------------------------------------

var (
	// Title: primary orange
	dTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	// Table header: secondary blue
	dHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSecondary).
			PaddingLeft(1).
			PaddingRight(1)

	// Status colors – green/red kept for semantic clarity
	dOnlineStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary) // secondary blue = healthy

	dOfflineStyle = lipgloss.NewStyle().
			Foreground(ColorError) // red

	dDegradedStyle = lipgloss.NewStyle().
			Foreground(ColorWarn) // yellow

	dDrainingStyle = lipgloss.NewStyle().
			Foreground(ColorAccent) // muted blue-gray

	dUnreachableStyle = lipgloss.NewStyle().
			Foreground(ColorPanel) // gray

	dCellStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingRight(1)

	dFooterStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			MarginTop(1)

	// Summary box uses panel border (like claudechic tool blocks)
	dBoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true).
			BorderTop(false).BorderBottom(false).BorderRight(false).
			BorderForeground(ColorPanel).
			PaddingLeft(1)

	dSummaryOnline = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSecondary) // blue for online

	dSummaryOffline = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorError)
)

// ------------------------------------------------------------------
// Messages
// ------------------------------------------------------------------

type tickMsg time.Time
type nodesMsg []*fleet.Node
type summaryMsg *fleet.FleetSummary

// ------------------------------------------------------------------
// Model
// ------------------------------------------------------------------

// FleetDashboard is the Bubble Tea model for the fleet status TUI.
type FleetDashboard struct {
	store   fleet.Store
	nodeMgr *fleet.NodeManager
	nodes   []*fleet.Node
	summary *fleet.FleetSummary
	err     error
	width   int
	height  int
	quitting bool
}

// NewFleetDashboard creates a new fleet dashboard TUI model.
func NewFleetDashboard(store fleet.Store, nodeMgr *fleet.NodeManager) FleetDashboard {
	return FleetDashboard{
		store:   store,
		nodeMgr: nodeMgr,
		width:   80,
		height:  24,
	}
}

func (m FleetDashboard) Init() tea.Cmd {
	return tea.Batch(
		m.fetchNodes,
		m.fetchSummary,
		tickCmd(),
	)
}

func (m FleetDashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "r":
			// Manual refresh
			return m, tea.Batch(m.fetchNodes, m.fetchSummary)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchNodes, m.fetchSummary, tickCmd())

	case nodesMsg:
		m.nodes = []*fleet.Node(msg)
		return m, nil

	case summaryMsg:
		m.summary = (*fleet.FleetSummary)(msg)
		return m, nil
	}

	return m, nil
}

func (m FleetDashboard) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Title
	b.WriteString(dTitleStyle.Render("🦞 DevOpsClaw Fleet Dashboard"))
	b.WriteString("\n")

	// Summary bar – left-border block (claudechic style)
	if m.summary != nil {
		summaryLine := fmt.Sprintf(
			"%s  %s  %s  %s  %s",
			dSummaryOnline.Render(fmt.Sprintf("● %d online", m.summary.Online)),
			dSummaryOffline.Render(fmt.Sprintf("○ %d offline", m.summary.Offline)),
			dDegradedStyle.Render(fmt.Sprintf("⚠ %d degraded", m.summary.Degraded)),
			dDrainingStyle.Render(fmt.Sprintf("◐ %d draining", m.summary.Draining)),
			dUnreachableStyle.Render(fmt.Sprintf("✗ %d unreachable", m.summary.Unreachable)),
		)
		b.WriteString(dBoxStyle.Render(fmt.Sprintf("Total: %d nodes  │  %s",
			m.summary.TotalNodes, summaryLine)))
		b.WriteString("\n\n")
	}

	// Node table
	if len(m.nodes) == 0 {
		b.WriteString(dFooterStyle.Render("  No nodes registered. Use 'devopsclaw node register' to add nodes."))
		b.WriteString("\n")
	} else {
		// Header
		header := fmt.Sprintf("%-20s %-14s %-30s %s",
			dHeaderStyle.Render("NODE"),
			dHeaderStyle.Render("STATUS"),
			dHeaderStyle.Render("LABELS"),
			dHeaderStyle.Render("LAST SEEN"),
		)
		b.WriteString(header)
		b.WriteString("\n")
		b.WriteString(PanelText.Render(strings.Repeat("─", clampInt(m.width, 85))))
		b.WriteString("\n")

		// Rows
		for _, n := range m.nodes {
			statusStr := renderStatus(n.Status)
			labels := formatLabelsShort(n.Labels, 28)
			lastSeen := renderLastSeen(n.LastSeen)

			row := fmt.Sprintf("%-20s %-14s %-30s %s",
				dCellStyle.Render(string(n.ID)),
				statusStr,
				dCellStyle.Render(labels),
				dCellStyle.Render(lastSeen),
			)
			b.WriteString(row)
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(dFooterStyle.Render(fmt.Sprintf("  [r] refresh  [q] quit  │  Updated: %s",
		time.Now().Format("15:04:05"))))

	return b.String()
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

func renderStatus(status fleet.NodeStatus) string {
	switch status {
	case fleet.NodeStatusOnline:
		return dOnlineStyle.Render("● online")
	case fleet.NodeStatusOffline:
		return dOfflineStyle.Render("○ offline")
	case fleet.NodeStatusDegraded:
		return dDegradedStyle.Render("⚠ degraded")
	case fleet.NodeStatusDraining:
		return dDrainingStyle.Render("◐ draining")
	case fleet.NodeStatusUnreachable:
		return dUnreachableStyle.Render("✗ unreach.")
	default:
		return dCellStyle.Render("? " + string(status))
	}
}

func renderLastSeen(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func formatLabelsShort(labels map[string]string, maxLen int) string {
	if len(labels) == 0 {
		return "-"
	}
	var parts []string
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	s := strings.Join(parts, ",")
	if len(s) > maxLen {
		return s[:maxLen-1] + "…"
	}
	return s
}

func tickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m FleetDashboard) fetchNodes() tea.Msg {
	nodes, err := m.store.ListNodes(context.Background())
	if err != nil {
		return nodesMsg(nil)
	}
	return nodesMsg(nodes)
}

func (m FleetDashboard) fetchSummary() tea.Msg {
	summary, err := m.nodeMgr.Summary(context.Background())
	if err != nil {
		return summaryMsg(nil)
	}
	return summaryMsg(summary)
}

func clampInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RunFleetDashboard starts the Bubble Tea fleet dashboard.
func RunFleetDashboard(store fleet.Store, nodeMgr *fleet.NodeManager) error {
	model := NewFleetDashboard(store, nodeMgr)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
