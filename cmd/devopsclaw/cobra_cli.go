// DevOpsClaw — Production-grade DevOps automation platform
// CLI-first, fleet-scale, browser-capable
// License: MIT
//
// Copyright (c) 2026 DevOpsClaw contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/freitascorp/devopsclaw/pkg/audit"
	"github.com/freitascorp/devopsclaw/pkg/config"
	"github.com/freitascorp/devopsclaw/pkg/deploy"
	"github.com/freitascorp/devopsclaw/pkg/fleet"
	"github.com/freitascorp/devopsclaw/pkg/logger"
	"github.com/freitascorp/devopsclaw/pkg/relay"
	"github.com/freitascorp/devopsclaw/pkg/runbook"
	"github.com/freitascorp/devopsclaw/pkg/skills"
	"github.com/freitascorp/devopsclaw/pkg/tui"
)

// ------------------------------------------------------------------
// Global flags
// ------------------------------------------------------------------

var (
	flagDebug  bool
	flagJSON   bool
)

func getConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".devopsclaw")
}

func newAuditStore() *audit.FileStore {
	return audit.NewFileStore(filepath.Join(getConfigDir(), "audit"))
}

func newRunbookEngine() *runbook.Engine {
	return runbook.NewEngine(filepath.Join(getConfigDir(), "runbooks"))
}

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// newFleetStack creates the full fleet management stack (store, relay, node manager, executor).
func newFleetStack(cfg *config.Config, slogger *slog.Logger) (fleet.Store, *fleet.NodeManager, *fleet.Executor, *relay.WSServer) {
	store := fleet.NewMemoryStore()
	nodeMgr := fleet.NewNodeManager(store, slogger)

	relayConfig := relay.ServerConfig{
		ListenAddr:   cfg.Relay.ListenAddr,
		AuthToken:    cfg.Relay.AuthToken,
		MaxNodes:     cfg.Relay.MaxNodes,
		PingInterval: 15 * time.Second,
	}
	if relayConfig.ListenAddr == "" {
		relayConfig.ListenAddr = ":9443"
	}
	if relayConfig.MaxNodes <= 0 {
		relayConfig.MaxNodes = 1000
	}

	wsServer := relay.NewWSServer(relayConfig, store, slogger)
	relayClient := relay.NewWSRelayClient(wsServer, slogger)
	executor := fleet.NewExecutor(store, relayClient, slogger)

	return store, nodeMgr, executor, wsServer
}

// ------------------------------------------------------------------
// Root command
// ------------------------------------------------------------------

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "devopsclaw",
		Short: "DevOpsClaw — Production-grade DevOps automation platform",
		Long: `DevOpsClaw is a CLI-first, fleet-scale, browser-capable DevOps automation platform.

It provides multi-machine orchestration, NAT-safe relay connectivity,
deployment strategies, browser automation, runbooks, and a full audit trail.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if flagDebug {
				logger.SetLevel(logger.DEBUG)
			}
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().BoolVarP(&flagDebug, "debug", "d", false, "Enable debug logging")
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output in JSON format")

	// Register all command groups
	root.AddCommand(
		// Existing commands (preserved)
		newOnboardCmd(),
		newAgentCobraCmd(),
		newGatewayCobraCmd(),
		newStatusCobraCmd(),
		newMigrateCobraCmd(),
		newAuthCobraCmd(),
		newCronCobraCmd(),
		newSkillsCobraCmd(),
		newVersionCmd(),

		// New fleet/DevOps commands
		newRunCmd(),
		newFleetCmd(),
		newDeployCmd(),
		newBrowseCmd(),
		newNodeCmd(),
		newRunbookCmd(),
		newAuditCmd(),
		newRelayCmd(),
		newAgentDaemonCmd(),
		newMCPCobraCmd(),
	)

	return root
}

// ------------------------------------------------------------------
// Wrapper commands for existing functionality
// ------------------------------------------------------------------

func newOnboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "onboard",
		Short: "Initialize DevOpsClaw configuration and workspace",
		Run: func(cmd *cobra.Command, args []string) {
			onboard()
		},
	}
}

func newAgentCobraCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agent",
		Short: "Interact with the AI agent directly",
		Long:  "Start an interactive agent session or send a one-shot message.",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			agentCmd()
		},
	}
}

func newGatewayCobraCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gateway",
		Short: "Start the DevOpsClaw gateway (channels, health, cron)",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			gatewayCmd()
		},
	}
}

func newStatusCobraCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show DevOpsClaw system status",
		Run: func(cmd *cobra.Command, args []string) {
			statusCmd()
		},
	}
}

func newMigrateCobraCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Migrate from OpenClaw to DevOpsClaw",
		Run: func(cmd *cobra.Command, args []string) {
			migrateCmd()
		},
	}
}

func newAuthCobraCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication (login, logout, status)",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			authCmd()
		},
	}
}

func newCronCobraCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cron",
		Short: "Manage scheduled tasks",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			cronCmd()
		},
	}
}

func newSkillsCobraCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skills",
		Short: "Manage skills (install, list, remove, search)",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			// Re-dispatch to existing skills handler
			skillsCobraDispatch()
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			printVersion()
		},
	}
}

// ------------------------------------------------------------------
// `devopsclaw run` — Execute command on one or more nodes
// ------------------------------------------------------------------

func newRunCmd() *cobra.Command {
	var (
		flagNode   string
		flagTag    string
		flagEnv    string
		flagDryRun bool
	)

	cmd := &cobra.Command{
		Use:   "run [command]",
		Short: "Execute a command on one or more fleet nodes",
		Long: `Execute a shell command on targeted fleet nodes.

