package sse

import (
	"testing"
	"time"
)

func TestHubPublishToSubscriber(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe()
	defer cancel()

	h.Publish([]byte(`{"type":"new_mail"}`))

	select {
	case msg := <-ch:
		if string(msg) != `{"type":"new_mail"}` {
			t.Fatalf("unexpected payload: %s", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive event")
	}
}

func TestHubUnsubscribeStopsDelivery(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe()
	cancel()
	h.Publish([]byte("x"))
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected no live value after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("read after cancel should not block")
	}
}

func TestHubPublishNonBlockingWhenBufferFull(t *testing.T) {
	h := NewHub()
	_, cancel := h.Subscribe() // 不消费
	defer cancel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			h.Publish([]byte("x"))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on full subscriber buffer")
	}
}
