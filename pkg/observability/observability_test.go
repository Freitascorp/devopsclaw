package observability

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ------------------------------------------------------------------
// Counter tests
// ------------------------------------------------------------------

func TestCounter(t *testing.T) {
	r := NewMetricsRegistry()
	c := r.GetCounter("test_counter", "A test counter")

	if c.Value() != 0 {
		t.Errorf("expected initial value 0, got %d", c.Value())
	}

	c.Inc()
	if c.Value() != 1 {
		t.Errorf("expected 1, got %d", c.Value())
	}

	c.Add(5)
	if c.Value() != 6 {
		t.Errorf("expected 6, got %d", c.Value())
	}
}

func TestCounter_GetExisting(t *testing.T) {
	r := NewMetricsRegistry()
	c1 := r.GetCounter("test", "desc")
	c1.Inc()
	c2 := r.GetCounter("test", "desc")

	if c1 != c2 {
		t.Fatal("expected same counter instance")
	}
	if c2.Value() != 1 {
		t.Errorf("expected 1, got %d", c2.Value())
	}
}

// ------------------------------------------------------------------
// Gauge tests
// ------------------------------------------------------------------

func TestGauge(t *testing.T) {
	r := NewMetricsRegistry()
	g := r.GetGauge("test_gauge", "A test gauge")

	if g.Value() != 0 {
		t.Errorf("expected initial value 0, got %d", g.Value())
	}

	g.Set(42)
	if g.Value() != 42 {
		t.Errorf("expected 42, got %d", g.Value())
	}

	g.Inc()
	if g.Value() != 43 {
		t.Errorf("expected 43, got %d", g.Value())
	}

	g.Dec()
	if g.Value() != 42 {
		t.Errorf("expected 42, got %d", g.Value())
	}
}

func TestGauge_GetExisting(t *testing.T) {
	r := NewMetricsRegistry()
	g1 := r.GetGauge("test", "desc")
	g1.Set(10)
	g2 := r.GetGauge("test", "desc")

	if g1 != g2 {
		t.Fatal("expected same gauge instance")
	}
	if g2.Value() != 10 {
		t.Errorf("expected 10, got %d", g2.Value())
	}
}

// ------------------------------------------------------------------
// Histogram tests
// ------------------------------------------------------------------

func TestHistogram(t *testing.T) {
	r := NewMetricsRegistry()
	h := r.GetHistogram("test_hist", "A test histogram", []float64{1, 5, 10, 50})

	h.Observe(0.5)  // bucket <= 1
	h.Observe(3.0)  // bucket <= 5
	h.Observe(7.5)  // bucket <= 10
	h.Observe(25.0) // bucket <= 50
	h.Observe(100)  // +Inf bucket

	if h.count != 5 {
		t.Errorf("expected count 5, got %d", h.count)
	}

	expectedSum := 0.5 + 3.0 + 7.5 + 25.0 + 100.0
	if h.sum != expectedSum {
		t.Errorf("expected sum %f, got %f", expectedSum, h.sum)
	}
}

func TestHistogram_GetExisting(t *testing.T) {
	r := NewMetricsRegistry()
	h1 := r.GetHistogram("test", "desc", []float64{1, 5, 10})
	h1.Observe(2.0)
	h2 := r.GetHistogram("test", "desc", []float64{1, 5, 10})

	if h1 != h2 {
		t.Fatal("expected same histogram instance")
	}
	if h2.count != 1 {
		t.Errorf("expected count 1, got %d", h2.count)
	}
}

func TestHistogram_BucketsSorted(t *testing.T) {
	r := NewMetricsRegistry()
	h := r.GetHistogram("sorted", "desc", []float64{10, 1, 5})

	// Buckets should be sorted
	if h.buckets[0] != 1 || h.buckets[1] != 5 || h.buckets[2] != 10 {
		t.Errorf("buckets not sorted: %v", h.buckets)
	}
}

// ------------------------------------------------------------------
// MetricsRegistry tests
// ------------------------------------------------------------------

func TestMetricsRegistry_ConcurrentAccess(t *testing.T) {
	r := NewMetricsRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := r.GetCounter("concurrent_counter", "test")
			c.Inc()
			g := r.GetGauge("concurrent_gauge", "test")
			g.Inc()
			h := r.GetHistogram("concurrent_hist", "test", []float64{1, 5, 10})
			h.Observe(float64(i))
		}(i)
	}
	wg.Wait()

	c := r.GetCounter("concurrent_counter", "test")
	if c.Value() != 100 {
		t.Errorf("expected counter 100, got %d", c.Value())
	}

	g := r.GetGauge("concurrent_gauge", "test")
	if g.Value() != 100 {
		t.Errorf("expected gauge 100, got %d", g.Value())
	}
}