Examples:
  devopsclaw run "df -h" --node prod-web-1
  devopsclaw run "uptime" --tag role=web --env prod
  devopsclaw run "nginx -t" --tag role=web --dry-run`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			slogger := newLogger()
			store, _, executor, _ := newFleetStack(cfg, slogger)

			// Build target
			target := buildTarget(flagNode, flagTag, flagEnv)

			// Build request
			cmdData, _ := json.Marshal(fleet.ShellCommand{Command: strings.Join(args, " ")})
			req := &fleet.ExecRequest{
				ID:        fmt.Sprintf("run_%d", time.Now().UnixNano()),
				Target:    target,
				Command:   fleet.TypedCommand{Type: "shell", Data: cmdData},
				Timeout:   30 * time.Second,
				DryRun:    flagDryRun,
				Requester: "cli",
				CreatedAt: time.Now(),
			}

			result, err := executor.Execute(context.Background(), req)
			if err != nil {
				// If no fleet nodes, fallback to listing from store
				nodes, _ := store.ListNodes(context.Background())
				if len(nodes) == 0 {
					return fmt.Errorf("no fleet nodes registered. Use 'devopsclaw node register' to add nodes or start a relay with 'devopsclaw relay start'")
				}
				return err
			}

			// Audit
			auditStore := newAuditStore()
			status := "success"
			if result.Summary.Failed > 0 {
				status = "partial"
			}
			audit.NewLogger(auditStore, "cli").LogFleetExec(context.Background(),
				strings.Join(args, " "),
				&audit.EventTarget{Command: strings.Join(args, " ")},
				&audit.EventResult{
					Status:       status,
					NodesTotal:   result.Summary.Total,
					NodesSuccess: result.Summary.Success,
					NodesFailed:  result.Summary.Failed,
					Duration:     result.Duration,
				},
			)

			return printExecResult(result)
		},
	}

	cmd.Flags().StringVar(&flagNode, "node", "", "Target node(s), comma-separated")
	cmd.Flags().StringVar(&flagTag, "tag", "", "Target by label tags (e.g., role=web,env=prod)")
	cmd.Flags().StringVar(&flagEnv, "env", "", "Target by environment shorthand")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Preview without executing")

	return cmd
}

// ------------------------------------------------------------------
// `devopsclaw fleet` — Fleet management commands
// ------------------------------------------------------------------

func newFleetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Fleet management and orchestration",
	}

	cmd.AddCommand(
		newFleetExecCmd(),
		newFleetStatusCmd(),
	)

	return cmd
}

func newFleetExecCmd() *cobra.Command {
	var (
		flagNode       string
		flagTag        string
		flagEnv        string
		flagExclude    string
		flagSerial     bool
		flagParallel   bool
		flagMaxConc    int
		flagDelay      time.Duration
		flagDryRun     bool
		flagTimeout    time.Duration
	)

	cmd := &cobra.Command{
		Use:   "exec [command]",
		Short: "Fan-out execution across fleet nodes",
		Long: `Execute a command across multiple fleet nodes with targeting and concurrency control.

