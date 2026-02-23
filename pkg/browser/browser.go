// Package browser provides browser automation capabilities for DevOpsClaw using Rod.
//
// It wraps the go-rod/rod Chrome DevTools Protocol driver to provide:
//   - Session management with automatic Chromium lifecycle
//   - Page pool for concurrent automation
//   - Navigation, interaction, extraction, and screenshot actions
//   - Incognito contexts for session isolation
//   - Configurable timeouts, viewport, and user-agent
//
// Usage:
//
//	mgr := browser.NewManager(browser.ManagerConfig{
//	    Headless:    true,
//	    PoolSize:    4,
//	    DefaultTimeout: 30 * time.Second,
//	})
//	defer mgr.Close()
//
//	sess, _ := mgr.NewSession("main")
//	result, _ := sess.Navigate(ctx, "https://example.com")
package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// ManagerConfig configures the browser manager.
type ManagerConfig struct {
	// Headless runs the browser without a visible window.
	Headless bool

	// PoolSize is the max number of concurrent pages in the page pool.
	// Default: 4.
	PoolSize int

	// DefaultTimeout is the timeout for page operations.
	// Default: 30s.
	DefaultTimeout time.Duration

	// ViewportWidth and ViewportHeight set the default viewport size.
	// Default: 1280x720.
	ViewportWidth  int
	ViewportHeight int

	// UserAgent overrides the browser's user agent string.
	// If empty, the default Chromium user agent is used.
	UserAgent string

	// BrowserBin is the path to a Chrome/Chromium binary.
	// If empty, Rod will auto-download one.
	BrowserBin string

	// AllowedDomains restricts navigation to these domains.
	// If empty, all domains are allowed.
	AllowedDomains []string
}

func (c *ManagerConfig) defaults() {
	if c.PoolSize <= 0 {
		c.PoolSize = 4
	}
	if c.DefaultTimeout <= 0 {
		c.DefaultTimeout = 30 * time.Second
	}
	if c.ViewportWidth <= 0 {
		c.ViewportWidth = 1280
	}
	if c.ViewportHeight <= 0 {
		c.ViewportHeight = 720
	}
}

// Manager manages the browser lifecycle and session pool.
type Manager struct {
	config   ManagerConfig
	browser  *rod.Browser
	sessions map[string]*Session
	mu       sync.Mutex
	closed   bool
}

// NewManager creates a new browser manager. It lazily connects the browser
// on the first session creation to avoid unnecessary Chromium downloads.
func NewManager(config ManagerConfig) *Manager {
	config.defaults()
	return &Manager{
		config:   config,
		sessions: make(map[string]*Session),
	}
}

// ensureBrowser connects to or launches the browser instance.
func (m *Manager) ensureBrowser() error {
	if m.browser != nil {
		return nil
	}

	l := launcher.New()
	if m.config.BrowserBin != "" {
		l = l.Bin(m.config.BrowserBin)
	}
	if m.config.Headless {
		l = l.Headless(true)
	} else {
		l = l.Headless(false)
	}

	controlURL, err := l.Launch()
	if err != nil {
		return fmt.Errorf("browser launch failed: %w", err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return fmt.Errorf("browser connect failed: %w", err)
	}

	m.browser = browser
	return nil
}

// NewSession creates or retrieves a named session.
// Each session is an incognito browser context with its own cookies and storage.
func (m *Manager) NewSession(name string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("manager is closed")
	}

	if sess, ok := m.sessions[name]; ok {
		return sess, nil
	}

	if err := m.ensureBrowser(); err != nil {
		return nil, err
	}

	// Create an incognito context for session isolation
	incognito, err := m.browser.Incognito()
	if err != nil {
		return nil, fmt.Errorf("incognito context failed: %w", err)
	}

	sess := &Session{
		name:      name,
		context:   incognito,
		manager:   m,
		pages:     make(map[string]*rod.Page),
		timeout:   m.config.DefaultTimeout,
		vpWidth:   m.config.ViewportWidth,
		vpHeight:  m.config.ViewportHeight,
		userAgent: m.config.UserAgent,
	}

	m.sessions[name] = sess
	return sess, nil
}

// GetSession returns an existing session by name.
func (m *Manager) GetSession(name string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[name]
	return sess, ok
}

// CloseSession closes and removes a named session.
func (m *Manager) CloseSession(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[name]
	if !ok {
		return fmt.Errorf("session %q not found", name)
	}

	sess.close()
	delete(m.sessions, name)
	return nil
}

// ListSessions returns the names of all active sessions.
func (m *Manager) ListSessions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := make([]string, 0, len(m.sessions))
	for name := range m.sessions {
		names = append(names, name)
	}
	return names
}

// Close shuts down all sessions and the browser.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	for _, sess := range m.sessions {
		sess.close()
	}
	m.sessions = make(map[string]*Session)

	if m.browser != nil {
		return m.browser.Close()
	}
	return nil
}

