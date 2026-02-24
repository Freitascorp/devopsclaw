// Package rbac provides role-based access control for DevOpsClaw Pro.
//
// It enforces who can do what across the fleet, with per-user, per-role,
// and per-resource permission boundaries. Every action is auditable.
//
// Design principles:
//   - Deny by default: no permission = denied
//   - Least privilege: grant only what's needed
//   - Audit everything: every decision is logged
//   - Multi-tenant safe: users see only their permitted scope
package rbac

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ------------------------------------------------------------------
// Core types
// ------------------------------------------------------------------

// UserID identifies a user across channels.
type UserID string

// RoleName is a named permission set.
type RoleName string

// Permission is a specific action that can be allowed or denied.
type Permission string

// Pre-defined permissions following resource:action pattern.
const (
	// Fleet operations
	PermFleetView      Permission = "fleet:view"
	PermFleetExec      Permission = "fleet:exec"
	PermFleetExecAll   Permission = "fleet:exec:all" // execute on all nodes
	PermFleetDeploy    Permission = "fleet:deploy"
	PermFleetManage    Permission = "fleet:manage" // register/deregister nodes

	// Shell operations
	PermShellExec      Permission = "shell:exec"
	PermShellExecSudo  Permission = "shell:exec:sudo"

	// File operations
	PermFileRead       Permission = "file:read"
	PermFileWrite      Permission = "file:write"
	PermFileDelete     Permission = "file:delete"

	// Docker operations
	PermDockerView     Permission = "docker:view"
	PermDockerManage   Permission = "docker:manage"

	// Kubernetes operations
	PermK8sView        Permission = "k8s:view"
	PermK8sManage      Permission = "k8s:manage"

	// Browser automation
	PermBrowserExec    Permission = "browser:exec"

	// Agent management
	PermAgentView      Permission = "agent:view"
	PermAgentManage    Permission = "agent:manage"
	PermAgentSwitch    Permission = "agent:switch"

	// Config operations
	PermConfigView     Permission = "config:view"
	PermConfigEdit     Permission = "config:edit"

	// Cron operations
	PermCronView       Permission = "cron:view"
	PermCronManage     Permission = "cron:manage"

	// Audit
	PermAuditView      Permission = "audit:view"

	// Admin
	PermAdmin          Permission = "admin:*"
)

// Pre-defined roles.
var (
	RoleAdmin = Role{
		Name:        "admin",
		Description: "Full access to all operations",
		Permissions: []Permission{PermAdmin},
	}
	RoleOperator = Role{
		Name:        "operator",
		Description: "Can execute commands and deploy across the fleet",
		Permissions: []Permission{
			PermFleetView, PermFleetExec, PermFleetDeploy,
			PermShellExec, PermFileRead, PermFileWrite,
			PermDockerView, PermDockerManage,
			PermK8sView, PermK8sManage,
			PermBrowserExec,
			PermAgentView, PermAgentSwitch, PermAgentManage,
			PermCronView, PermCronManage,
			PermAuditView,
		},
	}
	RoleViewer = Role{
		Name:        "viewer",
		Description: "Read-only access to fleet status and logs",
		Permissions: []Permission{
			PermFleetView, PermFileRead,
			PermDockerView, PermK8sView,
			PermAgentView, PermCronView,
			PermAuditView,
		},
	}
	RoleAgent = Role{
		Name:        "agent",
		Description: "Permissions for AI agent tool execution",
		Permissions: []Permission{
			PermFleetView, PermFleetExec,
			PermShellExec, PermFileRead, PermFileWrite,
			PermDockerView,
			PermAgentView,
		},
	}
)

// Role is a named collection of permissions.
type Role struct {
	Name        RoleName     `json:"name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions"`
}

