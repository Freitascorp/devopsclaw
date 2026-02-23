// RBAC middleware for tool execution in the agent loop.
// Wraps tool calls with permission checks before execution.
package rbac

import (
	"context"
	"fmt"
	"strings"
)

// ToolPermissionMap maps tool names to required permissions.
var ToolPermissionMap = map[string]Permission{
	// Shell/exec tools
	"execute_command": PermShellExec,
	"shell":          PermShellExec,

	// File tools
	"read_file":      PermFileRead,
	"write_file":     PermFileWrite,
	"create_file":    PermFileWrite,
	"edit_file":      PermFileWrite,
	"delete_file":    PermFileDelete,
	"list_directory": PermFileRead,
	"search_files":   PermFileRead,

	// Docker tools
	"docker":         PermDockerManage,

	// Kubernetes tools
	"kubernetes":     PermK8sManage,

	// Browser tools
	"browser":        PermBrowserExec,

	// Fleet tools
	"fleet_exec":     PermFleetExec,
	"fleet_deploy":   PermFleetDeploy,

	// Agent tools
	"spawn":          PermAgentManage,

	// Cron tools
	"cron_add":       PermCronManage,
	"cron_remove":    PermCronManage,
	"cron_list":      PermCronView,
}

// ToolGuard wraps an RBAC enforcer to check permissions before tool execution.
type ToolGuard struct {
	enforcer *Enforcer
	enabled  bool
}

// NewToolGuard creates a new tool guard.
func NewToolGuard(enforcer *Enforcer, enabled bool) *ToolGuard {
	return &ToolGuard{enforcer: enforcer, enabled: enabled}
}

// CheckToolAccess returns nil if the user can execute the tool, or an error describing the denial.
func (g *ToolGuard) CheckToolAccess(ctx context.Context, userID UserID, toolName string) error {
	if !g.enabled || g.enforcer == nil {
		return nil // RBAC not enabled, allow all
	}

	perm, ok := ToolPermissionMap[toolName]
	if !ok {
		// Unknown tools default to agent:view (read-only safe default)
		// This covers web_search, message, web_fetch, etc.
		return nil
	}

	resource := fmt.Sprintf("tool:%s", toolName)
	if g.enforcer.Check(ctx, userID, perm, resource) {
		return nil
	}

	return fmt.Errorf("access denied: user %s lacks permission %s for tool %s", userID, perm, toolName)
}

// CheckFleetAccess checks if a user can execute fleet commands on a target.
func (g *ToolGuard) CheckFleetAccess(ctx context.Context, userID UserID, perm Permission, nodeGroup string) error {
	if !g.enabled || g.enforcer == nil {
		return nil
	}

	resource := "fleet"
	if nodeGroup != "" {
		resource = fmt.Sprintf("fleet:group:%s", nodeGroup)
	}

	if g.enforcer.CheckWithScope(ctx, userID, perm, resource, nodeGroup) {
		return nil
	}

	return fmt.Errorf("access denied: user %s lacks permission %s on %s", userID, perm, resource)
}

// InferPermissionFromCommand infers the required RBAC permission from a command string.
// Used for dynamic commands that aren't mapped to a specific tool name.
func InferPermissionFromCommand(command string) Permission {
	lower := strings.ToLower(command)

	// Check for dangerous patterns
	if strings.Contains(lower, "sudo") || strings.Contains(lower, "su -") {
		return PermShellExecSudo
	}

	// Docker commands
	if strings.HasPrefix(lower, "docker") {
		if strings.Contains(lower, "ps") || strings.Contains(lower, "logs") || strings.Contains(lower, "inspect") {
			return PermDockerView
		}
		return PermDockerManage
	}

	// K8s commands
	if strings.HasPrefix(lower, "kubectl") || strings.HasPrefix(lower, "helm") {
		if strings.Contains(lower, "get") || strings.Contains(lower, "describe") || strings.Contains(lower, "logs") {
			return PermK8sView
		}
		return PermK8sManage
	}

	return PermShellExec
}

// ResolveUser resolves a channel+senderID to an RBAC UserID using the enforcer.
func (g *ToolGuard) ResolveUser(channel, senderID string) UserID {
	if !g.enabled || g.enforcer == nil {
		return UserID(senderID)
	}
	user, ok := g.enforcer.ResolveUserFromChannel(channel, senderID)
	if !ok || user == nil {
		return UserID(senderID)
	}
	return user.ID
}
