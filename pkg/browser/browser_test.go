package browser

import (
	"context"
	"testing"
	"time"
)

// ---- Unit tests for Manager and Tool dispatch (no browser needed) ----

func TestManagerConfig_Defaults(t *testing.T) {
	cfg := ManagerConfig{}
	cfg.defaults()

	if cfg.PoolSize != 4 {
		t.Errorf("PoolSize = %d, want 4", cfg.PoolSize)
	}
	if cfg.DefaultTimeout != 30*time.Second {
		t.Errorf("DefaultTimeout = %v, want 30s", cfg.DefaultTimeout)
	}
	if cfg.ViewportWidth != 1280 {
		t.Errorf("ViewportWidth = %d, want 1280", cfg.ViewportWidth)
	}
	if cfg.ViewportHeight != 720 {
		t.Errorf("ViewportHeight = %d, want 720", cfg.ViewportHeight)
	}
}

func TestManagerConfig_CustomValues(t *testing.T) {
	cfg := ManagerConfig{
		PoolSize:       8,
		DefaultTimeout: 60 * time.Second,
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}
	cfg.defaults()

	if cfg.PoolSize != 8 {
		t.Errorf("PoolSize = %d, want 8", cfg.PoolSize)
	}
	if cfg.DefaultTimeout != 60*time.Second {
		t.Errorf("DefaultTimeout = %v, want 60s", cfg.DefaultTimeout)
	}
}

func TestManager_ClosedManagerRejectsNewSessions(t *testing.T) {
	mgr := NewManager(ManagerConfig{Headless: true})
	_ = mgr.Close()

	_, err := mgr.NewSession("test")
	if err == nil {
		t.Fatal("expected error from closed manager")
	}
	if err.Error() != "manager is closed" {
		t.Errorf("error = %q, want %q", err.Error(), "manager is closed")
	}
}

func TestManager_ListSessions_Empty(t *testing.T) {
	mgr := NewManager(ManagerConfig{Headless: true})
	defer mgr.Close()

	sessions := mgr.ListSessions()
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestManager_IsDomainAllowed(t *testing.T) {
	tests := []struct {
		name     string
		allowed  []string
		url      string
		expected bool
	}{
		{
			name:     "no restrictions",
			allowed:  nil,
			url:      "https://anything.com",
			expected: true,
		},
		{
			name:     "allowed domain",
			allowed:  []string{"example.com", "test.org"},
			url:      "https://example.com/page",
			expected: true,
		},
		{
			name:     "blocked domain",
			allowed:  []string{"example.com"},
			url:      "https://evil.com/page",
			expected: false,
		},
		{
			name:     "subdomain match",
			allowed:  []string{"example.com"},
			url:      "https://sub.example.com/page",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(ManagerConfig{
				Headless:       true,
				AllowedDomains: tt.allowed,
			})
			got := mgr.isDomainAllowed(tt.url)
			if got != tt.expected {
				t.Errorf("isDomainAllowed(%q) = %v, want %v", tt.url, got, tt.expected)
			}
		})
	}
}

func TestBrowserTool_Name(t *testing.T) {
	tool := NewBrowserTool(nil)
	if tool.Name() != "browser" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "browser")
	}
}

func TestBrowserTool_Description(t *testing.T) {
	tool := NewBrowserTool(nil)
	desc := tool.Description()
	if desc == "" {
		t.Error("Description() is empty")
	}
	// Should mention key actions
	for _, keyword := range []string{"navigate", "click", "screenshot", "extract"} {
		if !contains(desc, keyword) {
			t.Errorf("Description() missing keyword %q", keyword)
		}
	}
}

func TestBrowserTool_Parameters(t *testing.T) {
	tool := NewBrowserTool(nil)
	params := tool.Parameters()

	if params["type"] != "object" {
		t.Error("Parameters type should be 'object'")
	}

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("Parameters missing 'properties'")
	}

	required := []string{"action", "url", "selector", "text", "javascript", "session"}
	for _, key := range required {
		if _, ok := props[key]; !ok {
			t.Errorf("Parameters missing property %q", key)
		}
	}
}

func TestBrowserTool_Execute_MissingAction(t *testing.T) {
	tool := NewBrowserTool(nil)
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{})
	if !result.IsError {
		t.Error("expected error for missing action")
	}
	if !contains(result.ForLLM, "action is required") {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
}

