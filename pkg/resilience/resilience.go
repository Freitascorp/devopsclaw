// Package resilience provides production-grade reliability primitives:
// circuit breakers, retry with exponential backoff, rate limiting,
// bulkheads, and idempotency controls.
//
// These are the "boring features" that make production systems survive
// at 3am when your pager goes off.
package resilience

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// ------------------------------------------------------------------
// Circuit Breaker
// ------------------------------------------------------------------

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // normal operation
	CircuitOpen                         // failing, reject requests
	CircuitHalfOpen                     // testing recovery
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig configures a circuit breaker.
type CircuitBreakerConfig struct {
	Name             string        // identifier for logging
	MaxFailures      int           // failures before opening (default: 5)
	ResetTimeout     time.Duration // time to wait before half-open (default: 30s)
	HalfOpenMaxCalls int           // max calls in half-open state (default: 1)
	OnStateChange    func(name string, from, to CircuitState)
}

// CircuitBreaker prevents cascading failures by stopping calls to failing services.
type CircuitBreaker struct {
	config   CircuitBreakerConfig
	mu       sync.Mutex
	state    CircuitState
	failures int
	lastFail time.Time
	halfOpenCalls int
}

// NewCircuitBreaker creates a circuit breaker with the given config.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.MaxFailures <= 0 {
		config.MaxFailures = 5
	}
	if config.ResetTimeout <= 0 {
		config.ResetTimeout = 30 * time.Second
	}
	if config.HalfOpenMaxCalls <= 0 {
		config.HalfOpenMaxCalls = 1
	}
	return &CircuitBreaker{config: config, state: CircuitClosed}
}

// Execute runs the function through the circuit breaker.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.beforeCall(); err != nil {
		return err
	}
	err := fn()
	cb.afterCall(err)
	return err
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	// Check if open circuit should transition to half-open
	if cb.state == CircuitOpen && time.Since(cb.lastFail) > cb.config.ResetTimeout {
		cb.transition(CircuitHalfOpen)
	}
	return cb.state
}

func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return nil
	case CircuitOpen:
		if time.Since(cb.lastFail) > cb.config.ResetTimeout {
			cb.transition(CircuitHalfOpen)
			cb.halfOpenCalls = 1
			return nil
		}
		return fmt.Errorf("circuit breaker %s is open", cb.config.Name)
	case CircuitHalfOpen:
		if cb.halfOpenCalls >= cb.config.HalfOpenMaxCalls {
			return fmt.Errorf("circuit breaker %s is half-open (max test calls reached)", cb.config.Name)
		}
		cb.halfOpenCalls++
		return nil
	}
	return nil
}

func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFail = time.Now()
		if cb.state == CircuitHalfOpen || cb.failures >= cb.config.MaxFailures {
			cb.transition(CircuitOpen)
		}
	} else {
		if cb.state == CircuitHalfOpen {
			cb.transition(CircuitClosed)
		}
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) transition(to CircuitState) {
	from := cb.state
	cb.state = to
	cb.halfOpenCalls = 0
	if from != to && cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.config.Name, from, to)
	}
}

// ------------------------------------------------------------------
// Retry with exponential backoff
// ------------------------------------------------------------------

// RetryConfig configures retry behavior.
type RetryConfig struct {
	MaxAttempts  int           // max retry attempts (default: 3)
	InitialDelay time.Duration // first retry delay (default: 100ms)
	MaxDelay     time.Duration // cap on delay (default: 30s)
	Multiplier   float64       // backoff multiplier (default: 2.0)
	JitterFrac   float64       // jitter fraction 0-1 (default: 0.1)
	RetryableErr func(error) bool // returns true if error is retriable
}

// DefaultRetryConfig returns a sensible default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		JitterFrac:   0.1,
		RetryableErr: func(err error) bool { return true }, // retry everything
	}
}

// Retry executes a function with exponential backoff retry.
func Retry(ctx context.Context, config RetryConfig, fn func(attempt int) error) error {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 30 * time.Second
	}
	if config.Multiplier <= 0 {
		config.Multiplier = 2.0
	}

	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		lastErr = fn(attempt)
		if lastErr == nil {
			return nil
		}

		// Check if error is retriable
		if config.RetryableErr != nil && !config.RetryableErr(lastErr) {
			return lastErr
		}

		// Don't sleep after last attempt
		if attempt < config.MaxAttempts-1 {
			jitter := time.Duration(float64(delay) * config.JitterFrac * (rand.Float64()*2 - 1))
			sleepDur := delay + jitter
			if sleepDur > config.MaxDelay {
				sleepDur = config.MaxDelay
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleepDur):
			}

			delay = time.Duration(float64(delay) * config.Multiplier)
		}
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", config.MaxAttempts, lastErr)
}