Examples:
  devopsclaw fleet exec "uptime" --tag role=web
  devopsclaw fleet exec "docker ps" --tag role=api,env=prod
  devopsclaw fleet exec "systemctl restart nginx" --env staging
  devopsclaw fleet exec "apt upgrade -y" --serial --delay 5s
  devopsclaw fleet exec "docker pull myapp:latest" --parallel --max 10`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			slogger := newLogger()
			_, _, executor, _ := newFleetStack(cfg, slogger)

			target := buildTarget(flagNode, flagTag, flagEnv)
			if flagMaxConc > 0 {
				target.MaxConcurrency = flagMaxConc
			}
			if flagSerial {
				target.MaxConcurrency = 1
			}

			if flagTimeout <= 0 {
				flagTimeout = 30 * time.Second
			}

			cmdData, _ := json.Marshal(fleet.ShellCommand{Command: strings.Join(args, " ")})
			req := &fleet.ExecRequest{
				ID:        fmt.Sprintf("fleet_%d", time.Now().UnixNano()),
				Target:    target,
				Command:   fleet.TypedCommand{Type: "shell", Data: cmdData},
				Timeout:   flagTimeout,
				DryRun:    flagDryRun,
				Requester: "cli",
				CreatedAt: time.Now(),
			}

			result, err := executor.Execute(context.Background(), req)
			if err != nil {
				return err
			}

			return printExecResult(result)
		},
	}

	cmd.Flags().StringVar(&flagNode, "node", "", "Target node(s), comma-separated")
	cmd.Flags().StringVar(&flagTag, "tag", "", "Target by label tags")
	cmd.Flags().StringVar(&flagEnv, "env", "", "Target by environment")
	cmd.Flags().StringVar(&flagExclude, "exclude", "", "Exclude nodes matching these tags")
	cmd.Flags().BoolVar(&flagSerial, "serial", false, "Execute one at a time")
	cmd.Flags().BoolVar(&flagParallel, "parallel", false, "Execute in parallel (default)")
	cmd.Flags().IntVar(&flagMaxConc, "max", 0, "Max concurrent executions")
	cmd.Flags().DurationVar(&flagDelay, "delay", 0, "Delay between serial executions")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Preview without executing")
	cmd.Flags().DurationVar(&flagTimeout, "timeout", 30*time.Second, "Execution timeout")

	return cmd
}

func newFleetStatusCmd() *cobra.Command {
	var flagLive bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show fleet status dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			slogger := newLogger()
			store, nodeMgr, _, _ := newFleetStack(cfg, slogger)

			// Live TUI mode
			if flagLive {
				return tui.RunFleetDashboard(store, nodeMgr)
			}

			summary, err := nodeMgr.Summary(context.Background())
			if err != nil {
				return err
			}

			if flagJSON {
				data, _ := json.MarshalIndent(summary, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			// Text output
			nodes, _ := store.ListNodes(context.Background())
			printFleetStatus(summary, nodes)
			return nil
		},
	}

	cmd.Flags().BoolVar(&flagLive, "live", false, "Live TUI dashboard with auto-refresh")

	return cmd
}

// ------------------------------------------------------------------
// `devopsclaw deploy` — Deployment management
// ------------------------------------------------------------------

func newDeployCmd() *cobra.Command {
	var (
		flagStrategy       string
		flagTag            string
		flagEnv            string
		flagNode           string
		flagHealthURL      string
		flagRollbackOnFail bool
		flagMaxUnavailable int
		flagDryRun         bool
		flagRollbackCmd    string
	)

	cmd := &cobra.Command{
		Use:   "deploy [service:version] [deploy-command]",
		Short: "Deploy services with strategies and automatic rollback",
		Long: `Deploy a service across fleet nodes with rolling, canary, blue-green, or serial strategies.

Examples:
  devopsclaw deploy myapp:v2.1.3 "docker pull && docker restart" --strategy rolling --env prod
  devopsclaw deploy myapp:v2.1.3 "./deploy.sh" --strategy canary --rollback-on-fail
  devopsclaw deploy myapp:v2.1.3 "helm upgrade" --strategy blue-green --health-check /health`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			slogger := newLogger()
			store, _, executor, _ := newFleetStack(cfg, slogger)

			// Parse service:version
			parts := strings.SplitN(args[0], ":", 2)
			service := parts[0]
			version := "latest"
			if len(parts) > 1 {
				version = parts[1]
			}

			deployCommand := strings.Join(args[1:], " ")
			target := buildTarget(flagNode, flagTag, flagEnv)

			spec := deploy.Spec{
				Service:        service,
				Version:        version,
				Strategy:       deploy.Strategy(flagStrategy),
				Target:         target,
				HealthCheckURL: flagHealthURL,
				RollbackOnFail: flagRollbackOnFail,
				MaxUnavailable: flagMaxUnavailable,
				DeployCommand:  deployCommand,
				RollbackCommand: flagRollbackCmd,
				Requester:      "cli",
			}

			deployer := deploy.NewDeployer(executor, store, slogger)
			result, err := deployer.Deploy(context.Background(), spec)

			if flagJSON {
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
				if err != nil {
					return err
				}
				return nil
			}

			if result != nil {
				fmt.Printf("Deploy %s:%s — %s (%s)\n", service, version, result.State, result.Duration.Round(time.Millisecond))
				fmt.Printf("  Strategy:  %s\n", flagStrategy)
				fmt.Printf("  Batches:   %d\n", len(result.Batches))
				if result.RolledBack {
					fmt.Println("  ⚠ ROLLED BACK")
				}
			}

			return err
		},
	}

	cmd.Flags().StringVar(&flagStrategy, "strategy", "rolling", "Deployment strategy: rolling, canary, blue-green, all-at-once, serial")
	cmd.Flags().StringVar(&flagTag, "tag", "", "Target by label tags")
	cmd.Flags().StringVar(&flagEnv, "env", "", "Target by environment")
	cmd.Flags().StringVar(&flagNode, "node", "", "Target specific nodes")
	cmd.Flags().StringVar(&flagHealthURL, "health-check", "", "Health check URL (e.g., /health)")
	cmd.Flags().BoolVar(&flagRollbackOnFail, "rollback-on-fail", false, "Automatically rollback on failure")
	cmd.Flags().IntVar(&flagMaxUnavailable, "max-unavailable", 1, "Max nodes unavailable during rolling deploy")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Preview without executing")
	cmd.Flags().StringVar(&flagRollbackCmd, "rollback-cmd", "", "Command to run for rollback")

	return cmd
}

// ------------------------------------------------------------------
// `devopsclaw browse` — Browser automation
// ------------------------------------------------------------------

func newBrowseCmd() *cobra.Command {
	var (
		flagURL     string
		flagTask    string
		flagSession string
	)

	cmd := &cobra.Command{
		Use:   "browse",
		Short: "AI-powered browser automation for web UIs",
		Long: `Automate web browser interactions — log in, click buttons, read dashboards, take screenshots.