func TestBrowserTool_Execute_UnknownAction(t *testing.T) {
	tool := NewBrowserTool(nil)
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "dance",
	})
	if !result.IsError {
		t.Error("expected error for unknown action")
	}
	if !contains(result.ForLLM, "unknown action") {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
}

func TestBrowserTool_Execute_NavigateMissingURL(t *testing.T) {
	tool := NewBrowserTool(nil)
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "navigate",
	})
	if !result.IsError {
		t.Error("expected error for missing url")
	}
}

func TestBrowserTool_Execute_ClickMissingSelector(t *testing.T) {
	tool := NewBrowserTool(nil)
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "click",
	})
	if !result.IsError {
		t.Error("expected error for missing selector")
	}
}

func TestBrowserTool_Execute_TypeMissingArgs(t *testing.T) {
	tool := NewBrowserTool(nil)
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "type",
	})
	if !result.IsError {
		t.Error("expected error for missing type args")
	}
}

func TestBrowserTool_Execute_EvaluateMissingJS(t *testing.T) {
	tool := NewBrowserTool(nil)
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "evaluate",
	})
	if !result.IsError {
		t.Error("expected error for missing javascript")
	}
}

func TestBrowserTool_Execute_ExtractMissingSelector(t *testing.T) {
	tool := NewBrowserTool(nil)
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "extract",
	})
	if !result.IsError {
		t.Error("expected error for missing selector")
	}
}

func TestBrowserTool_Execute_WaitForMissingSelector(t *testing.T) {
	tool := NewBrowserTool(nil)
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "wait_for",
	})
	if !result.IsError {
		t.Error("expected error for missing selector")
	}
}

func TestBrowserTool_Execute_SetCookieMissingArgs(t *testing.T) {
	tool := NewBrowserTool(nil)
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "set_cookie",
	})
	if !result.IsError {
		t.Error("expected error for missing cookie args")
	}
}

func TestBrowserTool_Execute_ListSessions(t *testing.T) {
	tool := NewBrowserTool(nil)
	defer tool.Close()
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "list_sessions",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "Active sessions") {
		t.Errorf("unexpected result: %s", result.ForLLM)
	}
}

func TestBrowserTool_Execute_NewSessionMissingName(t *testing.T) {
	tool := NewBrowserTool(nil)
	defer tool.Close()
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "new_session",
	})
	if !result.IsError {
		t.Error("expected error for missing session name")
	}
}

func TestBrowserTool_Execute_CloseSessionMissing(t *testing.T) {
	tool := NewBrowserTool(nil)
	defer tool.Close()
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action":  "close_session",
		"session": "nonexistent",
	})
	if !result.IsError {
		t.Error("expected error for nonexistent session")
	}
}

func TestStringArg(t *testing.T) {
	args := map[string]any{
		"present": "hello",
		"empty":   "",
		"number":  42,
	}

	if got := stringArg(args, "present", "default"); got != "hello" {
		t.Errorf("stringArg(present) = %q, want %q", got, "hello")
	}
	if got := stringArg(args, "empty", "default"); got != "default" {
		t.Errorf("stringArg(empty) = %q, want %q", got, "default")
	}
	if got := stringArg(args, "missing", "default"); got != "default" {
		t.Errorf("stringArg(missing) = %q, want %q", got, "default")
	}
	if got := stringArg(args, "number", "default"); got != "default" {
		t.Errorf("stringArg(number) = %q, want %q", got, "default")
	}
}

func TestActionResult_Structure(t *testing.T) {
	result := &ActionResult{
		Action:  "test",
		Success: true,
		Data: map[string]any{
			"key": "value",
		},
	}

	if result.Action != "test" {
		t.Errorf("Action = %q, want %q", result.Action, "test")
	}
	if !result.Success {
		t.Error("Success should be true")
	}
	if result.Data["key"] != "value" {
		t.Error("Data[key] should be 'value'")
	}
}

// ---- Integration tests (require Chromium, skipped in CI) ----