// ------------------------------------------------------------------
// Rate Limiter (token bucket)
// ------------------------------------------------------------------

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	rate     float64   // tokens per second
	burst    int       // max tokens
	tokens   float64
	lastTime time.Time
}

// NewRateLimiter creates a rate limiter.
// rate: requests per second, burst: max burst size.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		rate:     rate,
		burst:    burst,
		tokens:   float64(burst),
		lastTime: time.Now(),
	}
}

// Allow checks if a request is allowed under the rate limit.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTime).Seconds()
	rl.lastTime = now

	rl.tokens += elapsed * rl.rate
	if rl.tokens > float64(rl.burst) {
		rl.tokens = float64(rl.burst)
	}

	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

// Wait blocks until a token is available or ctx is cancelled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.Allow() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1.0/rl.rate*1000) * time.Millisecond):
		}
	}
}

// ------------------------------------------------------------------
// Rate Limiter Registry (per-user, per-provider)
// ------------------------------------------------------------------

// RateLimiterRegistry manages per-key rate limiters.
type RateLimiterRegistry struct {
	mu       sync.RWMutex
	limiters map[string]*RateLimiter
	defaultRate  float64
	defaultBurst int
}

// NewRateLimiterRegistry creates a rate limiter registry.
func NewRateLimiterRegistry(defaultRate float64, defaultBurst int) *RateLimiterRegistry {
	return &RateLimiterRegistry{
		limiters:     make(map[string]*RateLimiter),
		defaultRate:  defaultRate,
		defaultBurst: defaultBurst,
	}
}

// Get returns (or creates) a rate limiter for the given key.
func (r *RateLimiterRegistry) Get(key string) *RateLimiter {
	r.mu.RLock()
	rl, ok := r.limiters[key]
	r.mu.RUnlock()
	if ok {
		return rl
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// Double-check
	if rl, ok = r.limiters[key]; ok {
		return rl
	}
	rl = NewRateLimiter(r.defaultRate, r.defaultBurst)
	r.limiters[key] = rl
	return rl
}

// ------------------------------------------------------------------
// Bulkhead (concurrency limiter)
// ------------------------------------------------------------------

// Bulkhead limits concurrent executions to prevent resource exhaustion.
type Bulkhead struct {
	name     string
	sem      chan struct{}
	active   atomic.Int64
	rejected atomic.Int64
}

// NewBulkhead creates a bulkhead with the given concurrency limit.
func NewBulkhead(name string, maxConcurrent int) *Bulkhead {
	return &Bulkhead{
		name: name,
		sem:  make(chan struct{}, maxConcurrent),
	}
}

// Execute runs the function within the bulkhead's concurrency limit.
func (b *Bulkhead) Execute(ctx context.Context, fn func() error) error {
	select {
	case b.sem <- struct{}{}:
		b.active.Add(1)
		defer func() {
			<-b.sem
			b.active.Add(-1)
		}()
		return fn()
	case <-ctx.Done():
		b.rejected.Add(1)
		return fmt.Errorf("bulkhead %s: context cancelled while waiting", b.name)
	}
}

// TryExecute runs the function if capacity is available, otherwise returns error immediately.
func (b *Bulkhead) TryExecute(fn func() error) error {
	select {
	case b.sem <- struct{}{}:
		b.active.Add(1)
		defer func() {
			<-b.sem
			b.active.Add(-1)
		}()
		return fn()
	default:
		b.rejected.Add(1)
		return fmt.Errorf("bulkhead %s: no capacity available (%d active)", b.name, b.active.Load())
	}
}

// Stats returns bulkhead usage statistics.
func (b *Bulkhead) Stats() BulkheadStats {
	return BulkheadStats{
		Name:     b.name,
		Active:   int(b.active.Load()),
		Capacity: cap(b.sem),
		Rejected: int(b.rejected.Load()),
	}
}

// BulkheadStats reports bulkhead utilization.
type BulkheadStats struct {
	Name     string `json:"name"`
	Active   int    `json:"active"`
	Capacity int    `json:"capacity"`
	Rejected int    `json:"rejected"`
}

// ------------------------------------------------------------------
// Idempotency Controller
// ------------------------------------------------------------------

// IdempotencyController prevents duplicate execution of the same command.
type IdempotencyController struct {
	mu      sync.RWMutex
	seen    map[string]*idempotencyEntry
	ttl     time.Duration
	logger  *slog.Logger
}

type idempotencyEntry struct {
	result   any
	err      error
	created  time.Time
}

// NewIdempotencyController creates an idempotency controller.
func NewIdempotencyController(ttl time.Duration, logger *slog.Logger) *IdempotencyController {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	ic := &IdempotencyController{
		seen:   make(map[string]*idempotencyEntry),
		ttl:    ttl,
		logger: logger,
	}
	return ic
}

// Execute runs fn only if the key hasn't been seen recently.
// Returns the cached result if the key was already processed.
func (ic *IdempotencyController) Execute(key string, fn func() (any, error)) (any, error) {
	ic.mu.RLock()
	entry, ok := ic.seen[key]
	ic.mu.RUnlock()

	if ok && time.Since(entry.created) < ic.ttl {
		ic.logger.Debug("idempotency hit, returning cached result", "key", key)
		return entry.result, entry.err
	}

	result, err := fn()

	ic.mu.Lock()
	ic.seen[key] = &idempotencyEntry{
		result:  result,
		err:     err,
		created: time.Now(),
	}
	ic.mu.Unlock()

	return result, err
}

// Cleanup removes expired entries. Should be called periodically.
func (ic *IdempotencyController) Cleanup() {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	now := time.Now()
	for key, entry := range ic.seen {
		if now.Sub(entry.created) > ic.ttl {
			delete(ic.seen, key)
		}
	}
}

// RunCleanup starts a background cleanup goroutine.
func (ic *IdempotencyController) RunCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ic.Cleanup()
		}
	}
}

