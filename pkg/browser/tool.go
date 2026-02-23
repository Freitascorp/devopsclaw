package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/freitascorp/devopsclaw/pkg/tools"
)

// BrowserTool implements tools.Tool for browser automation.
// It dispatches LLM tool calls to the underlying Session methods.
//
// Supported actions:
//   - navigate: Open a URL
//   - click: Click an element by CSS selector
//   - type: Type text into an input
//   - screenshot: Capture page screenshot (base64 PNG)
//   - evaluate: Execute JavaScript
//   - extract: Extract text/attributes from elements
//   - wait_for: Wait for an element to appear
//   - scroll: Scroll the page
//   - get_text: Get all page text
//   - page_info: Get current page title/URL/dimensions
//   - hover: Hover over an element
//   - select: Select an option in a <select> element
//   - get_cookies: List all cookies
//   - set_cookie: Set a cookie
//   - pdf: Generate PDF (base64)
//   - new_session: Create a new isolated session
//   - close_session: Close a session
//   - list_sessions: List active sessions
type BrowserTool struct {
	manager *Manager
}

// NewBrowserTool creates a new browser tool with the given config.
// If config is nil, defaults are used (headless, pool size 4, 30s timeout).
func NewBrowserTool(config *ManagerConfig) *BrowserTool {
	if config == nil {
		config = &ManagerConfig{
			Headless: true,
		}
	}
	return &BrowserTool{
		manager: NewManager(*config),
	}
}

// NewBrowserToolWithManager creates a browser tool with an existing manager.
// Useful for testing or sharing a manager across tools.
func NewBrowserToolWithManager(mgr *Manager) *BrowserTool {
	return &BrowserTool{manager: mgr}
}

func (t *BrowserTool) Name() string {
	return "browser"
}

func (t *BrowserTool) Description() string {
	return `Automate a web browser to navigate pages, interact with elements, take screenshots, and extract data. ` +
		`Actions: navigate, click, type, screenshot, evaluate, extract, wait_for, scroll, get_text, ` +
		`page_info, hover, select, get_cookies, set_cookie, pdf, new_session, close_session, list_sessions.`
}

func (t *BrowserTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"description": "The browser action to perform. One of: navigate, click, type, screenshot, " +
					"evaluate, extract, wait_for, scroll, get_text, page_info, hover, select, " +
					"get_cookies, set_cookie, pdf, new_session, close_session, list_sessions",
				"enum": []string{
					"navigate", "click", "type", "screenshot", "evaluate",
					"extract", "wait_for", "scroll", "get_text", "page_info",
					"hover", "select", "get_cookies", "set_cookie", "pdf",
					"new_session", "close_session", "list_sessions",
				},
			},
			"url": map[string]any{
				"type":        "string",
				"description": "URL to navigate to (for 'navigate' action)",
			},
			"selector": map[string]any{
				"type":        "string",
				"description": "CSS selector for targeting elements (for click, type, extract, wait_for, hover, select)",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "Text to type into an input field (for 'type' action)",
			},
			"clear": map[string]any{
				"type":        "boolean",
				"description": "Clear the input field before typing (for 'type' action, default false)",
			},
			"javascript": map[string]any{
				"type":        "string",
				"description": "JavaScript code to evaluate on the page (for 'evaluate' action)",
			},
			"attribute": map[string]any{
				"type":        "string",
				"description": "HTML attribute to extract (for 'extract' action, e.g. 'href', 'src')",
			},
			"full_page": map[string]any{
				"type":        "boolean",
				"description": "Capture full scrollable page (for 'screenshot' action, default false)",
			},
			"scroll_x": map[string]any{
				"type":        "number",
				"description": "Horizontal scroll pixels (for 'scroll' action)",
			},
			"scroll_y": map[string]any{
				"type":        "number",
				"description": "Vertical scroll pixels (for 'scroll' action, positive = down)",
			},
			"max_length": map[string]any{
				"type":        "integer",
				"description": "Maximum text length to return (for 'get_text' action, default 8000)",
			},
			"values": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Values to select (for 'select' action)",
			},
			"cookie_name": map[string]any{
				"type":        "string",
				"description": "Cookie name (for 'set_cookie' action)",
			},
			"cookie_value": map[string]any{
				"type":        "string",
				"description": "Cookie value (for 'set_cookie' action)",
			},
			"cookie_domain": map[string]any{
				"type":        "string",
				"description": "Cookie domain (for 'set_cookie' action)",
			},
			"cookie_path": map[string]any{
				"type":        "string",
				"description": "Cookie path (for 'set_cookie' action, default '/')",
			},
			"session": map[string]any{
				"type":        "string",
				"description": "Session name for isolation (default 'default'). Use new_session to create.",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds for the action (default: manager default)",
			},
		},
		"required": []string{"action"},
	}
}

