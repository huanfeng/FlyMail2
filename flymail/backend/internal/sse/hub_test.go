package sse

import (
	"testing"
	"time"
)

func TestHubPublishToSubscriber(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe()
	defer sub.Cancel()

	h.Publish([]byte(`{"type":"new_mail"}`))

	select {
	case msg := <-sub.Events:
		if string(msg) != `{"type":"new_mail"}` {
			t.Fatalf("unexpected payload: %s", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive event")
	}
}

func TestHubUnsubscribeStopsDelivery(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe()
	sub.Cancel()
	h.Publish([]byte("x"))
	select {
	case _, ok := <-sub.Events:
		if ok {
			t.Fatal("expected no live value after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("read after cancel should not block")
	}
}

func TestHubPublishNonBlockingWhenBufferFull(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe() // 不消费
	defer sub.Cancel()
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

// TestProgressNeverStarvesEvents 是分两条队列的**全部理由**。
//
// 同步进度一轮就有 2n+4 条（12 个文件夹 = 28 条），而缓冲只有十几格。
// 两者共用一条队列时，进度会把 new_mail / notify 整个挤出去——
// 用户看到的是「明明来了新邮件，列表不动，手动刷新才出来」，且无从察觉丢了什么。
func TestProgressNeverStarvesEvents(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe() // 不消费，模拟慢客户端
	defer sub.Cancel()

	// 灌满进度：远超任何缓冲
	for i := 0; i < 200; i++ {
		h.PublishProgress([]byte(`{"type":"sync_status"}`))
	}
	// 此时来一封新邮件
	h.Publish([]byte(`{"type":"new_mail"}`))

	select {
	case msg := <-sub.Events:
		if string(msg) != `{"type":"new_mail"}` {
			t.Fatalf("收到的不是新邮件事件：%s", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("new_mail 被进度事件挤掉了")
	}
}

// TestProgressKeepsLatest 进度队列满时挤掉最旧的，保住最新那帧。
//
// 反过来（丢弃新的、保住旧的）会让慢客户端永远停在一串过期进度上：
// 同步早就结束了，界面还显示「第 3 / 12 个文件夹」。
func TestProgressKeepsLatest(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe()
	defer sub.Cancel()

	for i := 0; i < progressBuffer*3; i++ {
		h.PublishProgress([]byte{byte(i)})
	}

	// 排空，最后一条必须是最新发的那条
	var last []byte
	for {
		select {
		case msg := <-sub.Progress:
			last = msg
			continue
		default:
		}
		break
	}
	if len(last) != 1 || last[0] != byte(progressBuffer*3-1) {
		t.Fatalf("队尾是 %v，want %v——最新一帧没保住", last, byte(progressBuffer*3-1))
	}
}

// TestPublishProgressNonBlocking 慢客户端不能把发布方（runner goroutine）拖住。
func TestPublishProgressNonBlocking(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe() // 不消费
	defer sub.Cancel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			h.PublishProgress([]byte("x"))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("PublishProgress 在满缓冲上阻塞了")
	}
}
