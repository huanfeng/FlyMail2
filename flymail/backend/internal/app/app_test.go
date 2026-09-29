package app

import (
	"encoding/json"
	"testing"

	"flymail/internal/sse"
)

// EmitExternal（SkipInApp，24h 重复提醒）必须同样到达实时通道——dto 的
// SkipInApp 注释承诺「只走外发渠道与 SSE」，而 SSE 与桌面 toast 的发布点
// 在 app 层的 broadcast 里；直绑 EmitExternalFunc 会把它俩一起丢掉，
// 持续故障的每日重复提醒就只剩飞书/webhook 能看到。
func TestBroadcastToStreamsFiresHookAndSSE(t *testing.T) {
	a := &App{}
	hub := sse.NewHub()
	sub := hub.Subscribe()
	defer sub.Cancel()

	var hooked []string
	a.emitHook = func(eventType string, accountID uint, messageID uint, title, body string) {
		hooked = append(hooked, eventType)
	}

	a.broadcastToStreams(hub, "sync_failed", 7, 0, "同步异常：a", "连续 5 次失败")

	if len(hooked) != 1 || hooked[0] != "sync_failed" {
		t.Fatalf("emitHook should fire once with the event: %+v", hooked)
	}
	select {
	case payload := <-sub.Events:
		var ev map[string]any
		if err := json.Unmarshal(payload, &ev); err != nil {
			t.Fatalf("SSE payload is not JSON: %v", err)
		}
		if ev["type"] != "notify" || ev["event"] != "sync_failed" || ev["account_id"] != float64(7) {
			t.Fatalf("unexpected SSE event: %s", payload)
		}
	default:
		t.Fatal("SSE subscriber should receive the event")
	}
}