Examples:
  devopsclaw browse --url https://console.aws.amazon.com --task "check RDS storage"
  devopsclaw browse --url https://app.datadoghq.com --task "get P95 latency for last 1h"
  devopsclaw browse --session datadog-prod --task "get alert count"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagURL == "" && flagSession == "" {
				return fmt.Errorf("either --url or --session is required")
			}
			if flagTask == "" {
				return fmt.Errorf("--task is required")
			}

			fmt.Printf("🌐 Browse: %s\n", flagURL)
			fmt.Printf("  Task: %s\n", flagTask)
			if flagSession != "" {
				fmt.Printf("  Session: %s\n", flagSession)
			}

			// The browse command dispatches to the agent with browser tool
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			prompt := fmt.Sprintf("Use the browser tool to go to %s and %s", flagURL, flagTask)
			if flagSession != "" {
				prompt = fmt.Sprintf("Use browser session %s to %s", flagSession, flagTask)
			}

			return runAgentOnce(cfg, prompt)
		},
	}

	cmd.Flags().StringVar(&flagURL, "url", "", "URL to navigate to")
	cmd.Flags().StringVar(&flagTask, "task", "", "Natural language task to perform")
	cmd.Flags().StringVar(&flagSession, "session", "", "Saved browser session name")

	return cmd
}

// ------------------------------------------------------------------
// `devopsclaw node` — Node management
// ------------------------------------------------------------------

func newNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Register, tag, and manage fleet nodes",
	}

	cmd.AddCommand(
		newNodeRegisterCmd(),
		newNodeListCmd(),
		newNodeRemoveCmd(),
		newNodeDrainCmd(),
	)

	return cmd
}

func newNodeRegisterCmd() *cobra.Command {
	var (
		flagName    string
		flagAddress string
		flagTags    string
		flagGroups  string
	)

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a new node in the fleet",
		Long: `Register a node with the fleet. Provide --address for direct SSH/IP connectivity,
or use the relay agent (devopsclaw agent-daemon) for NAT-safe auto-registration.

Examples:
  devopsclaw node register --name prod-web-1 --address 10.0.1.50 --tags env=prod,role=web,region=eu-west-1
  devopsclaw node register --name prod-web-2 --address prod-web-2.internal:22 --tags env=prod,role=web
  devopsclaw node register --name staging-api --address 192.168.1.100 --tags env=staging,role=api --groups staging`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			slogger := newLogger()
			_, nodeMgr, _, _ := newFleetStack(cfg, slogger)

			node := &fleet.Node{
				ID:           fleet.NodeID(flagName),
				Hostname:     flagName,
				Address:      flagAddress,
				Status:       fleet.NodeStatusOnline,
				Labels:       parseTags(flagTags),
				RegisteredAt: time.Now(),
				LastSeen:     time.Now(),
			}

			if flagGroups != "" {
				for _, g := range strings.Split(flagGroups, ",") {
					node.Groups = append(node.Groups, fleet.GroupName(strings.TrimSpace(g)))
				}
			}

			if err := nodeMgr.Register(context.Background(), node); err != nil {
				return err
			}

			if flagAddress != "" {
				fmt.Printf("✓ Node %s registered (%s)\n", flagName, flagAddress)
			} else {
				fmt.Printf("✓ Node %s registered (no address — will connect via relay)\n", flagName)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&flagName, "name", "", "Node name/ID (required)")
	cmd.Flags().StringVar(&flagAddress, "address", "", "Node IP or hostname (e.g., 10.0.1.50, web1.internal:22)")
	cmd.Flags().StringVar(&flagTags, "tags", "", "Labels in key=value,key=value format")
	cmd.Flags().StringVar(&flagGroups, "groups", "", "Groups, comma-separated")
	cmd.MarkFlagRequired("name")

	return cmd
}

func newNodeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered fleet nodes",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			slogger := newLogger()
			store, _, _, _ := newFleetStack(cfg, slogger)

			nodes, err := store.ListNodes(context.Background())
			if err != nil {
				return err
			}

			if flagJSON {
				data, _ := json.MarshalIndent(nodes, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(nodes) == 0 {
				fmt.Println("No nodes registered. Use 'devopsclaw node register' to add nodes.")
				return nil
			}

			fmt.Printf("%-20s %-22s %-12s %-24s %s\n", "NODE", "ADDRESS", "STATUS", "LABELS", "LAST SEEN")
			fmt.Println(strings.Repeat("─", 100))
			for _, n := range nodes {
				labels := formatLabels(n.Labels)
				lastSeen := "never"
				if !n.LastSeen.IsZero() {
					lastSeen = time.Since(n.LastSeen).Round(time.Second).String() + " ago"
				}
				status := statusIcon(n.Status) + " " + string(n.Status)
				addr := n.Address
				if addr == "" {
					if n.TunnelID != "" {
						addr = "(relay)"
					} else {
						addr = "—"
					}
				}
				fmt.Printf("%-20s %-22s %-12s %-24s %s\n", n.ID, addr, status, labels, lastSeen)
			}
			return nil
		},
	}
}

func newNodeRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [node-id]",
		Short: "Remove a node from the fleet",
		Aliases: []string{"rm"},
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			slogger := newLogger()
			_, nodeMgr, _, _ := newFleetStack(cfg, slogger)

			if err := nodeMgr.Deregister(context.Background(), fleet.NodeID(args[0])); err != nil {
				return err
			}
			fmt.Printf("✓ Node %s removed\n", args[0])
			return nil
		},
	}
}

func newNodeDrainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drain [node-id]",
		Short: "Drain a node (stop receiving new commands)",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			slogger := newLogger()
			_, nodeMgr, _, _ := newFleetStack(cfg, slogger)

			if err := nodeMgr.Drain(context.Background(), fleet.NodeID(args[0])); err != nil {
				return err
			}
			fmt.Printf("✓ Node %s draining\n", args[0])
			return nil
		},
	}
}

// ------------------------------------------------------------------
// `devopsclaw runbook` — Runbook management
// ------------------------------------------------------------------

func newRunbookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runbook",
		Short: "Define, store, and run reusable workflows",
	}

	cmd.AddCommand(
		newRunbookRunCmd(),
		newRunbookListCmd(),
		newRunbookShowCmd(),
	)

	return cmd
}