// isDomainAllowed checks if a URL's domain is in the allowed list.
func (m *Manager) isDomainAllowed(url string) bool {
	if len(m.config.AllowedDomains) == 0 {
		return true
	}
	for _, domain := range m.config.AllowedDomains {
		if strings.Contains(url, domain) {
			return true
		}
	}
	return false
}

// Session represents an isolated browser session with its own cookie jar and storage.
type Session struct {
	name      string
	context   *rod.Browser // incognito browser context
	manager   *Manager
	pages     map[string]*rod.Page
	activePage *rod.Page
	mu        sync.Mutex
	timeout   time.Duration
	vpWidth   int
	vpHeight  int
	userAgent string
}

// Navigate opens a URL in the session. Returns the page title and URL.
func (s *Session) Navigate(ctx context.Context, url string) (*ActionResult, error) {
	if !s.manager.isDomainAllowed(url) {
		return nil, fmt.Errorf("domain not allowed: %s", url)
	}

	page, err := s.getOrCreatePage(ctx, "default")
	if err != nil {
		return nil, err
	}

	err = page.Timeout(s.timeout).Navigate(url)
	if err != nil {
		return nil, fmt.Errorf("navigate failed: %w", err)
	}

	// Wait for the page to stabilize
	err = page.Timeout(s.timeout).WaitStable(300 * time.Millisecond)
	if err != nil {
		// Not fatal — some pages never fully stabilize
	}

	info, err := page.Info()
	if err != nil {
		return nil, fmt.Errorf("page info failed: %w", err)
	}

	return &ActionResult{
		Action:  "navigate",
		Success: true,
		Data: map[string]any{
			"title": info.Title,
			"url":   info.URL,
		},
	}, nil
}

// Click clicks an element matching the CSS selector.
func (s *Session) Click(ctx context.Context, selector string) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	el, err := page.Timeout(s.timeout).Element(selector)
	if err != nil {
		return nil, fmt.Errorf("element not found: %s: %w", selector, err)
	}

	err = el.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return nil, fmt.Errorf("click failed: %w", err)
	}

	// Wait briefly for any navigation or AJAX
	_ = page.WaitStable(200 * time.Millisecond)

	return &ActionResult{
		Action:  "click",
		Success: true,
		Data: map[string]any{
			"selector": selector,
		},
	}, nil
}

// Type types text into an element matching the CSS selector.
// If clear is true, the field is cleared before typing.
func (s *Session) Type(ctx context.Context, selector, text string, clear bool) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	el, err := page.Timeout(s.timeout).Element(selector)
	if err != nil {
		return nil, fmt.Errorf("element not found: %s: %w", selector, err)
	}

	if clear {
		err = el.SelectAllText()
		if err != nil {
			return nil, fmt.Errorf("select text failed: %w", err)
		}
	}

	err = el.Input(text)
	if err != nil {
		return nil, fmt.Errorf("type failed: %w", err)
	}

	return &ActionResult{
		Action:  "type",
		Success: true,
		Data: map[string]any{
			"selector": selector,
			"text":     text,
		},
	}, nil
}

// Screenshot captures a screenshot of the current page.
// If fullPage is true, captures the entire scrollable area.
// Returns base64-encoded PNG data.
func (s *Session) Screenshot(ctx context.Context, fullPage bool) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	var data []byte
	if fullPage {
		data, err = page.Timeout(s.timeout).Screenshot(true, nil)
	} else {
		data, err = page.Timeout(s.timeout).Screenshot(false, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("screenshot failed: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return &ActionResult{
		Action:  "screenshot",
		Success: true,
		Data: map[string]any{
			"base64":    encoded,
			"full_page": fullPage,
			"size":      len(data),
		},
	}, nil
}

// Evaluate executes JavaScript on the page and returns the result.
// The js argument can be a raw expression (e.g. "document.title") or
// an arrow/function expression (e.g. "() => document.title").
// Raw expressions are automatically wrapped in an arrow function for Rod.
func (s *Session) Evaluate(ctx context.Context, js string) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	// Rod's Eval expects a function expression. Wrap raw expressions.
	wrapped := wrapJSExpression(js)

	result, err := page.Timeout(s.timeout).Eval(wrapped)
	if err != nil {
		return nil, fmt.Errorf("eval failed: %w", err)
	}

	return &ActionResult{
		Action:  "evaluate",
		Success: true,
		Data: map[string]any{
			"result": result.Value.Val(),
		},
	}, nil
}