// Execute dispatches the action to the appropriate session method.
func (t *BrowserTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	action, _ := args["action"].(string)
	if action == "" {
		return tools.ErrorResult("action is required")
	}

	// Handle session management actions first (no session needed)
	switch action {
	case "new_session":
		return t.doNewSession(args)
	case "close_session":
		return t.doCloseSession(args)
	case "list_sessions":
		return t.doListSessions()
	}

	// Get or create session
	sessionName := stringArg(args, "session", "default")
	sess, err := t.manager.NewSession(sessionName) // idempotent — returns existing
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("session error: %v", err))
	}

	// Apply per-action timeout if specified
	if timeoutSec, ok := args["timeout"].(float64); ok && timeoutSec > 0 {
		sess.timeout = time.Duration(timeoutSec) * time.Second
	}

	var result *ActionResult
	switch action {
	case "navigate":
		url, _ := args["url"].(string)
		if url == "" {
			return tools.ErrorResult("url is required for navigate action")
		}
		result, err = sess.Navigate(ctx, url)

	case "click":
		selector := stringArg(args, "selector", "")
		if selector == "" {
			return tools.ErrorResult("selector is required for click action")
		}
		result, err = sess.Click(ctx, selector)

	case "type":
		selector := stringArg(args, "selector", "")
		text := stringArg(args, "text", "")
		if selector == "" || text == "" {
			return tools.ErrorResult("selector and text are required for type action")
		}
		clear, _ := args["clear"].(bool)
		result, err = sess.Type(ctx, selector, text, clear)

	case "screenshot":
		fullPage, _ := args["full_page"].(bool)
		result, err = sess.Screenshot(ctx, fullPage)

	case "evaluate":
		js := stringArg(args, "javascript", "")
		if js == "" {
			return tools.ErrorResult("javascript is required for evaluate action")
		}
		result, err = sess.Evaluate(ctx, js)

	case "extract":
		selector := stringArg(args, "selector", "")
		if selector == "" {
			return tools.ErrorResult("selector is required for extract action")
		}
		attr := stringArg(args, "attribute", "")
		result, err = sess.Extract(ctx, selector, attr)

	case "wait_for":
		selector := stringArg(args, "selector", "")
		if selector == "" {
			return tools.ErrorResult("selector is required for wait_for action")
		}
		var timeout time.Duration
		if timeoutSec, ok := args["timeout"].(float64); ok {
			timeout = time.Duration(timeoutSec) * time.Second
		}
		result, err = sess.WaitFor(ctx, selector, timeout)

	case "scroll":
		scrollX, _ := args["scroll_x"].(float64)
		scrollY, _ := args["scroll_y"].(float64)
		result, err = sess.Scroll(ctx, scrollX, scrollY)

	case "get_text":
		maxLen := 8000
		if ml, ok := args["max_length"].(float64); ok && ml > 0 {
			maxLen = int(ml)
		}
		result, err = sess.GetText(ctx, maxLen)

	case "page_info":
		result, err = sess.GetPageInfo(ctx)

	case "hover":
		selector := stringArg(args, "selector", "")
		if selector == "" {
			return tools.ErrorResult("selector is required for hover action")
		}
		result, err = sess.Hover(ctx, selector)

	case "select":
		selector := stringArg(args, "selector", "")
		if selector == "" {
			return tools.ErrorResult("selector is required for select action")
		}
		var values []string
		if v, ok := args["values"].([]any); ok {
			for _, item := range v {
				if s, ok := item.(string); ok {
					values = append(values, s)
				}
			}
		}
		result, err = sess.SelectOption(ctx, selector, values)

	case "get_cookies":
		result, err = sess.GetCookies(ctx)

	case "set_cookie":
		name := stringArg(args, "cookie_name", "")
		value := stringArg(args, "cookie_value", "")
		domain := stringArg(args, "cookie_domain", "")
		path := stringArg(args, "cookie_path", "/")
		if name == "" || value == "" {
			return tools.ErrorResult("cookie_name and cookie_value are required for set_cookie")
		}
		result, err = sess.SetCookie(ctx, name, value, domain, path)

	case "pdf":
		result, err = sess.PDF(ctx)

	default:
		return tools.ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}

	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("%s failed: %v", action, err))
	}

	return t.formatResult(result)
}

// Close shuts down the browser manager and all sessions.
func (t *BrowserTool) Close() error {
	return t.manager.Close()
}

// ---- session management actions ----

func (t *BrowserTool) doNewSession(args map[string]any) *tools.ToolResult {
	name := stringArg(args, "session", "")
	if name == "" {
		return tools.ErrorResult("session name is required for new_session")
	}
	_, err := t.manager.NewSession(name)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to create session: %v", err))
	}
	return tools.SilentResult(fmt.Sprintf("Session '%s' created", name))
}

func (t *BrowserTool) doCloseSession(args map[string]any) *tools.ToolResult {
	name := stringArg(args, "session", "")
	if name == "" {
		return tools.ErrorResult("session name is required for close_session")
	}
	if err := t.manager.CloseSession(name); err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to close session: %v", err))
	}
	return tools.SilentResult(fmt.Sprintf("Session '%s' closed", name))
}

func (t *BrowserTool) doListSessions() *tools.ToolResult {
	names := t.manager.ListSessions()
	return tools.SilentResult(fmt.Sprintf("Active sessions: %s", strings.Join(names, ", ")))
}

// ---- helpers ----

func (t *BrowserTool) formatResult(result *ActionResult) *tools.ToolResult {
	// For screenshots and PDFs, return a summary (base64 is huge)
	if result.Action == "screenshot" || result.Action == "pdf" {
		size, _ := result.Data["size"].(int)
		summary := fmt.Sprintf("%s completed: %d bytes captured", result.Action, size)
		// Attach the full data as JSON for downstream processing
		fullJSON, _ := json.Marshal(result.Data)
		return &tools.ToolResult{
			ForLLM:  summary,
			ForUser: string(fullJSON),
			Silent:  false,
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		return tools.SilentResult(fmt.Sprintf("%s completed", result.Action))
	}

	return tools.SilentResult(string(data))
}

func stringArg(args map[string]any, key, defaultVal string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return defaultVal
}
