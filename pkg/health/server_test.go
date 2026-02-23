package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.ready {
		t.Fatal("new server should not be ready")
	}
	if len(s.checks) != 0 {
		t.Fatal("expected empty checks map")
	}
}

func TestHealthHandler(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	s.healthHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("expected status ok, got %s", body.Status)
	}
	if body.Uptime == "" {
		t.Error("expected non-empty uptime")
	}
}

func TestReadyHandler_NotReady(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	// ready is false by default

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	s.readyHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}

	var body StatusResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Status != "not ready" {
		t.Errorf("expected 'not ready', got '%s'", body.Status)
	}
}

func TestReadyHandler_Ready(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	s.SetReady(true)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	s.readyHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body StatusResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Status != "ready" {
		t.Errorf("expected 'ready', got '%s'", body.Status)
	}
}

func TestSetReady(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	if s.ready {
		t.Fatal("should start not ready")
	}

	s.SetReady(true)
	if !s.ready {
		t.Fatal("expected ready after SetReady(true)")
	}

	s.SetReady(false)
	if s.ready {
		t.Fatal("expected not ready after SetReady(false)")
	}
}

func TestRegisterCheck_Passing(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	s.SetReady(true)

	s.RegisterCheck("database", func() (bool, string) {
		return true, "connected"
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	s.readyHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body StatusResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Status != "ready" {
		t.Errorf("expected 'ready', got '%s'", body.Status)
	}

	dbCheck, ok := body.Checks["database"]
	if !ok {
		t.Fatal("expected database check in response")
	}
	if dbCheck.Status != "ok" {
		t.Errorf("expected check status 'ok', got '%s'", dbCheck.Status)
	}
	if dbCheck.Message != "connected" {
		t.Errorf("expected message 'connected', got '%s'", dbCheck.Message)
	}
}

func TestRegisterCheck_Failing(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	s.SetReady(true)

	s.RegisterCheck("redis", func() (bool, string) {
		return false, "connection refused"
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	s.readyHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}

	var body StatusResponse
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Status != "not ready" {
		t.Errorf("expected 'not ready' with failing check, got '%s'", body.Status)
	}
}

func TestMultipleChecks_MixedStatus(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	s.SetReady(true)

	s.RegisterCheck("database", func() (bool, string) {
		return true, "connected"
	})
	s.RegisterCheck("cache", func() (bool, string) {
		return false, "timeout"
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	s.readyHandler(w, req)

	resp := w.Result()
	// Should be unavailable because cache check fails
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestStopServer(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	s.SetReady(true)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := s.Stop(ctx)
	if err != nil {
		t.Fatalf("unexpected error stopping server: %v", err)
	}
	if s.ready {
		t.Fatal("server should not be ready after stop")
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		input    bool
		expected string
	}{
		{true, "ok"},
		{false, "fail"},
	}

	for _, tt := range tests {
		got := statusString(tt.input)
		if got != tt.expected {
			t.Errorf("statusString(%v) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestCheckSerialization(t *testing.T) {
	check := Check{
		Name:      "db",
		Status:    "ok",
		Message:   "healthy",
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(check)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Check
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Name != "db" {
		t.Errorf("wrong name: %s", decoded.Name)
	}
	if decoded.Status != "ok" {
		t.Errorf("wrong status: %s", decoded.Status)
	}
}

func TestStatusResponseSerialization(t *testing.T) {
	resp := StatusResponse{
		Status: "ready",
		Uptime: "5m30s",
		Checks: map[string]Check{
			"db": {Name: "db", Status: "ok"},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded StatusResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Status != "ready" {
		t.Errorf("wrong status: %s", decoded.Status)
	}
	if len(decoded.Checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(decoded.Checks))
	}
}

func TestHealthContentType(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	s.healthHandler(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}