// User represents an authenticated identity with role bindings.
type User struct {
	ID          UserID              `json:"id"`
	DisplayName string              `json:"display_name"`
	Roles       []RoleName          `json:"roles"`
	ChannelIDs  map[string]string   `json:"channel_ids"` // channel → platform user ID
	Scopes      []ResourceScope     `json:"scopes,omitempty"` // additional restrictions
	CreatedAt   time.Time           `json:"created_at"`
	LastSeen    time.Time           `json:"last_seen"`
	Disabled    bool                `json:"disabled"`
}

// ResourceScope limits a user's permissions to specific resources.
type ResourceScope struct {
	NodeGroups []string `json:"node_groups,omitempty"` // limit to specific groups
	NodeIDs    []string `json:"node_ids,omitempty"`    // limit to specific nodes
	WorkDirs   []string `json:"work_dirs,omitempty"`   // limit to specific directories
}

// ------------------------------------------------------------------
// Enforcer
// ------------------------------------------------------------------

// Enforcer evaluates access control decisions.
type Enforcer struct {
	mu    sync.RWMutex
	roles map[RoleName]*Role
	users map[UserID]*User
	audit AuditLogger
}

// AuditLogger records access control decisions.
type AuditLogger interface {
	LogDecision(entry AuditEntry)
}

// AuditEntry records a single access control decision.
type AuditEntry struct {
	Timestamp  time.Time  `json:"timestamp"`
	UserID     UserID     `json:"user_id"`
	Permission Permission `json:"permission"`
	Resource   string     `json:"resource"`
	Decision   string     `json:"decision"` // "allow", "deny"
	Reason     string     `json:"reason"`
	Channel    string     `json:"channel"`
	SessionKey string     `json:"session_key"`
	IP         string     `json:"ip,omitempty"`
}

// NewEnforcer creates an RBAC enforcer with default roles.
func NewEnforcer(audit AuditLogger) *Enforcer {
	e := &Enforcer{
		roles: make(map[RoleName]*Role),
		users: make(map[UserID]*User),
		audit: audit,
	}
	// Register default roles
	for _, r := range []Role{RoleAdmin, RoleOperator, RoleViewer, RoleAgent} {
		e.roles[r.Name] = &r
	}
	return e
}

// Check evaluates whether a user has a specific permission.
func (e *Enforcer) Check(ctx context.Context, userID UserID, perm Permission, resource string) bool {
	e.mu.RLock()
	user, ok := e.users[userID]
	e.mu.RUnlock()

	if !ok || user.Disabled {
		e.logDeny(userID, perm, resource, "user not found or disabled")
		return false
	}

	// Check each role
	for _, roleName := range user.Roles {
		e.mu.RLock()
		role, exists := e.roles[roleName]
		e.mu.RUnlock()
		if !exists {
			continue
		}
		for _, p := range role.Permissions {
			if matchPermission(p, perm) {
				e.logAllow(userID, perm, resource)
				return true
			}
		}
	}

	e.logDeny(userID, perm, resource, "no matching permission")
	return false
}

// CheckWithScope evaluates permission + scope restrictions.
func (e *Enforcer) CheckWithScope(ctx context.Context, userID UserID, perm Permission, resource string, nodeGroup string) bool {
	if !e.Check(ctx, userID, perm, resource) {
		return false
	}

	e.mu.RLock()
	user := e.users[userID]
	e.mu.RUnlock()

	// If no scopes defined, allow (permission already checked)
	if len(user.Scopes) == 0 {
		return true
	}

	// Check scope restrictions
	for _, scope := range user.Scopes {
		if len(scope.NodeGroups) > 0 && nodeGroup != "" {
			allowed := false
			for _, g := range scope.NodeGroups {
				if g == nodeGroup {
					allowed = true
					break
				}
			}
			if !allowed {
				e.logDeny(userID, perm, resource, "node group not in scope: "+nodeGroup)
				return false
			}
		}
	}
	return true
}

// RegisterUser adds a user.
func (e *Enforcer) RegisterUser(user *User) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	e.users[user.ID] = user
}

