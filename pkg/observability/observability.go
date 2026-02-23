// Package observability provides structured metrics, tracing, and logging
// for production DevOpsClaw deployments. It integrates with Prometheus for
// metrics and supports OpenTelemetry-compatible trace propagation.
//
// This replaces DevOpsClaw's ad-hoc file logging with structured,
// queryable, exportable telemetry.
package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ------------------------------------------------------------------
// Metrics
// ------------------------------------------------------------------

// MetricType classifies a metric.
type MetricType string

const (
	MetricCounter   MetricType = "counter"
	MetricGauge     MetricType = "gauge"
	MetricHistogram MetricType = "histogram"
)

// Metric is a single named metric.
type Metric struct {
	Name        string            `json:"name"`
	Type        MetricType        `json:"type"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// MetricsRegistry collects and exposes application metrics.
type MetricsRegistry struct {
	mu         sync.RWMutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
}

// NewMetricsRegistry creates a metrics registry.
func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
	}
}

// Counter is a monotonically increasing metric.
type Counter struct {
	name  string
	desc  string
	value atomic.Int64
}

// Gauge is a metric that can go up and down.
type Gauge struct {
	name  string
	desc  string
	value atomic.Int64 // stores float64 as int64 bits
}

// Histogram tracks value distributions with pre-defined buckets.
type Histogram struct {
	mu      sync.Mutex
	name    string
	desc    string
	buckets []float64
	counts  []int64
	sum     float64
	count   int64
}

// GetCounter returns (or creates) a counter metric.
func (r *MetricsRegistry) GetCounter(name, description string) *Counter {
	r.mu.RLock()
	c, ok := r.counters[name]
	r.mu.RUnlock()
	if ok {
		return c
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok = r.counters[name]; ok {
		return c
	}
	c = &Counter{name: name, desc: description}
	r.counters[name] = c
	return c
}

// GetGauge returns (or creates) a gauge metric.
func (r *MetricsRegistry) GetGauge(name, description string) *Gauge {
	r.mu.RLock()
	g, ok := r.gauges[name]
	r.mu.RUnlock()
	if ok {
		return g
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok = r.gauges[name]; ok {
		return g
	}
	g = &Gauge{name: name, desc: description}
	r.gauges[name] = g
	return g
}

// GetHistogram returns (or creates) a histogram metric.
func (r *MetricsRegistry) GetHistogram(name, description string, buckets []float64) *Histogram {
	r.mu.RLock()
	h, ok := r.histograms[name]
	r.mu.RUnlock()
	if ok {
		return h
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok = r.histograms[name]; ok {
		return h
	}
	sort.Float64s(buckets)
	h = &Histogram{name: name, desc: description, buckets: buckets, counts: make([]int64, len(buckets)+1)}
	r.histograms[name] = h
	return h
}

// Inc increments a counter by 1.
func (c *Counter) Inc() { c.value.Add(1) }

// Add increments a counter by n.
func (c *Counter) Add(n int64) { c.value.Add(n) }

// Value returns the counter's current value.
func (c *Counter) Value() int64 { return c.value.Load() }

// Set sets the gauge value.
func (g *Gauge) Set(v int64) { g.value.Store(v) }

// Inc increments the gauge by 1.
func (g *Gauge) Inc() { g.value.Add(1) }

// Dec decrements the gauge by 1.
func (g *Gauge) Dec() { g.value.Add(-1) }

// Value returns the gauge's current value.
func (g *Gauge) Value() int64 { return g.value.Load() }

// Observe records a value in the histogram.
func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum += v
	h.count++
	for i, b := range h.buckets {
		if v <= b {
			h.counts[i]++
			return
		}
	}
	h.counts[len(h.buckets)]++ // +Inf bucket
}

// ------------------------------------------------------------------
// Pre-defined DevOpsClaw metrics
// ------------------------------------------------------------------

// DevOpsClawMetrics holds all DevOpsClaw-specific metrics.
type DevOpsClawMetrics struct {
	Registry *MetricsRegistry

	// Agent loop
	MessagesReceived  *Counter
	MessagesProcessed *Counter
	MessageErrors     *Counter
	LLMCalls          *Counter
	LLMErrors         *Counter
	LLMLatency        *Histogram
	ToolCalls         *Counter
	ToolErrors        *Counter
	ToolLatency       *Histogram
	ActiveSessions    *Gauge

	// Fleet
	FleetNodesTotal   *Gauge
	FleetNodesOnline  *Gauge
	FleetExecTotal    *Counter
	FleetExecErrors   *Counter
	FleetExecLatency  *Histogram

	// Channels
	ChannelMessages   *Counter
	ChannelErrors     *Counter

	// Providers
	ProviderCalls     *Counter
	ProviderErrors    *Counter
	ProviderFallbacks *Counter

	// Resilience
	CircuitBreakerTrips  *Counter
	RateLimitRejects     *Counter
	BulkheadRejects      *Counter
	RetryAttempts        *Counter

	// System
	Uptime            *Gauge
	GoroutineCount    *Gauge
}

// NewDevOpsClawMetrics creates the standard DevOpsClaw metrics suite.
func NewDevOpsClawMetrics() *DevOpsClawMetrics {
	r := NewMetricsRegistry()

	latencyBuckets := []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}

	return &DevOpsClawMetrics{
		Registry: r,

		MessagesReceived:  r.GetCounter("devopsclaw_messages_received_total", "Total messages received"),
		MessagesProcessed: r.GetCounter("devopsclaw_messages_processed_total", "Total messages processed"),
		MessageErrors:     r.GetCounter("devopsclaw_message_errors_total", "Total message processing errors"),
		LLMCalls:          r.GetCounter("devopsclaw_llm_calls_total", "Total LLM API calls"),
		LLMErrors:         r.GetCounter("devopsclaw_llm_errors_total", "Total LLM API errors"),
		LLMLatency:        r.GetHistogram("devopsclaw_llm_latency_seconds", "LLM call latency", latencyBuckets),
		ToolCalls:         r.GetCounter("devopsclaw_tool_calls_total", "Total tool executions"),
		ToolErrors:        r.GetCounter("devopsclaw_tool_errors_total", "Total tool execution errors"),
		ToolLatency:       r.GetHistogram("devopsclaw_tool_latency_seconds", "Tool execution latency", latencyBuckets),
		ActiveSessions:    r.GetGauge("devopsclaw_active_sessions", "Currently active sessions"),

		FleetNodesTotal:   r.GetGauge("devopsclaw_fleet_nodes_total", "Total registered fleet nodes"),
		FleetNodesOnline:  r.GetGauge("devopsclaw_fleet_nodes_online", "Online fleet nodes"),
		FleetExecTotal:    r.GetCounter("devopsclaw_fleet_exec_total", "Total fleet command executions"),
		FleetExecErrors:   r.GetCounter("devopsclaw_fleet_exec_errors_total", "Total fleet execution errors"),
		FleetExecLatency:  r.GetHistogram("devopsclaw_fleet_exec_latency_seconds", "Fleet execution latency", latencyBuckets),

		ChannelMessages:   r.GetCounter("devopsclaw_channel_messages_total", "Total channel messages"),
		ChannelErrors:     r.GetCounter("devopsclaw_channel_errors_total", "Total channel errors"),

		ProviderCalls:     r.GetCounter("devopsclaw_provider_calls_total", "Total provider API calls"),
		ProviderErrors:    r.GetCounter("devopsclaw_provider_errors_total", "Total provider errors"),
		ProviderFallbacks: r.GetCounter("devopsclaw_provider_fallbacks_total", "Total provider fallback events"),

		CircuitBreakerTrips: r.GetCounter("devopsclaw_circuit_breaker_trips_total", "Circuit breaker trip events"),
		RateLimitRejects:    r.GetCounter("devopsclaw_rate_limit_rejects_total", "Rate limit rejections"),
		BulkheadRejects:     r.GetCounter("devopsclaw_bulkhead_rejects_total", "Bulkhead rejections"),
		RetryAttempts:       r.GetCounter("devopsclaw_retry_attempts_total", "Retry attempts"),

		Uptime:         r.GetGauge("devopsclaw_uptime_seconds", "Process uptime in seconds"),
		GoroutineCount: r.GetGauge("devopsclaw_goroutine_count", "Number of goroutines"),
	}
}

// ------------------------------------------------------------------
// Metrics HTTP endpoint (Prometheus-compatible)
// ------------------------------------------------------------------

// MetricsHandler returns an HTTP handler that exports metrics in
// Prometheus exposition format.
func MetricsHandler(registry *MetricsRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		registry.mu.RLock()
		defer registry.mu.RUnlock()

		for _, c := range registry.counters {
			fmt.Fprintf(w, "# HELP %s %s\n", c.name, c.desc)
			fmt.Fprintf(w, "# TYPE %s counter\n", c.name)
			fmt.Fprintf(w, "%s %d\n", c.name, c.value.Load())
		}
		for _, g := range registry.gauges {
			fmt.Fprintf(w, "# HELP %s %s\n", g.name, g.desc)
			fmt.Fprintf(w, "# TYPE %s gauge\n", g.name)
			fmt.Fprintf(w, "%s %d\n", g.name, g.value.Load())
		}
		for _, h := range registry.histograms {
			fmt.Fprintf(w, "# HELP %s %s\n", h.name, h.desc)
			fmt.Fprintf(w, "# TYPE %s histogram\n", h.name)
			h.mu.Lock()
			cumulative := int64(0)
			for i, b := range h.buckets {
				cumulative += h.counts[i]
				fmt.Fprintf(w, "%s_bucket{le=\"%g\"} %d\n", h.name, b, cumulative)
			}
			cumulative += h.counts[len(h.buckets)]
			fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n", h.name, cumulative)
			fmt.Fprintf(w, "%s_sum %g\n", h.name, h.sum)
			fmt.Fprintf(w, "%s_count %d\n", h.name, h.count)
			h.mu.Unlock()
		}
	}
}

// ------------------------------------------------------------------
// Structured tracing
// ------------------------------------------------------------------

// Span represents a unit of work in a trace.
type Span struct {
	TraceID    string            `json:"trace_id"`
	SpanID     string            `json:"span_id"`
	ParentID   string            `json:"parent_id,omitempty"`
	Name       string            `json:"name"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time,omitempty"`
	Duration   time.Duration     `json:"duration,omitempty"`
	Status     string            `json:"status"` // "ok", "error"
	Attributes map[string]string `json:"attributes,omitempty"`
	Events     []SpanEvent       `json:"events,omitempty"`
}

