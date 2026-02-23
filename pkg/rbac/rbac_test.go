package rbac

import (
	"context"
	"testing"
)

func TestEnforcer_AdminAccess(t *testing.T) {
	audit := NewStructuredAuditLogger(1000)
	enforcer := NewEnforcer(audit)

	enforcer.RegisterUser(&User{
		ID:    "admin-1",
		Roles: []RoleName{"admin"},
	})

	ctx := context.Background()
	if !enforcer.Check(ctx, "admin-1", PermFleetExec, "any") {
		t.Error("admin should have fleet exec permission")
	}
	if !enforcer.Check(ctx, "admin-1", PermShellExecSudo, "any") {
		t.Error("admin should have sudo permission")
	}
}

func TestEnforcer_ViewerRestrictions(t *testing.T) {
	audit := NewStructuredAuditLogger(1000)
	enforcer := NewEnforcer(audit)

	enforcer.RegisterUser(&User{
		ID:    "viewer-1",
		Roles: []RoleName{"viewer"},
	})

	ctx := context.Background()
	if !enforcer.Check(ctx, "viewer-1", PermFleetView, "any") {
		t.Error("viewer should have fleet view permission")
	}
	if enforcer.Check(ctx, "viewer-1", PermFleetExec, "any") {
		t.Error("viewer should NOT have fleet exec permission")
	}
	if enforcer.Check(ctx, "viewer-1", PermShellExec, "any") {
		t.Error("viewer should NOT have shell exec permission")
	}
}

func TestEnforcer_UnknownUser(t *testing.T) {
	audit := NewStructuredAuditLogger(1000)
	enforcer := NewEnforcer(audit)

	ctx := context.Background()
	if enforcer.Check(ctx, "nobody", PermFleetView, "any") {
		t.Error("unknown user should be denied")
	}
}

func TestEnforcer_DisabledUser(t *testing.T) {
	audit := NewStructuredAuditLogger(1000)
	enforcer := NewEnforcer(audit)

	enforcer.RegisterUser(&User{
		ID:       "disabled-1",
		Roles:    []RoleName{"admin"},
		Disabled: true,
	})

	ctx := context.Background()
	if enforcer.Check(ctx, "disabled-1", PermFleetView, "any") {
		t.Error("disabled user should be denied")
	}
}

func TestEnforcer_ScopeRestriction(t *testing.T) {
	audit := NewStructuredAuditLogger(1000)
	enforcer := NewEnforcer(audit)

	enforcer.RegisterUser(&User{
		ID:    "scoped-1",
		Roles: []RoleName{"operator"},
		Scopes: []ResourceScope{
			{NodeGroups: []string{"staging"}},
		},
	})

	ctx := context.Background()

	// Should allow in-scope group
	if !enforcer.CheckWithScope(ctx, "scoped-1", PermFleetExec, "deploy", "staging") {
		t.Error("should allow scoped group")
	}

	// Should deny out-of-scope group
	if enforcer.CheckWithScope(ctx, "scoped-1", PermFleetExec, "deploy", "production") {
		t.Error("should deny out-of-scope group")
	}
}

func TestEnforcer_ChannelResolution(t *testing.T) {
	audit := NewStructuredAuditLogger(1000)
	enforcer := NewEnforcer(audit)

	enforcer.RegisterUser(&User{
		ID:    "multi-channel",
		Roles: []RoleName{"operator"},
		ChannelIDs: map[string]string{
			"telegram": "12345",
			"discord":  "67890",
		},
	})

	user, ok := enforcer.ResolveUserFromChannel("telegram", "12345")
	if !ok || user.ID != "multi-channel" {
		t.Error("should resolve user from telegram channel")
	}

	user, ok = enforcer.ResolveUserFromChannel("discord", "67890")
	if !ok || user.ID != "multi-channel" {
		t.Error("should resolve user from discord channel")
	}

	_, ok = enforcer.ResolveUserFromChannel("slack", "unknown")
	if ok {
		t.Error("should not resolve unknown channel mapping")
	}
}

func TestMatchPermission(t *testing.T) {
	tests := []struct {
		granted, requested Permission
		expected           bool
	}{
		{PermAdmin, PermFleetExec, true},         // admin:* matches everything
		{PermFleetView, PermFleetView, true},      // exact match
		{PermFleetView, PermFleetExec, false},     // different action
		{PermShellExec, PermShellExecSudo, false}, // no wildcard
		{"fleet:*", PermFleetExec, true},          // resource wildcard
		{"fleet:*", PermShellExec, false},         // different resource
	}

	for _, tt := range tests {
		t.Run(string(tt.granted)+"→"+string(tt.requested), func(t *testing.T) {
			got := matchPermission(tt.granted, tt.requested)
			if got != tt.expected {
				t.Errorf("matchPermission(%s, %s) = %v, want %v", tt.granted, tt.requested, got, tt.expected)
			}
		})
	}
}

func TestAuditLogger_Query(t *testing.T) {
	audit := NewStructuredAuditLogger(1000)
	enforcer := NewEnforcer(audit)

	enforcer.RegisterUser(&User{ID: "user-1", Roles: []RoleName{"viewer"}})

	ctx := context.Background()
	enforcer.Check(ctx, "user-1", PermFleetView, "nodes")  // allow
	enforcer.Check(ctx, "user-1", PermFleetExec, "deploy") // deny

	entries := audit.Query(AuditQueryOptions{UserID: "user-1"})
	if len(entries) != 2 {
		t.Fatalf("expected 2 audit entries, got %d", len(entries))
	}

	allows := audit.Query(AuditQueryOptions{UserID: "user-1", Decision: "allow"})
	if len(allows) != 1 {
		t.Errorf("expected 1 allow entry, got %d", len(allows))
	}

	denies := audit.Query(AuditQueryOptions{UserID: "user-1", Decision: "deny"})
	if len(denies) != 1 {
		t.Errorf("expected 1 deny entry, got %d", len(denies))
	}
}
