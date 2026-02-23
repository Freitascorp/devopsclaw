package resilience

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 3,
		ResetTimeout: 100 * time.Millisecond,
	})

	// 3 failures should open the circuit
	for i := 0; i < 3; i++ {
		cb.Execute(func() error { return fmt.Errorf("fail") })
	}

	if cb.State() != CircuitOpen {
		t.Errorf("expected open, got %s", cb.State())
	}

	// Should reject calls while open
	err := cb.Execute(func() error { return nil })
	if err == nil {
		t.Error("expected error when circuit is open")
	}
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:         "test",
		MaxFailures:  2,
		ResetTimeout: 50 * time.Millisecond,
	})

	cb.Execute(func() error { return fmt.Errorf("fail") })
	cb.Execute(func() error { return fmt.Errorf("fail") })

	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}

	time.Sleep(60 * time.Millisecond)

	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected half-open, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:         "test",
		MaxFailures:  1,
		ResetTimeout: 50 * time.Millisecond,
	})

	cb.Execute(func() error { return fmt.Errorf("fail") })
	time.Sleep(60 * time.Millisecond)

	// Half-open: one success should close it
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed, got %s", cb.State())
	}
}

func TestRetry_Success(t *testing.T) {
	var attempts int
	err := Retry(context.Background(), RetryConfig{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
	}, func(attempt int) error {
		attempts++
		if attempt < 2 {
			return fmt.Errorf("not yet")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_MaxExceeded(t *testing.T) {
	err := Retry(context.Background(), RetryConfig{
		MaxAttempts:  2,
		InitialDelay: time.Millisecond,
	}, func(attempt int) error {
		return fmt.Errorf("always fails")
	})

	if err == nil {
		t.Error("expected error on max retries exceeded")
	}
}

func TestRetry_NonRetriable(t *testing.T) {
	permanentErr := errors.New("permanent")
	var attempts int
	err := Retry(context.Background(), RetryConfig{
		MaxAttempts:  5,
		InitialDelay: time.Millisecond,
		RetryableErr: func(err error) bool { return !errors.Is(err, permanentErr) },
	}, func(attempt int) error {
		attempts++
		return permanentErr
	})

	if attempts != 1 {
		t.Errorf("expected 1 attempt for non-retriable, got %d", attempts)
	}
	if !errors.Is(err, permanentErr) {
		t.Errorf("expected permanent error, got %v", err)
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(10, 5)

	// Should allow burst of 5
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// Should deny after burst exhausted
	if rl.Allow() {
		t.Error("request should be denied after burst")
	}
}

func TestBulkhead_ConcurrencyLimit(t *testing.T) {
	bh := NewBulkhead("test", 2)
	var active atomic.Int64
	var maxActive atomic.Int64

	ctx := context.Background()
	done := make(chan struct{}, 5)

	for i := 0; i < 5; i++ {
		go func() {
			bh.Execute(ctx, func() error {
				cur := active.Add(1)
				if cur > maxActive.Load() {
					maxActive.Store(cur)
				}
				time.Sleep(50 * time.Millisecond)
				active.Add(-1)
				return nil
			})
			done <- struct{}{}
		}()
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	if maxActive.Load() > 2 {
		t.Errorf("max active %d exceeded bulkhead limit 2", maxActive.Load())
	}
}

func TestBulkhead_TryExecute_Reject(t *testing.T) {
	bh := NewBulkhead("test", 1)

	started := make(chan struct{})
	release := make(chan struct{})

	// Fill the bulkhead
	go bh.Execute(context.Background(), func() error {
		close(started)
		<-release
		return nil
	})

	<-started

	// Should reject immediately
	err := bh.TryExecute(func() error { return nil })
	if err == nil {
		t.Error("expected rejection when bulkhead is full")
	}

	close(release)
}

func TestIdempotencyController(t *testing.T) {
	ic := NewIdempotencyController(time.Second, slog.Default())
	callCount := 0

	fn := func() (any, error) {
		callCount++
		return "result", nil
	}

	// First call
	r1, _ := ic.Execute("key-1", fn)
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Second call with same key should be cached
	r2, _ := ic.Execute("key-1", fn)
	if callCount != 1 {
		t.Errorf("expected still 1 call (cached), got %d", callCount)
	}
	if r1 != r2 {
		t.Error("cached result should match")
	}

	// Different key should execute
	ic.Execute("key-2", fn)
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestWithTimeout(t *testing.T) {
	err := WithTimeout(context.Background(), 50*time.Millisecond, func(ctx context.Context) error {
		time.Sleep(200 * time.Millisecond)
		return nil
	})

	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestPipeline_Composed(t *testing.T) {
	logger := slog.Default()
	cb := NewCircuitBreaker(CircuitBreakerConfig{Name: "test", MaxFailures: 5})
	rl := NewRateLimiter(100, 10)
	bh := NewBulkhead("test", 5)

	pipeline := NewPipeline(logger,
		WithCircuitBreaker(cb),
		WithRateLimit(rl),
		WithBulkhead(bh),
		WithRetry(RetryConfig{MaxAttempts: 2, InitialDelay: time.Millisecond}),
		WithPipelineTimeout(time.Second),
	)

	err := pipeline.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if err != nil {
		t.Errorf("pipeline should succeed: %v", err)
	}
}
