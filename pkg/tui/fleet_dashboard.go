// Package tui provides terminal UI components for DevOpsClaw using Bubble Tea.
// Currently includes a fleet status dashboard with live node status.
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
// Styles
// ------------------------------------------------------------------

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6B6B")).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7B68EE")).
			PaddingLeft(1).
			PaddingRight(1)

	onlineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF88"))

	offlineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4444"))

	degradedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFB347"))

	drainingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#87CEEB"))

	unreachableStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#999999"))

	cellStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingRight(1)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			MarginTop(1)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#555555")).
			Padding(0, 1)

	summaryOnline = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00FF88"))

	summaryOffline = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF4444"))
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
	b.WriteString(titleStyle.Render("🦞 DevOpsClaw Fleet Dashboard"))
	b.WriteString("\n")

	// Summary bar
	if m.summary != nil {
		summaryLine := fmt.Sprintf(
			"%s  %s  %s  %s  %s",
			summaryOnline.Render(fmt.Sprintf("● %d online", m.summary.Online)),
			summaryOffline.Render(fmt.Sprintf("○ %d offline", m.summary.Offline)),
			degradedStyle.Render(fmt.Sprintf("⚠ %d degraded", m.summary.Degraded)),
			drainingStyle.Render(fmt.Sprintf("◐ %d draining", m.summary.Draining)),
			unreachableStyle.Render(fmt.Sprintf("✗ %d unreachable", m.summary.Unreachable)),
		)
		b.WriteString(boxStyle.Render(fmt.Sprintf("Total: %d nodes  │  %s",
			m.summary.TotalNodes, summaryLine)))
		b.WriteString("\n\n")
	}

	// Node table
	if len(m.nodes) == 0 {
		b.WriteString(footerStyle.Render("  No nodes registered. Use 'devopsclaw node register' to add nodes."))
		b.WriteString("\n")
	} else {
		// Header
		header := fmt.Sprintf("%-20s %-14s %-30s %s",
			headerStyle.Render("NODE"),
			headerStyle.Render("STATUS"),
			headerStyle.Render("LABELS"),
			headerStyle.Render("LAST SEEN"),
		)
		b.WriteString(header)
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", clampInt(m.width, 85)))
		b.WriteString("\n")

		// Rows
		for _, n := range m.nodes {
			statusStr := renderStatus(n.Status)
			labels := formatLabelsShort(n.Labels, 28)
			lastSeen := renderLastSeen(n.LastSeen)

			row := fmt.Sprintf("%-20s %-14s %-30s %s",
				cellStyle.Render(string(n.ID)),
				statusStr,
				cellStyle.Render(labels),
				cellStyle.Render(lastSeen),
			)
			b.WriteString(row)
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(footerStyle.Render(fmt.Sprintf("  [r] refresh  [q] quit  │  Updated: %s",
		time.Now().Format("15:04:05"))))

	return b.String()
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

func renderStatus(status fleet.NodeStatus) string {
	switch status {
	case fleet.NodeStatusOnline:
		return onlineStyle.Render("● online")
	case fleet.NodeStatusOffline:
		return offlineStyle.Render("○ offline")
	case fleet.NodeStatusDegraded:
		return degradedStyle.Render("⚠ degraded")
	case fleet.NodeStatusDraining:
		return drainingStyle.Render("◐ draining")
	case fleet.NodeStatusUnreachable:
		return unreachableStyle.Render("✗ unreach.")
	default:
		return cellStyle.Render("? " + string(status))
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