// RegisterRole adds or updates a role.
func (e *Enforcer) RegisterRole(role *Role) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.roles[role.Name] = role
}

// ResolveUserFromChannel maps a channel + sender ID to a User.
func (e *Enforcer) ResolveUserFromChannel(channel, senderID string) (*User, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, u := range e.users {
		if cid, ok := u.ChannelIDs[channel]; ok && cid == senderID {
			return u, true
		}
	}
	return nil, false
}

// matchPermission checks if a granted permission covers the requested one.
// Supports wildcards: "admin:*" matches everything, "fleet:*" matches "fleet:view".
func matchPermission(granted, requested Permission) bool {
	if granted == requested {
		return true
	}
	// Global admin wildcard — "admin:*" matches any permission
	if granted == PermAdmin {
		return true
	}
	// Scoped wildcard match (e.g., "fleet:*" matches "fleet:exec")
	gParts := strings.Split(string(granted), ":")
	rParts := strings.Split(string(requested), ":")
	for i, gp := range gParts {
		if gp == "*" {
			return true
		}
		if i >= len(rParts) {
			return false
		}
		if gp != rParts[i] {
			return false
		}
	}
	return len(gParts) == len(rParts)
}

func (e *Enforcer) logAllow(userID UserID, perm Permission, resource string) {
	if e.audit != nil {
		e.audit.LogDecision(AuditEntry{
			Timestamp:  time.Now(),
			UserID:     userID,
			Permission: perm,
			Resource:   resource,
			Decision:   "allow",
		})
	}
}

func (e *Enforcer) logDeny(userID UserID, perm Permission, resource, reason string) {
	if e.audit != nil {
		e.audit.LogDecision(AuditEntry{
			Timestamp:  time.Now(),
			UserID:     userID,
			Permission: perm,
			Resource:   resource,
			Decision:   "deny",
			Reason:     reason,
		})
	}
}

// ------------------------------------------------------------------
// Default audit logger (structured log)
// ------------------------------------------------------------------

// StructuredAuditLogger writes audit entries as structured log events.
type StructuredAuditLogger struct {
	mu      sync.Mutex
	entries []AuditEntry
	maxSize int
}

// NewStructuredAuditLogger creates an in-memory audit logger.
// For production, replace with a persistent backend (SQLite, PostgreSQL, etc.)
func NewStructuredAuditLogger(maxSize int) *StructuredAuditLogger {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &StructuredAuditLogger{
		entries: make([]AuditEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

func (l *StructuredAuditLogger) LogDecision(entry AuditEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) >= l.maxSize {
		// Ring buffer: drop oldest 10%
		drop := l.maxSize / 10
		l.entries = l.entries[drop:]
	}
	l.entries = append(l.entries, entry)
}

// Query returns audit entries matching the filter.
func (l *StructuredAuditLogger) Query(opts AuditQueryOptions) []AuditEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []AuditEntry
	for _, e := range l.entries {
		if opts.UserID != "" && e.UserID != opts.UserID {
			continue
		}
		if opts.Decision != "" && e.Decision != opts.Decision {
			continue
		}
		if !opts.Since.IsZero() && e.Timestamp.Before(opts.Since) {
			continue
		}
		if opts.Permission != "" && e.Permission != opts.Permission {
			continue
		}
		out = append(out, e)
		if opts.Limit > 0 && len(out) >= opts.Limit {
			break
		}
	}
	return out
}

// AuditQueryOptions filters audit log queries.
type AuditQueryOptions struct {
	UserID     UserID
	Permission Permission
	Decision   string // "allow" or "deny"
	Since      time.Time
	Limit      int
}

// String returns a human-readable audit entry.
func (e AuditEntry) String() string {
	return fmt.Sprintf("[%s] user=%s perm=%s resource=%s decision=%s reason=%s",
		e.Timestamp.Format(time.RFC3339), e.UserID, e.Permission, e.Resource, e.Decision, e.Reason)
}