// ------------------------------------------------------------------
// DevOpsClawMetrics tests
// ------------------------------------------------------------------

func TestNewDevOpsClawMetrics(t *testing.T) {
	m := NewDevOpsClawMetrics()
	if m == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m.Registry == nil {
		t.Fatal("expected non-nil registry")
	}

	// Verify all metrics are initialized
	checks := []struct {
		name   string
		metric interface{ Value() int64 }
	}{
		{"MessagesReceived", m.MessagesReceived},
		{"MessagesProcessed", m.MessagesProcessed},
		{"MessageErrors", m.MessageErrors},
		{"LLMCalls", m.LLMCalls},
		{"LLMErrors", m.LLMErrors},
		{"ToolCalls", m.ToolCalls},
		{"ToolErrors", m.ToolErrors},
		{"ActiveSessions", m.ActiveSessions},
		{"FleetNodesTotal", m.FleetNodesTotal},
		{"FleetNodesOnline", m.FleetNodesOnline},
		{"FleetExecTotal", m.FleetExecTotal},
		{"FleetExecErrors", m.FleetExecErrors},
		{"ChannelMessages", m.ChannelMessages},
		{"ChannelErrors", m.ChannelErrors},
		{"ProviderCalls", m.ProviderCalls},
		{"ProviderErrors", m.ProviderErrors},
		{"ProviderFallbacks", m.ProviderFallbacks},
		{"CircuitBreakerTrips", m.CircuitBreakerTrips},
		{"RateLimitRejects", m.RateLimitRejects},
		{"BulkheadRejects", m.BulkheadRejects},
		{"RetryAttempts", m.RetryAttempts},
		{"Uptime", m.Uptime},
		{"GoroutineCount", m.GoroutineCount},
	}

	for _, check := range checks {
		if check.metric == nil {
			t.Errorf("%s is nil", check.name)
		}
	}

	// Verify histograms
	if m.LLMLatency == nil {
		t.Error("LLMLatency is nil")
	}
	if m.ToolLatency == nil {
		t.Error("ToolLatency is nil")
	}
	if m.FleetExecLatency == nil {
		t.Error("FleetExecLatency is nil")
	}
}

func TestDevOpsClawMetrics_Usage(t *testing.T) {
	m := NewDevOpsClawMetrics()

	m.MessagesReceived.Inc()
	m.MessagesProcessed.Inc()
	m.LLMCalls.Add(3)
	m.ActiveSessions.Set(5)
	m.LLMLatency.Observe(0.25)

	if m.MessagesReceived.Value() != 1 {
		t.Errorf("expected 1, got %d", m.MessagesReceived.Value())
	}
	if m.LLMCalls.Value() != 3 {
		t.Errorf("expected 3, got %d", m.LLMCalls.Value())
	}
	if m.ActiveSessions.Value() != 5 {
		t.Errorf("expected 5, got %d", m.ActiveSessions.Value())
	}
}

// ------------------------------------------------------------------
// MetricsHandler tests
// ------------------------------------------------------------------

func TestMetricsHandler(t *testing.T) {
	r := NewMetricsRegistry()
	c := r.GetCounter("test_requests_total", "Total requests")
	c.Add(42)
	g := r.GetGauge("test_active", "Active connections")
	g.Set(5)
	h := r.GetHistogram("test_latency_seconds", "Request latency", []float64{0.1, 0.5, 1.0})
	h.Observe(0.3)
	h.Observe(0.8)

	handler := MetricsHandler(r)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "test_requests_total 42") {
		t.Error("expected counter in output")
	}
	if !strings.Contains(body, "test_active 5") {
		t.Error("expected gauge in output")
	}
	if !strings.Contains(body, "test_latency_seconds_count 2") {
		t.Error("expected histogram count in output")
	}
	if !strings.Contains(body, "# TYPE test_requests_total counter") {
		t.Error("expected counter TYPE annotation")
	}
	if !strings.Contains(body, "# TYPE test_active gauge") {
		t.Error("expected gauge TYPE annotation")
	}
	if !strings.Contains(body, "# TYPE test_latency_seconds histogram") {
		t.Error("expected histogram TYPE annotation")
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/plain; charset=utf-8" {
		t.Errorf("expected text/plain content type, got %s", ct)
	}
}

// ------------------------------------------------------------------
// Tracer / Span tests
// ------------------------------------------------------------------