// SpanEvent is a timestamped annotation within a span.
type SpanEvent struct {
	Name       string            `json:"name"`
	Timestamp  time.Time         `json:"timestamp"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Tracer creates and manages spans.
type Tracer struct {
	mu       sync.Mutex
	spans    []*Span
	maxSpans int
	logger   *slog.Logger
}

// NewTracer creates a tracer.
func NewTracer(maxSpans int, logger *slog.Logger) *Tracer {
	if maxSpans <= 0 {
		maxSpans = 10000
	}
	return &Tracer{
		spans:    make([]*Span, 0, maxSpans),
		maxSpans: maxSpans,
		logger:   logger,
	}
}

type traceContextKey struct{}

// StartSpan begins a new span and attaches it to the context.
func (t *Tracer) StartSpan(ctx context.Context, name string, attrs map[string]string) (context.Context, *Span) {
	span := &Span{
		TraceID:    generateID(),
		SpanID:     generateID(),
		Name:       name,
		StartTime:  time.Now(),
		Status:     "ok",
		Attributes: attrs,
	}

	// Inherit trace from parent
	if parent, ok := ctx.Value(traceContextKey{}).(*Span); ok {
		span.TraceID = parent.TraceID
		span.ParentID = parent.SpanID
	}

	return context.WithValue(ctx, traceContextKey{}, span), span
}

// EndSpan completes a span and records it.
func (t *Tracer) EndSpan(span *Span, err error) {
	span.EndTime = time.Now()
	span.Duration = span.EndTime.Sub(span.StartTime)
	if err != nil {
		span.Status = "error"
		span.AddEvent("error", map[string]string{"message": err.Error()})
	}

	t.mu.Lock()
	if len(t.spans) >= t.maxSpans {
		t.spans = t.spans[t.maxSpans/10:]
	}
	t.spans = append(t.spans, span)
	t.mu.Unlock()

	t.logger.Debug("span completed",
		"trace_id", span.TraceID,
		"span_id", span.SpanID,
		"name", span.Name,
		"duration", span.Duration,
		"status", span.Status,
	)
}

// AddEvent adds a timestamped event to a span.
func (s *Span) AddEvent(name string, attrs map[string]string) {
	s.Events = append(s.Events, SpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attrs,
	})
}

// QuerySpans returns recent spans matching the filter.
func (t *Tracer) QuerySpans(opts SpanQueryOptions) []*Span {
	t.mu.Lock()
	defer t.mu.Unlock()

	var out []*Span
	for _, s := range t.spans {
		if opts.TraceID != "" && s.TraceID != opts.TraceID {
			continue
		}
		if opts.Name != "" && s.Name != opts.Name {
			continue
		}
		if !opts.Since.IsZero() && s.StartTime.Before(opts.Since) {
			continue
		}
		if opts.Status != "" && s.Status != opts.Status {
			continue
		}
		out = append(out, s)
		if opts.Limit > 0 && len(out) >= opts.Limit {
			break
		}
	}
	return out
}

// SpanQueryOptions filters trace queries.
type SpanQueryOptions struct {
	TraceID string
	Name    string
	Status  string
	Since   time.Time
	Limit   int
}

// ------------------------------------------------------------------
// Task history (replayable execution log)
// ------------------------------------------------------------------

// TaskRecord is a persistent record of every agent action for replay and debugging.
type TaskRecord struct {
	ID          string                 `json:"id"`
	TraceID     string                 `json:"trace_id"`
	UserID      string                 `json:"user_id"`
	Channel     string                 `json:"channel"`
	AgentID     string                 `json:"agent_id"`
	Action      string                 `json:"action"` // "llm_call", "tool_exec", "fleet_exec", etc.
	Input       json.RawMessage        `json:"input"`
	Output      json.RawMessage        `json:"output"`
	Error       string                 `json:"error,omitempty"`
	Duration    time.Duration          `json:"duration"`
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]string      `json:"metadata,omitempty"`
}

// TaskHistory stores and queries task execution records.
type TaskHistory struct {
	mu      sync.Mutex
	records []*TaskRecord
	maxSize int
}

// NewTaskHistory creates a task history store.
func NewTaskHistory(maxSize int) *TaskHistory {
	if maxSize <= 0 {
		maxSize = 50000
	}
	return &TaskHistory{
		records: make([]*TaskRecord, 0, maxSize),
		maxSize: maxSize,
	}
}

// Record adds a task record.
func (th *TaskHistory) Record(rec *TaskRecord) {
	th.mu.Lock()
	defer th.mu.Unlock()
	if len(th.records) >= th.maxSize {
		th.records = th.records[th.maxSize/10:]
	}
	th.records = append(th.records, rec)
}

// Query returns records matching the filter.
func (th *TaskHistory) Query(opts TaskQueryOptions) []*TaskRecord {
	th.mu.Lock()
	defer th.mu.Unlock()
	var out []*TaskRecord
	for _, r := range th.records {
		if opts.UserID != "" && r.UserID != opts.UserID {
			continue
		}
		if opts.AgentID != "" && r.AgentID != opts.AgentID {
			continue
		}
		if opts.Action != "" && r.Action != opts.Action {
			continue
		}
		if !opts.Since.IsZero() && r.Timestamp.Before(opts.Since) {
			continue
		}
		if opts.TraceID != "" && r.TraceID != opts.TraceID {
			continue
		}
		out = append(out, r)
		if opts.Limit > 0 && len(out) >= opts.Limit {
			break
		}
	}
	return out
}

// TaskQueryOptions filters task history queries.
type TaskQueryOptions struct {
	UserID  string
	AgentID string
	Action  string
	TraceID string
	Since   time.Time
	Limit   int
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

var idCounter atomic.Int64

func generateID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), idCounter.Add(1))
}