// wrapJSExpression wraps a raw JS expression in an arrow function if it isn't
// already a function. Rod's Eval expects `() => expr` or `function() {...}`.
func wrapJSExpression(js string) string {
	trimmed := strings.TrimSpace(js)
	// Already a function expression — leave it alone
	if strings.HasPrefix(trimmed, "()") ||
		strings.HasPrefix(trimmed, "function") ||
		strings.HasPrefix(trimmed, "(function") ||
		strings.HasPrefix(trimmed, "(()") {
		return js
	}
	return "() => " + js
}

// Extract extracts text content from elements matching the selector.
func (s *Session) Extract(ctx context.Context, selector string, attribute string) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	elements, err := page.Timeout(s.timeout).Elements(selector)
	if err != nil {
		return nil, fmt.Errorf("elements not found: %s: %w", selector, err)
	}

	var results []map[string]string
	for _, el := range elements {
		entry := map[string]string{}

		text, err := el.Text()
		if err == nil {
			entry["text"] = text
		}

		if attribute != "" {
			val, err := el.Attribute(attribute)
			if err == nil && val != nil {
				entry[attribute] = *val
			}
		}

		// Always try to get href for links
		if attribute != "href" {
			href, err := el.Attribute("href")
			if err == nil && href != nil {
				entry["href"] = *href
			}
		}

		results = append(results, entry)
	}

	return &ActionResult{
		Action:  "extract",
		Success: true,
		Data: map[string]any{
			"selector": selector,
			"count":    len(results),
			"elements": results,
		},
	}, nil
}

// WaitFor waits for an element matching the selector to appear.
func (s *Session) WaitFor(ctx context.Context, selector string, timeout time.Duration) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	if timeout <= 0 {
		timeout = s.timeout
	}

	start := time.Now()
	_, err = page.Timeout(timeout).Element(selector)
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("wait timed out for %s after %v: %w", selector, elapsed, err)
	}

	return &ActionResult{
		Action:  "wait_for",
		Success: true,
		Data: map[string]any{
			"selector": selector,
			"elapsed":  elapsed.String(),
		},
	}, nil
}

// Scroll scrolls the page by the given pixel amounts.
// Use negative values to scroll up/left.
func (s *Session) Scroll(ctx context.Context, x, y float64) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	// Use JavaScript to scroll
	_, err = page.Eval(fmt.Sprintf("() => window.scrollBy(%f, %f)", x, y))
	if err != nil {
		return nil, fmt.Errorf("scroll failed: %w", err)
	}

	return &ActionResult{
		Action:  "scroll",
		Success: true,
		Data: map[string]any{
			"x": x,
			"y": y,
		},
	}, nil
}

// GetPageInfo returns information about the current page.
func (s *Session) GetPageInfo(ctx context.Context) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	info, err := page.Info()
	if err != nil {
		return nil, fmt.Errorf("page info failed: %w", err)
	}

	// Get scroll dimensions
	dims, err := page.Eval(`() => JSON.stringify({
		scrollWidth: document.documentElement.scrollWidth,
		scrollHeight: document.documentElement.scrollHeight,
		clientWidth: document.documentElement.clientWidth,
		clientHeight: document.documentElement.clientHeight,
		scrollX: window.scrollX,
		scrollY: window.scrollY,
	})`)

	data := map[string]any{
		"title": info.Title,
		"url":   info.URL,
	}
	if err == nil {
		data["dimensions"] = dims.Value.Str()
	}

	return &ActionResult{
		Action:  "page_info",
		Success: true,
		Data:    data,
	}, nil
}

// SetCookie sets a cookie in the session.
func (s *Session) SetCookie(ctx context.Context, name, value, domain, path string) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	cookie := &proto.NetworkCookieParam{
		Name:   name,
		Value:  value,
		Domain: domain,
		Path:   path,
	}

	err = page.SetCookies([]*proto.NetworkCookieParam{cookie})
	if err != nil {
		return nil, fmt.Errorf("set cookie failed: %w", err)
	}

	return &ActionResult{
		Action:  "set_cookie",
		Success: true,
		Data: map[string]any{
			"name":   name,
			"domain": domain,
		},
	}, nil
}

// GetCookies returns all cookies for the current page.
func (s *Session) GetCookies(ctx context.Context) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	cookies, err := page.Cookies(nil)
	if err != nil {
		return nil, fmt.Errorf("get cookies failed: %w", err)
	}

	var cookieData []map[string]any
	for _, c := range cookies {
		cookieData = append(cookieData, map[string]any{
			"name":     c.Name,
			"value":    c.Value,
			"domain":   c.Domain,
			"path":     c.Path,
			"httpOnly": c.HTTPOnly,
			"secure":   c.Secure,
		})
	}

	return &ActionResult{
		Action:  "get_cookies",
		Success: true,
		Data: map[string]any{
			"cookies": cookieData,
			"count":   len(cookieData),
		},
	}, nil
}