func TestTracer_StartAndEndSpan(t *testing.T) {
	tracer := NewTracer(100, testLogger())
	ctx := context.Background()

	ctx, span := tracer.StartSpan(ctx, "test-operation", map[string]string{"key": "value"})
	if span == nil {
		t.Fatal("expected non-nil span")
	}
	if span.Name != "test-operation" {
		t.Errorf("expected name 'test-operation', got %s", span.Name)
	}
	if span.TraceID == "" {
		t.Error("expected non-empty trace ID")
	}
	if span.SpanID == "" {
		t.Error("expected non-empty span ID")
	}
	if span.Attributes["key"] != "value" {
		t.Error("expected attribute key=value")
	}
	_ = ctx

	tracer.EndSpan(span, nil)
	if span.Status != "ok" {
		t.Errorf("expected status 'ok', got %s", span.Status)
	}
	if span.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestTracer_EndSpanWithError(t *testing.T) {
	tracer := NewTracer(100, testLogger())
	_, span := tracer.StartSpan(context.Background(), "failing-op", nil)

	tracer.EndSpan(span, errors.New("something went wrong"))
	if span.Status != "error" {
		t.Errorf("expected status 'error', got %s", span.Status)
	}
	if len(span.Events) == 0 {
		t.Fatal("expected error event")
	}
	if span.Events[0].Name != "error" {
		t.Errorf("expected event name 'error', got %s", span.Events[0].Name)
	}
	if span.Events[0].Attributes["message"] != "something went wrong" {
		t.Error("expected error message in event")
	}
}

func TestTracer_ParentChildSpans(t *testing.T) {
	tracer := NewTracer(100, testLogger())
	ctx := context.Background()

	ctx, parent := tracer.StartSpan(ctx, "parent-op", nil)
	_, child := tracer.StartSpan(ctx, "child-op", nil)

	if child.TraceID != parent.TraceID {
		t.Error("child should inherit parent's trace ID")
	}
	if child.ParentID != parent.SpanID {
		t.Error("child's parent ID should be parent's span ID")
	}
}

func TestTracer_QuerySpans(t *testing.T) {
	tracer := NewTracer(100, testLogger())

	_, s1 := tracer.StartSpan(context.Background(), "op-a", nil)
	tracer.EndSpan(s1, nil)

	_, s2 := tracer.StartSpan(context.Background(), "op-b", nil)
	tracer.EndSpan(s2, errors.New("fail"))

	_, s3 := tracer.StartSpan(context.Background(), "op-a", nil)
	tracer.EndSpan(s3, nil)

	// Query by name
	results := tracer.QuerySpans(SpanQueryOptions{Name: "op-a"})
	if len(results) != 2 {
		t.Errorf("expected 2 spans named op-a, got %d", len(results))
	}

	// Query by status
	results = tracer.QuerySpans(SpanQueryOptions{Status: "error"})
	if len(results) != 1 {
		t.Errorf("expected 1 error span, got %d", len(results))
	}

	// Query with limit
	results = tracer.QuerySpans(SpanQueryOptions{Limit: 1})
	if len(results) != 1 {
		t.Errorf("expected 1 span with limit, got %d", len(results))
	}

	// Query by trace ID
	results = tracer.QuerySpans(SpanQueryOptions{TraceID: s1.TraceID})
	if len(results) != 1 {
		t.Errorf("expected 1 span for trace ID, got %d", len(results))
	}
}

func TestTracer_QuerySpans_Since(t *testing.T) {
	tracer := NewTracer(100, testLogger())

	_, s1 := tracer.StartSpan(context.Background(), "old", nil)
	tracer.EndSpan(s1, nil)

	cutoff := time.Now()
	time.Sleep(10 * time.Millisecond)

	_, s2 := tracer.StartSpan(context.Background(), "new", nil)
	tracer.EndSpan(s2, nil)

	results := tracer.QuerySpans(SpanQueryOptions{Since: cutoff})
	if len(results) != 1 {
		t.Errorf("expected 1 span since cutoff, got %d", len(results))
	}
	if results[0].Name != "new" {
		t.Errorf("expected 'new' span, got %s", results[0].Name)
	}
}

func TestTracer_Eviction(t *testing.T) {
	tracer := NewTracer(10, testLogger())

	// Add more spans than maxSpans
	for i := 0; i < 15; i++ {
		_, span := tracer.StartSpan(context.Background(), "op", nil)
		tracer.EndSpan(span, nil)
	}

	// Should have evicted some
	results := tracer.QuerySpans(SpanQueryOptions{})
	if len(results) > 10 {
		t.Errorf("expected <= 10 spans after eviction, got %d", len(results))
	}
}

func TestSpan_AddEvent(t *testing.T) {
	span := &Span{Name: "test"}
	span.AddEvent("checkpoint", map[string]string{"step": "1"})
	span.AddEvent("checkpoint", map[string]string{"step": "2"})

	if len(span.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(span.Events))
	}
	if span.Events[0].Attributes["step"] != "1" {
		t.Error("expected step 1")
	}
	if span.Events[1].Attributes["step"] != "2" {
		t.Error("expected step 2")
	}
}

