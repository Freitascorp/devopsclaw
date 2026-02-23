package rbac

import (
	"context"
	"testing"
)

func TestToolGuard_DisabledAllowsAll(t *testing.T) {
	guard := NewToolGuard(nil, false)

	err := guard.CheckToolAccess(context.Background(), "alice", "execute_command")
	if err != nil {
		t.Errorf("disabled guard should allow all: %v", err)
	}
}

func TestToolGuard_EnabledDeniesUnknownUser(t *testing.T) {
	enforcer := NewEnforcer(nil)
	guard := NewToolGuard(enforcer, true)

	err := guard.CheckToolAccess(context.Background(), "nobody", "execute_command")
	if err == nil {
		t.Error("enabled guard should deny unknown user for execute_command")
	}
}

func TestToolGuard_EnabledAllowsAdmin(t *testing.T) {
	enforcer := NewEnforcer(nil)
	enforcer.RegisterUser(&User{
		ID:    "admin-user",
		Roles: []RoleName{RoleAdmin.Name},
	})
	guard := NewToolGuard(enforcer, true)

	tests := []string{
		"execute_command", "shell", "read_file", "write_file",
		"create_file", "edit_file", "delete_file", "list_directory",
		"search_files", "docker", "kubernetes", "browser",
		"fleet_exec", "fleet_deploy", "spawn", "cron_add", "cron_list",
	}

	for _, tool := range tests {
		err := guard.CheckToolAccess(context.Background(), "admin-user", tool)
		if err != nil {
			t.Errorf("admin should have access to %q: %v", tool, err)
		}
	}
}

func TestToolGuard_ViewerDeniedWriteTools(t *testing.T) {
	enforcer := NewEnforcer(nil)
	enforcer.RegisterUser(&User{
		ID:    "viewer",
		Roles: []RoleName{RoleViewer.Name},
	})
	guard := NewToolGuard(enforcer, true)

	// Viewer should be denied write operations
	deniedTools := []string{
		"execute_command", "shell", "write_file", "create_file",
		"edit_file", "delete_file", "docker", "kubernetes",
		"fleet_exec", "fleet_deploy", "spawn",
	}

	for _, tool := range deniedTools {
		err := guard.CheckToolAccess(context.Background(), "viewer", tool)
		if err == nil {
			t.Errorf("viewer should NOT have access to %q", tool)
		}
	}
}

func TestToolGuard_ViewerAllowedReadTools(t *testing.T) {
	enforcer := NewEnforcer(nil)
	enforcer.RegisterUser(&User{
		ID:    "viewer",
		Roles: []RoleName{RoleViewer.Name},
	})
	guard := NewToolGuard(enforcer, true)

	// Viewer should be allowed read operations
	allowedTools := []string{"read_file", "list_directory", "search_files", "cron_list"}

	for _, tool := range allowedTools {
		err := guard.CheckToolAccess(context.Background(), "viewer", tool)
		if err != nil {
			t.Errorf("viewer should have access to %q: %v", tool, err)
		}
	}
}

func TestToolGuard_UnknownToolDefaultsToAllow(t *testing.T) {
	enforcer := NewEnforcer(nil)
	// Don't add user — for unknown tools, the check returns nil without checking permissions
	guard := NewToolGuard(enforcer, true)

	// Unknown tools (like web_search, message) should default to allow
	err := guard.CheckToolAccess(context.Background(), "anyone", "web_search")
	if err != nil {
		t.Errorf("unknown tool should default to allow: %v", err)
	}
}

func TestToolGuard_CheckFleetAccess_Disabled(t *testing.T) {
	guard := NewToolGuard(nil, false)
	err := guard.CheckFleetAccess(context.Background(), "anyone", PermFleetExec, "prod")
	if err != nil {
		t.Errorf("disabled guard should allow fleet access: %v", err)
	}
}

func TestToolGuard_CheckFleetAccess_Admin(t *testing.T) {
	enforcer := NewEnforcer(nil)
	enforcer.RegisterUser(&User{
		ID:    "admin",
		Roles: []RoleName{RoleAdmin.Name},
	})
	guard := NewToolGuard(enforcer, true)

	err := guard.CheckFleetAccess(context.Background(), "admin", PermFleetExec, "production")
	if err != nil {
		t.Errorf("admin should have fleet access: %v", err)
	}
}

func TestToolGuard_ResolveUser_Disabled(t *testing.T) {
	guard := NewToolGuard(nil, false)
	id := guard.ResolveUser("slack", "U123")
	if id != "U123" {
		t.Errorf("disabled guard should return senderID as-is: got %q", id)
	}
}

func TestToolGuard_ResolveUser_NotFound(t *testing.T) {
	enforcer := NewEnforcer(nil)
	guard := NewToolGuard(enforcer, true)
	id := guard.ResolveUser("slack", "U_UNKNOWN")
	// When user not found, should return senderID
	if id != "U_UNKNOWN" {
		t.Errorf("expected fallback to senderID, got %q", id)
	}
}

func TestToolPermissionMap_Coverage(t *testing.T) {
	// Ensure all mapped tools have valid permissions
	for tool, perm := range ToolPermissionMap {
		if tool == "" {
			t.Error("empty tool name in ToolPermissionMap")
		}
		if perm == "" {
			t.Errorf("empty permission for tool %q", tool)
		}
	}
}

func TestInferPermissionFromCommand(t *testing.T) {
	tests := []struct {
		command string
		want    Permission
	}{
		{"sudo apt upgrade", PermShellExecSudo},
		{"su - root", PermShellExecSudo},
		{"docker ps", PermDockerView},
		{"docker logs myapp", PermDockerView},
		{"docker inspect myapp", PermDockerView},
		{"docker run myapp", PermDockerManage},
		{"docker stop myapp", PermDockerManage},
		{"kubectl get pods", PermK8sView},
		{"kubectl describe node", PermK8sView},
		{"kubectl logs myapp", PermK8sView},
		{"kubectl apply -f deploy.yaml", PermK8sManage},
		{"helm install myapp", PermK8sManage},
		{"helm upgrade myapp", PermK8sManage},
		{"ls -la", PermShellExec},
		{"echo hello", PermShellExec},
	}

	for _, tt := range tests {
		got := InferPermissionFromCommand(tt.command)
		if got != tt.want {
			t.Errorf("InferPermissionFromCommand(%q) = %q, want %q", tt.command, got, tt.want)
		}
	}
}
