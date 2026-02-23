package bus

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewMessageBus(t *testing.T) {
	mb := NewMessageBus()
	if mb == nil {
		t.Fatal("expected non-nil MessageBus")
	}
	if mb.closed {
		t.Fatal("expected new bus to not be closed")
	}
	if len(mb.handlers) != 0 {
		t.Fatal("expected empty handlers map")
	}
}

func TestPublishAndConsumeInbound(t *testing.T) {
	mb := NewMessageBus()
	defer mb.Close()

	msg := InboundMessage{
		Channel:    "slack",
		SenderID:   "user-1",
		ChatID:     "chat-1",
		Content:    "hello",
		SessionKey: "sess-1",
	}

	mb.PublishInbound(msg)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, ok := mb.ConsumeInbound(ctx)
	if !ok {
		t.Fatal("expected to consume message")
	}
	if got.Channel != "slack" {
		t.Errorf("expected channel slack, got %s", got.Channel)
	}
	if got.Content != "hello" {
		t.Errorf("expected content hello, got %s", got.Content)
	}
	if got.SenderID != "user-1" {
		t.Errorf("expected sender user-1, got %s", got.SenderID)
	}
}

func TestConsumeInbound_ContextCancelled(t *testing.T) {
	mb := NewMessageBus()
	defer mb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, ok := mb.ConsumeInbound(ctx)
	if ok {
		t.Fatal("expected consume to fail on cancelled context")
	}
}

func TestPublishAndSubscribeOutbound(t *testing.T) {
	mb := NewMessageBus()
	defer mb.Close()

	msg := OutboundMessage{
		Channel: "telegram",
		ChatID:  "chat-42",
		Content: "deploy complete",
	}

	mb.PublishOutbound(msg)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, ok := mb.SubscribeOutbound(ctx)
	if !ok {
		t.Fatal("expected to receive outbound message")
	}
	if got.Channel != "telegram" {
		t.Errorf("expected channel telegram, got %s", got.Channel)
	}
	if got.Content != "deploy complete" {
		t.Errorf("expected content 'deploy complete', got %s", got.Content)
	}
}

func TestSubscribeOutbound_ContextCancelled(t *testing.T) {
	mb := NewMessageBus()
	defer mb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, ok := mb.SubscribeOutbound(ctx)
	if ok {
		t.Fatal("expected subscribe to fail on cancelled context")
	}
}

func TestRegisterAndGetHandler(t *testing.T) {
	mb := NewMessageBus()
	defer mb.Close()

	called := false
	handler := func(msg InboundMessage) error {
		called = true
		return nil
	}

	mb.RegisterHandler("slack", handler)

	got, ok := mb.GetHandler("slack")
	if !ok {
		t.Fatal("expected to find handler for slack")
	}
	if got == nil {
		t.Fatal("expected non-nil handler")
	}

	// Verify handler executes
	_ = got(InboundMessage{})
	if !called {
		t.Fatal("handler was not invoked")
	}
}

func TestGetHandler_NotFound(t *testing.T) {
	mb := NewMessageBus()
	defer mb.Close()

	_, ok := mb.GetHandler("nonexistent")
	if ok {
		t.Fatal("expected handler not found")
	}
}

func TestClose(t *testing.T) {
	mb := NewMessageBus()
	mb.Close()

	if !mb.closed {
		t.Fatal("expected bus to be closed")
	}
}

func TestClose_Idempotent(t *testing.T) {
	mb := NewMessageBus()
	mb.Close()
	mb.Close() // should not panic
}

func TestPublishInbound_AfterClose(t *testing.T) {
	mb := NewMessageBus()
	mb.Close()
	// Should not panic — silently drops message
	mb.PublishInbound(InboundMessage{Content: "dropped"})
}

func TestPublishOutbound_AfterClose(t *testing.T) {
	mb := NewMessageBus()
	mb.Close()
	// Should not panic — silently drops message
	mb.PublishOutbound(OutboundMessage{Content: "dropped"})
}

func TestConcurrentPublishConsume(t *testing.T) {
	mb := NewMessageBus()
	defer mb.Close()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n * 2) // n producers + n consumers

	// Producers
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			mb.PublishInbound(InboundMessage{
				Content: "msg",
				ChatID:  "chat",
			})
		}(i)
	}

	// Consumers
	consumed := make(chan struct{}, n)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, ok := mb.ConsumeInbound(ctx)
			if ok {
				consumed <- struct{}{}
			}
		}()
	}

	wg.Wait()
	close(consumed)

	count := 0
	for range consumed {
		count++
	}
	if count != n {
		t.Errorf("expected %d consumed messages, got %d", n, count)
	}
}

func TestInboundMessageMetadata(t *testing.T) {
	msg := InboundMessage{
		Channel:    "discord",
		SenderID:   "user-42",
		ChatID:     "general",
		Content:    "test",
		Media:      []string{"image.png"},
		SessionKey: "session-abc",
		Metadata:   map[string]string{"guild": "12345"},
	}

	if msg.Channel != "discord" {
		t.Error("wrong channel")
	}
	if len(msg.Media) != 1 || msg.Media[0] != "image.png" {
		t.Error("wrong media")
	}
	if msg.Metadata["guild"] != "12345" {
		t.Error("wrong metadata")
	}
}

func TestMultipleHandlers(t *testing.T) {
	mb := NewMessageBus()
	defer mb.Close()

	channels := []string{"slack", "discord", "telegram"}
	for _, ch := range channels {
		mb.RegisterHandler(ch, func(msg InboundMessage) error { return nil })
	}

	for _, ch := range channels {
		_, ok := mb.GetHandler(ch)
		if !ok {
			t.Errorf("expected handler for %s", ch)
		}
	}
}