// ------------------------------------------------------------------
// TaskHistory tests
// ------------------------------------------------------------------

func TestNewTaskHistory(t *testing.T) {
	th := NewTaskHistory(0) // should default
	if th == nil {
		t.Fatal("expected non-nil task history")
	}
	if th.maxSize != 50000 {
		t.Errorf("expected default max size 50000, got %d", th.maxSize)
	}
}

func TestTaskHistory_RecordAndQuery(t *testing.T) {
	th := NewTaskHistory(100)

	th.Record(&TaskRecord{
		ID:      "1",
		UserID:  "user-1",
		AgentID: "agent-1",
		Action:  "llm_call",
	})
	th.Record(&TaskRecord{
		ID:      "2",
		UserID:  "user-2",
		AgentID: "agent-1",
		Action:  "tool_exec",
	})
	th.Record(&TaskRecord{
		ID:      "3",
		UserID:  "user-1",
		AgentID: "agent-2",
		Action:  "llm_call",
	})

	// Query by user
	results := th.Query(TaskQueryOptions{UserID: "user-1"})
	if len(results) != 2 {
		t.Errorf("expected 2 records for user-1, got %d", len(results))
	}

	// Query by agent
	results = th.Query(TaskQueryOptions{AgentID: "agent-1"})
	if len(results) != 2 {
		t.Errorf("expected 2 records for agent-1, got %d", len(results))
	}

	// Query by action
	results = th.Query(TaskQueryOptions{Action: "llm_call"})
	if len(results) != 2 {
		t.Errorf("expected 2 llm_call records, got %d", len(results))
	}

	// Query with limit
	results = th.Query(TaskQueryOptions{Limit: 1})
	if len(results) != 1 {
		t.Errorf("expected 1 record with limit, got %d", len(results))
	}

	// Query all
	results = th.Query(TaskQueryOptions{})
	if len(results) != 3 {
		t.Errorf("expected 3 total records, got %d", len(results))
	}
}

func TestTaskHistory_QueryByTraceID(t *testing.T) {
	th := NewTaskHistory(100)

	th.Record(&TaskRecord{ID: "1", TraceID: "trace-abc", Action: "llm_call"})
	th.Record(&TaskRecord{ID: "2", TraceID: "trace-xyz", Action: "tool_exec"})

	results := th.Query(TaskQueryOptions{TraceID: "trace-abc"})
	if len(results) != 1 {
		t.Errorf("expected 1 record, got %d", len(results))
	}
}

func TestTaskHistory_QuerySince(t *testing.T) {
	th := NewTaskHistory(100)

	th.Record(&TaskRecord{ID: "1", Timestamp: time.Now().Add(-time.Hour)})
	th.Record(&TaskRecord{ID: "2", Timestamp: time.Now()})

	results := th.Query(TaskQueryOptions{Since: time.Now().Add(-30 * time.Minute)})
	if len(results) != 1 {
		t.Errorf("expected 1 recent record, got %d", len(results))
	}
}

func TestTaskHistory_Eviction(t *testing.T) {
	th := NewTaskHistory(10)

	for i := 0; i < 15; i++ {
		th.Record(&TaskRecord{ID: string(rune('a' + i))})
	}

	results := th.Query(TaskQueryOptions{})
	if len(results) > 10 {
		t.Errorf("expected <= 10 records after eviction, got %d", len(results))
	}
}

func TestTaskRecord_Serialization(t *testing.T) {
	rec := TaskRecord{
		ID:       "task-1",
		TraceID:  "trace-1",
		UserID:   "user-1",
		Channel:  "slack",
		AgentID:  "agent-1",
		Action:   "tool_exec",
		Input:    json.RawMessage(`{"command":"ls"}`),
		Output:   json.RawMessage(`{"stdout":"file1\nfile2"}`),
		Duration: 500 * time.Millisecond,
		Metadata: map[string]string{"tool": "shell_exec"},
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded TaskRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != "task-1" {
		t.Errorf("wrong ID: %s", decoded.ID)
	}
	if decoded.Action != "tool_exec" {
		t.Errorf("wrong action: %s", decoded.Action)
	}
	if decoded.Metadata["tool"] != "shell_exec" {
		t.Error("wrong metadata")
	}
}

// ------------------------------------------------------------------
// generateID tests
// ------------------------------------------------------------------

func TestGenerateID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateID()
		if ids[id] {
			t.Fatalf("duplicate ID: %s", id)
		}
		ids[id] = true
	}
}