func skipIfNoChrome(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

func TestIntegration_NavigateAndGetText(t *testing.T) {
	skipIfNoChrome(t)

	tool := NewBrowserTool(&ManagerConfig{Headless: true})
	defer tool.Close()
	ctx := context.Background()

	// Navigate to a simple page
	result := tool.Execute(ctx, map[string]any{
		"action": "navigate",
		"url":    "https://example.com",
	})
	if result.IsError {
		t.Fatalf("navigate failed: %s", result.ForLLM)
	}

	// Get page text
	result = tool.Execute(ctx, map[string]any{
		"action":     "get_text",
		"max_length": float64(2000),
	})
	if result.IsError {
		t.Fatalf("get_text failed: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "Example Domain") {
		t.Errorf("expected page to contain 'Example Domain', got: %s", result.ForLLM)
	}
}

func TestIntegration_Screenshot(t *testing.T) {
	skipIfNoChrome(t)

	tool := NewBrowserTool(&ManagerConfig{Headless: true})
	defer tool.Close()
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "navigate",
		"url":    "https://example.com",
	})
	if result.IsError {
		t.Fatalf("navigate failed: %s", result.ForLLM)
	}

	result = tool.Execute(ctx, map[string]any{
		"action": "screenshot",
	})
	if result.IsError {
		t.Fatalf("screenshot failed: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "screenshot completed") {
		t.Errorf("unexpected result: %s", result.ForLLM)
	}
}

func TestIntegration_Evaluate(t *testing.T) {
	skipIfNoChrome(t)

	tool := NewBrowserTool(&ManagerConfig{Headless: true})
	defer tool.Close()
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "navigate",
		"url":    "https://example.com",
	})
	if result.IsError {
		t.Fatalf("navigate failed: %s", result.ForLLM)
	}

	result = tool.Execute(ctx, map[string]any{
		"action":     "evaluate",
		"javascript": "document.title",
	})
	if result.IsError {
		t.Fatalf("evaluate failed: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "Example Domain") {
		t.Errorf("expected title to contain 'Example Domain', got: %s", result.ForLLM)
	}
}

func TestIntegration_SessionIsolation(t *testing.T) {
	skipIfNoChrome(t)

	tool := NewBrowserTool(&ManagerConfig{Headless: true})
	defer tool.Close()
	ctx := context.Background()

	// Create two sessions
	result := tool.Execute(ctx, map[string]any{
		"action":  "new_session",
		"session": "alpha",
	})
	if result.IsError {
		t.Fatalf("new_session alpha failed: %s", result.ForLLM)
	}

	result = tool.Execute(ctx, map[string]any{
		"action":  "new_session",
		"session": "beta",
	})
	if result.IsError {
		t.Fatalf("new_session beta failed: %s", result.ForLLM)
	}

	// List should show both
	result = tool.Execute(ctx, map[string]any{
		"action": "list_sessions",
	})
	if !contains(result.ForLLM, "alpha") || !contains(result.ForLLM, "beta") {
		t.Errorf("expected both sessions listed: %s", result.ForLLM)
	}

	// Navigate each to different pages
	result = tool.Execute(ctx, map[string]any{
		"action":  "navigate",
		"url":     "https://example.com",
		"session": "alpha",
	})
	if result.IsError {
		t.Fatalf("navigate alpha failed: %s", result.ForLLM)
	}

	result = tool.Execute(ctx, map[string]any{
		"action":  "navigate",
		"url":     "https://example.org",
		"session": "beta",
	})
	if result.IsError {
		t.Fatalf("navigate beta failed: %s", result.ForLLM)
	}

	// Close one
	result = tool.Execute(ctx, map[string]any{
		"action":  "close_session",
		"session": "alpha",
	})
	if result.IsError {
		t.Fatalf("close_session alpha failed: %s", result.ForLLM)
	}
}

func TestIntegration_DomainRestriction(t *testing.T) {
	skipIfNoChrome(t)

	tool := NewBrowserTool(&ManagerConfig{
		Headless:       true,
		AllowedDomains: []string{"example.com"},
	})
	defer tool.Close()
	ctx := context.Background()

	// Allowed domain should work
	result := tool.Execute(ctx, map[string]any{
		"action": "navigate",
		"url":    "https://example.com",
	})
	if result.IsError {
		t.Fatalf("allowed domain failed: %s", result.ForLLM)
	}

	// Blocked domain should fail
	result = tool.Execute(ctx, map[string]any{
		"action": "navigate",
		"url":    "https://evil.com",
	})
	if !result.IsError {
		t.Error("expected error for blocked domain")
	}
}

// helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