func newRunbookRunCmd() *cobra.Command {
	var flagDryRun bool

	cmd := &cobra.Command{
		Use:   "run [name]",
		Short: "Execute a runbook",
		Long: `Execute a YAML-defined runbook workflow.

Examples:
  devopsclaw runbook run incident-db-high-connections
  devopsclaw runbook run incident-db-high-connections --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engine := newRunbookEngine()
			rb, err := engine.Get(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("📋 Running runbook: %s\n", rb.Name)
			if rb.Description != "" {
				fmt.Printf("   %s\n", rb.Description)
			}
			if flagDryRun {
				fmt.Println("   Mode: DRY RUN")
			}
			fmt.Println()

			result, err := engine.Run(context.Background(), rb, flagDryRun)

			// Audit
			auditStore := newAuditStore()
			status := "success"
			if err != nil {
				status = "failure"
			}
			audit.NewLogger(auditStore, "cli").LogRunbook(context.Background(), args[0], flagDryRun, &audit.EventResult{
				Status:   status,
				Duration: result.Duration,
			})

			if flagJSON {
				out, _ := runbook.FormatResultJSON(result)
				fmt.Println(out)
			} else {
				fmt.Print(runbook.FormatResult(result))
			}

			return err
		},
	}

	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Preview actions without executing")

	return cmd
}

func newRunbookListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available runbooks",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			engine := newRunbookEngine()
			runbooks, err := engine.List()
			if err != nil {
				return err
			}

			if len(runbooks) == 0 {
				fmt.Printf("No runbooks found in %s\n", filepath.Join(getConfigDir(), "runbooks"))
				fmt.Println("Create YAML runbook files there to get started.")
				return nil
			}

			if flagJSON {
				data, _ := json.MarshalIndent(runbooks, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("%-30s %-40s %s\n", "NAME", "DESCRIPTION", "STEPS")
			fmt.Println(strings.Repeat("─", 80))
			for _, rb := range runbooks {
				desc := rb.Description
				if len(desc) > 38 {
					desc = desc[:38] + "…"
				}
				fmt.Printf("%-30s %-40s %d\n", rb.Name, desc, len(rb.Steps))
			}
			return nil
		},
	}
}

func newRunbookShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Show runbook details",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engine := newRunbookEngine()
			rb, err := engine.Get(args[0])
			if err != nil {
				return err
			}

			if flagJSON {
				data, _ := json.MarshalIndent(rb, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Name:        %s\n", rb.Name)
			fmt.Printf("Description: %s\n", rb.Description)
			if len(rb.Tags) > 0 {
				fmt.Printf("Tags:        %s\n", strings.Join(rb.Tags, ", "))
			}
			fmt.Printf("Steps:       %d\n\n", len(rb.Steps))
			for i, step := range rb.Steps {
				fmt.Printf("  %d. %s\n", i+1, step.Name)
				if step.Run != "" {
					fmt.Printf("     run: %s\n", step.Run)
				}
				if step.Browse != nil {
					fmt.Printf("     browse: %s\n", step.Browse.Task)
				}
				if step.RequiresApproval {
					fmt.Println("     ⏸ requires approval")
				}
			}
			return nil
		},
	}
}

// ------------------------------------------------------------------
// `devopsclaw audit` — Audit log queries
// ------------------------------------------------------------------

func newAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Query the immutable audit log",
	}

	cmd.AddCommand(
		newAuditListCmd(),
		newAuditExportCmd(),
	)

	return cmd
}

func newAuditListCmd() *cobra.Command {
	var (
		flagUser  string
		flagSince string
		flagLimit int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List audit events",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			store := newAuditStore()

			opts := audit.QueryOptions{
				User:  flagUser,
				Limit: flagLimit,
			}
			if flagSince != "" {
				dur, err := time.ParseDuration(flagSince)
				if err != nil {
					return fmt.Errorf("invalid --since duration: %w", err)
				}
				opts.Since = time.Now().Add(-dur)
			}

			events, err := store.Query(context.Background(), opts)
			if err != nil {
				return err
			}

			if flagJSON {
				data, _ := json.MarshalIndent(events, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(events) == 0 {
				fmt.Println("No audit events found.")
				return nil
			}

			fmt.Printf("%-24s %-15s %-15s %s\n", "TIMESTAMP", "USER", "TYPE", "ACTION")
			fmt.Println(strings.Repeat("─", 80))
			for _, e := range events {
				fmt.Printf("%-24s %-15s %-15s %s\n",
					e.Timestamp.Format("2006-01-02 15:04:05"),
					e.User,
					e.Type,
					e.Action,
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&flagUser, "user", "", "Filter by user")
	cmd.Flags().StringVar(&flagSince, "since", "", "Filter since duration (e.g., 2h, 24h)")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Max events to show")

	return cmd
}

func newAuditExportCmd() *cobra.Command {
	var flagSince string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export audit events as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := newAuditStore()

			since := time.Now().Add(-24 * time.Hour) // default 24h
			if flagSince != "" {
				dur, err := time.ParseDuration(flagSince)
				if err != nil {
					return fmt.Errorf("invalid --since: %w", err)
				}
				since = time.Now().Add(-dur)
			}

			events, err := store.Export(context.Background(), since)
			if err != nil {
				return err
			}

			data, _ := json.MarshalIndent(events, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&flagSince, "since", "24h", "Export since duration")

	return cmd
}

// ------------------------------------------------------------------
// `devopsclaw relay` — Relay server management
// ------------------------------------------------------------------

func newRelayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relay",
		Short: "Manage the relay server for NAT-safe fleet connectivity",
	}

	cmd.AddCommand(newRelayStartCmd())
	return cmd
}

func newRelayStartCmd() *cobra.Command {
	var (
		flagAddr  string
		flagToken string
		flagMax   int
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the relay server",
		Long: `Start the WebSocket relay server that brokers connections between the CLI and fleet nodes.

Nodes connect outbound to this server — no inbound ports required on nodes.

Examples:
  devopsclaw relay start
  devopsclaw relay start --addr :9443 --token my-secret-token
  devopsclaw relay start --max 500`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			if flagAddr != "" {
				cfg.Relay.ListenAddr = flagAddr
			}
			if flagToken != "" {
				cfg.Relay.AuthToken = flagToken
			}
			if flagMax > 0 {
				cfg.Relay.MaxNodes = flagMax
			}

			slogger := newLogger()
			_, _, _, wsServer := newFleetStack(cfg, slogger)

			fmt.Printf("🔗 Relay server starting on %s\n", cfg.Relay.ListenAddr)
			if cfg.Relay.AuthToken != "" {
				fmt.Println("  Auth: token-based")
			}
			fmt.Printf("  Max nodes: %d\n", cfg.Relay.MaxNodes)
			fmt.Println("  Press Ctrl+C to stop")

			ctx := context.Background()
			return wsServer.Start(ctx)
		},
	}

	cmd.Flags().StringVar(&flagAddr, "addr", ":9443", "Listen address")
	cmd.Flags().StringVar(&flagToken, "token", "", "Auth token for node registration")
	cmd.Flags().IntVar(&flagMax, "max", 1000, "Maximum connected nodes")

	return cmd
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

func buildTarget(node, tag, env string) fleet.TargetSelector {
	target := fleet.TargetSelector{}

	if node != "" {
		for _, n := range strings.Split(node, ",") {
			target.NodeIDs = append(target.NodeIDs, fleet.NodeID(strings.TrimSpace(n)))
		}
	}

	if tag != "" {
		target.Labels = parseTags(tag)
	}

	if env != "" {
		if target.Labels == nil {
			target.Labels = make(map[string]string)
		}
		target.Labels["env"] = env
	}

	if len(target.NodeIDs) == 0 && len(target.Labels) == 0 && len(target.Groups) == 0 {
		target.All = true
	}

	return target
}

func parseTags(s string) map[string]string {
	labels := make(map[string]string)
	if s == "" {
		return labels
	}
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			labels[parts[0]] = parts[1]
		}
	}
	return labels
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "-"
	}
	var parts []string
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func statusIcon(status fleet.NodeStatus) string {
	switch status {
	case fleet.NodeStatusOnline:
		return "●"
	case fleet.NodeStatusOffline:
		return "○"
	case fleet.NodeStatusDegraded:
		return "⚠"
	case fleet.NodeStatusDraining:
		return "◐"
	case fleet.NodeStatusUnreachable:
		return "✗"
	default:
		return "?"
	}
}

func printExecResult(result *fleet.ExecResult) error {
	if flagJSON {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Fleet Execution — %d nodes, %s\n", result.Summary.Total, result.Duration.Round(time.Millisecond))
	fmt.Printf("  ✓ %d success  ✗ %d failed  ⏱ %d timeout  ○ %d skipped\n\n",
		result.Summary.Success, result.Summary.Failed, result.Summary.Timeout, result.Summary.Skipped)

	for _, nr := range result.NodeResults {
		icon := "✓"
		if nr.Status == "failure" {
			icon = "✗"
		} else if nr.Status == "timeout" {
			icon = "⏱"
		} else if nr.Status == "skipped" {
			icon = "○"
		}

		fmt.Printf("  %s %s (%s)\n", icon, nr.NodeID, nr.Duration.Round(time.Millisecond))
		if nr.Output != "" {
			for _, line := range strings.Split(strings.TrimSpace(nr.Output), "\n") {
				fmt.Printf("    %s\n", line)
			}
		}
		if nr.Error != "" {
			fmt.Printf("    Error: %s\n", nr.Error)
		}
	}

	if result.Summary.Failed > 0 {
		return fmt.Errorf("%d node(s) failed", result.Summary.Failed)
	}
	return nil
}

func printFleetStatus(summary *fleet.FleetSummary, nodes []*fleet.Node) {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  FLEET STATUS  ·  %d nodes  ·  %s   ║\n",
		summary.TotalNodes,
		time.Now().Format("2006-01-02 15:04"),
	)
	fmt.Println("╠════════════════╦═════════════╦══════════════════════════════╣")
	fmt.Printf("║ %-14s ║ %-11s ║ %-28s ║\n", "NODE", "STATUS", "LABELS")
	fmt.Println("╠════════════════╬═════════════╬══════════════════════════════╣")

	for _, n := range nodes {
		status := statusIcon(n.Status) + " " + string(n.Status)
		labels := formatLabels(n.Labels)
		if len(labels) > 28 {
			labels = labels[:27] + "…"
		}
		fmt.Printf("║ %-14s ║ %-11s ║ %-28s ║\n", n.ID, status, labels)
	}

	fmt.Println("╚════════════════╩═════════════╩══════════════════════════════╝")
	fmt.Printf("  %d online · %d offline · %d degraded · %d unreachable\n",
		summary.Online, summary.Offline, summary.Degraded, summary.Unreachable)
}

// runAgentOnce creates an agent and processes a single message.
func runAgentOnce(cfg *config.Config, message string) error {
	// Delegate to the agent infrastructure
	// Set up os.Args temporarily for the legacy agent command
	originalArgs := os.Args
	os.Args = []string{"devopsclaw", "agent", "-m", message}
	defer func() { os.Args = originalArgs }()
	agentCmd()
	return nil
}

// ------------------------------------------------------------------
// `devopsclaw mcp` — Model Context Protocol stdio server
// ------------------------------------------------------------------

func newMCPCobraCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP stdio server (for Gemini CLI, Claude Desktop, etc.)",
		Long: `Start a Model Context Protocol (MCP) server over stdio.

This exposes devopsclaw's tools (exec, read_file, write_file, edit_file,
list_directory, web_search, web_fetch, etc.) as MCP tools that external
AI clients can call.

Gemini CLI configuration (~/.gemini/settings.json):
  {
    "mcpServers": {
      "devopsclaw": {
        "command": "devopsclaw",
        "args": ["mcp"]
      }
    }
  }

Claude Desktop configuration:
  {
    "mcpServers": {
      "devopsclaw": {
        "command": "devopsclaw",
        "args": ["mcp"]
      }
    }
  }`,
		Run: func(cmd *cobra.Command, args []string) {
			mcpCmd()
		},
	}
}

// ------------------------------------------------------------------
// `devopsclaw agent-daemon` — Lightweight node agent connecting to relay
// ------------------------------------------------------------------

func newAgentDaemonCmd() *cobra.Command {
	var (
		flagRelayAddr string
		flagNodeID    string
		flagToken     string
	)

	cmd := &cobra.Command{
		Use:   "agent-daemon",
		Short: "Run as a fleet node agent, connecting outbound to a relay",
		Long: `Start the DevOpsClaw node agent that connects to a relay server.

The agent establishes an outbound WebSocket connection to the relay.
It then listens for commands from the control plane and executes them locally.
No inbound ports required on the node.

Examples:
  devopsclaw agent-daemon --relay ws://relay.company.com:9443 --node-id prod-web-1
  devopsclaw agent-daemon --relay wss://relay:9443 --node-id staging-api --token my-secret
  RELAY_ADDR=ws://relay:9443 NODE_ID=worker-1 devopsclaw agent-daemon`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			// Override from flags, then config, then env
			if flagRelayAddr == "" {
				flagRelayAddr = cfg.Relay.RelayAddr
			}
			if flagRelayAddr == "" {
				flagRelayAddr = os.Getenv("RELAY_ADDR")
			}
			if flagRelayAddr == "" {
				return fmt.Errorf("--relay is required (or set RELAY_ADDR)")
			}

			if flagNodeID == "" {
				flagNodeID = cfg.Relay.NodeID
			}
			if flagNodeID == "" {
				flagNodeID = os.Getenv("NODE_ID")
			}
			if flagNodeID == "" {
				hostname, _ := os.Hostname()
				flagNodeID = hostname
			}

			if flagToken == "" {
				flagToken = cfg.Relay.AuthToken
			}
			if flagToken == "" {
				flagToken = os.Getenv("RELAY_TOKEN")
			}

			slogger := newLogger()
			agentCfg := relay.AgentConfig{
				RelayAddr:         flagRelayAddr,
				NodeID:            fleet.NodeID(flagNodeID),
				AuthToken:         flagToken,
				ReconnectInterval: 5 * time.Second,
				HeartbeatInterval: 30 * time.Second,
			}

			executor := relay.NewShellExecutor("")
			wsAgent := relay.NewWSAgent(agentCfg, executor, slogger)

			fmt.Printf("🔗 Agent daemon starting\n")
			fmt.Printf("  Node ID:  %s\n", flagNodeID)
			fmt.Printf("  Relay:    %s\n", flagRelayAddr)
			fmt.Println("  Press Ctrl+C to stop")

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle interrupt
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, os.Interrupt)
				<-sigCh
				fmt.Println("\nStopping agent daemon...")
				wsAgent.Stop()
				cancel()
			}()

			return wsAgent.Run(ctx)
		},
	}

	cmd.Flags().StringVar(&flagRelayAddr, "relay", "", "Relay server address (ws:// or wss://)")
	cmd.Flags().StringVar(&flagNodeID, "node-id", "", "Node identifier (default: hostname)")
	cmd.Flags().StringVar(&flagToken, "token", "", "Auth token for relay")

	return cmd
}