// PDF generates a PDF of the current page. Returns base64-encoded PDF data.
func (s *Session) PDF(ctx context.Context) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	reader, err := page.Timeout(s.timeout).PDF(&proto.PagePrintToPDF{
		PrintBackground: true,
	})
	if err != nil {
		return nil, fmt.Errorf("pdf generation failed: %w", err)
	}

	buf := make([]byte, 0, 1<<20) // 1MB initial
	tmp := make([]byte, 32*1024)
	for {
		n, readErr := reader.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if readErr != nil {
			break
		}
	}

	encoded := base64.StdEncoding.EncodeToString(buf)
	return &ActionResult{
		Action:  "pdf",
		Success: true,
		Data: map[string]any{
			"base64": encoded,
			"size":   len(buf),
		},
	}, nil
}

// WaitForNavigation waits for a page navigation to complete.
func (s *Session) WaitForNavigation(ctx context.Context) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	err = page.Timeout(s.timeout).WaitLoad()
	if err != nil {
		return nil, fmt.Errorf("wait for navigation failed: %w", err)
	}

	info, err := page.Info()
	if err != nil {
		return nil, fmt.Errorf("page info failed: %w", err)
	}

	return &ActionResult{
		Action:  "wait_navigation",
		Success: true,
		Data: map[string]any{
			"title": info.Title,
			"url":   info.URL,
		},
	}, nil
}

// Hover hovers over an element matching the CSS selector.
func (s *Session) Hover(ctx context.Context, selector string) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	el, err := page.Timeout(s.timeout).Element(selector)
	if err != nil {
		return nil, fmt.Errorf("element not found: %s: %w", selector, err)
	}

	err = el.Hover()
	if err != nil {
		return nil, fmt.Errorf("hover failed: %w", err)
	}

	return &ActionResult{
		Action:  "hover",
		Success: true,
		Data: map[string]any{
			"selector": selector,
		},
	}, nil
}

// SelectOption selects an option in a <select> element.
func (s *Session) SelectOption(ctx context.Context, selector string, values []string) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	el, err := page.Timeout(s.timeout).Element(selector)
	if err != nil {
		return nil, fmt.Errorf("element not found: %s: %w", selector, err)
	}

	err = el.Select(values, true, rod.SelectorTypeCSSSector)
	if err != nil {
		return nil, fmt.Errorf("select failed: %w", err)
	}

	return &ActionResult{
		Action:  "select",
		Success: true,
		Data: map[string]any{
			"selector": selector,
			"values":   values,
		},
	}, nil
}

// GetText returns the full text content of the page, truncated to maxLen.
func (s *Session) GetText(ctx context.Context, maxLen int) (*ActionResult, error) {
	page, err := s.getActivePage(ctx)
	if err != nil {
		return nil, err
	}

	if maxLen <= 0 {
		maxLen = 8000
	}

	// Extract readable text from the body
	result, err := page.Timeout(s.timeout).Eval(`() => {
		let body = document.body;
		if (!body) return '';
		// Remove script and style elements for cleaner text
		let clone = body.cloneNode(true);
		let scripts = clone.querySelectorAll('script, style, noscript');
		scripts.forEach(s => s.remove());
		return clone.innerText || clone.textContent || '';
	}`)
	if err != nil {
		return nil, fmt.Errorf("get text failed: %w", err)
	}

	text := result.Value.Str()
	truncated := false
	if len(text) > maxLen {
		text = text[:maxLen]
		truncated = true
	}

	return &ActionResult{
		Action:  "get_text",
		Success: true,
		Data: map[string]any{
			"text":      text,
			"truncated": truncated,
			"length":    len(text),
		},
	}, nil
}

// ---- internal helpers ----

func (s *Session) getOrCreatePage(ctx context.Context, id string) (*rod.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if page, ok := s.pages[id]; ok {
		s.activePage = page
		return page, nil
	}

	page, err := s.context.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("create page failed: %w", err)
	}

	// Set viewport
	err = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  s.vpWidth,
		Height: s.vpHeight,
	})
	if err != nil {
		return nil, fmt.Errorf("set viewport failed: %w", err)
	}

	// Set user agent if configured
	if s.userAgent != "" {
		err = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent: s.userAgent,
		})
		if err != nil {
			return nil, fmt.Errorf("set user agent failed: %w", err)
		}
	}

	s.pages[id] = page
	s.activePage = page
	return page, nil
}

func (s *Session) getActivePage(ctx context.Context) (*rod.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activePage == nil {
		return nil, fmt.Errorf("no active page — call navigate first")
	}
	return s.activePage, nil
}

func (s *Session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, page := range s.pages {
		_ = page.Close()
	}
	s.pages = make(map[string]*rod.Page)
	s.activePage = nil
	// close the incognito context
	_ = s.context.Close()
}

// ActionResult is the result of a browser action.
type ActionResult struct {
	Action  string         `json:"action"`
	Success bool           `json:"success"`
	Data    map[string]any `json:"data,omitempty"`
}