// ------------------------------------------------------------------
// Timeout wrapper
// ------------------------------------------------------------------

// WithTimeout runs fn with a timeout, returning error if deadline exceeded.
func WithTimeout(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- fn(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("operation timed out after %s", timeout)
	}
}

// ------------------------------------------------------------------
// Composed resilience pipeline
// ------------------------------------------------------------------

// Pipeline composes multiple resilience patterns into a single execution wrapper.
type Pipeline struct {
	circuitBreaker *CircuitBreaker
	rateLimiter    *RateLimiter
	bulkhead       *Bulkhead
	retryConfig    *RetryConfig
	timeout        time.Duration
	idempotency    *IdempotencyController
	logger         *slog.Logger
}

// PipelineOption configures a resilience pipeline.
type PipelineOption func(*Pipeline)

// WithCircuitBreaker adds circuit breaking to the pipeline.
func WithCircuitBreaker(cb *CircuitBreaker) PipelineOption {
	return func(p *Pipeline) { p.circuitBreaker = cb }
}

// WithRateLimit adds rate limiting to the pipeline.
func WithRateLimit(rl *RateLimiter) PipelineOption {
	return func(p *Pipeline) { p.rateLimiter = rl }
}

// WithBulkhead adds concurrency limiting to the pipeline.
func WithBulkhead(bh *Bulkhead) PipelineOption {
	return func(p *Pipeline) { p.bulkhead = bh }
}

// WithRetry adds retry with backoff to the pipeline.
func WithRetry(cfg RetryConfig) PipelineOption {
	return func(p *Pipeline) { p.retryConfig = &cfg }
}

// WithPipelineTimeout adds a timeout to the pipeline.
func WithPipelineTimeout(d time.Duration) PipelineOption {
	return func(p *Pipeline) { p.timeout = d }
}

// WithIdempotency adds idempotency control to the pipeline.
func WithIdempotency(ic *IdempotencyController) PipelineOption {
	return func(p *Pipeline) { p.idempotency = ic }
}

// NewPipeline creates a composed resilience pipeline.
func NewPipeline(logger *slog.Logger, opts ...PipelineOption) *Pipeline {
	p := &Pipeline{logger: logger}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Execute runs fn through the full resilience pipeline:
// rate limit → bulkhead → circuit breaker → retry → timeout → fn
func (p *Pipeline) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	// Rate limit
	if p.rateLimiter != nil {
		if err := p.rateLimiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limited: %w", err)
		}
	}

	// Bulkhead
	if p.bulkhead != nil {
		return p.bulkhead.Execute(ctx, func() error {
			return p.executeInner(ctx, fn)
		})
	}

	return p.executeInner(ctx, fn)
}

func (p *Pipeline) executeInner(ctx context.Context, fn func(ctx context.Context) error) error {
	exec := func() error {
		// Timeout
		if p.timeout > 0 {
			return WithTimeout(ctx, p.timeout, fn)
		}
		return fn(ctx)
	}

	// Circuit breaker
	if p.circuitBreaker != nil {
		exec2 := exec
		exec = func() error {
			return p.circuitBreaker.Execute(exec2)
		}
	}

	// Retry
	if p.retryConfig != nil {
		return Retry(ctx, *p.retryConfig, func(attempt int) error {
			if attempt > 0 {
				p.logger.Debug("retrying", "attempt", attempt)
			}
			return exec()
		})
	}

	return exec()
}

// Unused but keeping for potential future use
var _ = math.MaxFloat64