// skillsCobraDispatch re-dispatches skills commands to the existing handler.
func skillsCobraDispatch() {
	if len(os.Args) < 3 {
		skillsHelp()
		return
	}

	subcommand := os.Args[2]
	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	workspace := cfg.WorkspacePath()
	installer := skills.NewSkillInstaller(workspace)
	globalDir := filepath.Dir(getConfigPath())
	globalSkillsDir := filepath.Join(globalDir, "skills")
	builtinSkillsDir := filepath.Join(globalDir, "devopsclaw", "skills")
	skillsLoader := skills.NewSkillsLoader(workspace, globalSkillsDir, builtinSkillsDir)

	switch subcommand {
	case "list":
		skillsListCmd(skillsLoader)
	case "install":
		skillsInstallCmd(installer, cfg)
	case "remove", "uninstall":
		if len(os.Args) < 4 {
			fmt.Println("Usage: devopsclaw skills remove <skill-name>")
			return
		}
		skillsRemoveCmd(installer, os.Args[3])
	case "install-builtin":
		skillsInstallBuiltinCmd(workspace)
	case "list-builtin":
		skillsListBuiltinCmd()
	case "search":
		skillsSearchCmd(installer)
	case "show":
		if len(os.Args) < 4 {
			fmt.Println("Usage: devopsclaw skills show <skill-name>")
			return
		}
		skillsShowCmd(skillsLoader, os.Args[3])
	default:
		fmt.Printf("Unknown skills command: %s\n", subcommand)
		skillsHelp()
	}
}
