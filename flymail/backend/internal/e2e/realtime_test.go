package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

type sseEvent struct {
	Type      string `json:"type"`
	AccountID uint   `json:"account_id"`
	FolderID  uint   `json:"folder_id"`
	NewCount  int    `json:"new_count"`
}

// startSSEReader 订阅 /api/v1/events，后台解析 data: 行推入 channel。
// 先用 Bearer 凭据换一张一次性连接票据：EventSource 设不了请求头，SSE 的 URL 鉴权
// 只认这张 60 秒作废、用一次即销的票（见 internal/sse/ticket.go）。
func startSSEReader(t *testing.T, c *apiClient) <-chan sseEvent {
	t.Helper()
	var tk struct {
		Ticket string `json:"ticket"`
	}
	c.mustJSON(http.MethodPost, "/api/v1/events/ticket", nil, http.StatusOK, &tk)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/events?ticket="+tk.Ticket, nil)
	if err != nil {
		cancel()
		t.Fatalf("SSE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("SSE connect: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("SSE status=%d", resp.StatusCode)
	}
	events := make(chan sseEvent, 32)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue // 忽略 ": ping" 心跳与空行
			}
			var ev sseEvent
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev) == nil {
				select {
				case events <- ev:
				default: // 测试断言只关心是否出现，缓冲满丢弃无妨
				}
			}
		}
	}()
	t.Cleanup(cancel)
	return events
}

// TestRealtime_IDLESSE IDLE/SSE 链路：
// 建账户(enabled) → 订阅 SSE → startBackground(Manager.Start 立即 reconcile，
// 避免等 30s tick) → 初始 pollAll 同步 baseline → IDLE 中投递新邮件 → SSE 收到 new_mail。
// 探针结论 GreenMail SupportsIDLE=true；若推送时序不稳，事件也可能来自轮询路径，
// 断言不区分来源（链路等价），timeout 放宽 60s。
func TestRealtime_IDLESSE(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	mb := uniqueMailbox(t)
	acctID := c.createAccount(mb)
	sendSeed(t, "seeder@localhost", mb, "rt-baseline", "rt-body-0")

	events := startSSEReader(t, c)

	ta.startBackground()

	// 等初始同步落库（baseline 经 API 可见），确认 worker 已接管账户
	var inboxID uint
	eventually(t, 60*time.Second, 500*time.Millisecond, "后台首轮同步完成(baseline 可见)", func() bool {
		inbox := findFolder(c.listFolders(acctID), "inbox")
		if inbox == nil {
			return false
		}
		inboxID = inbox.ID
		return len(c.listMessages(inboxID)) == 1
	})

	// 清空初始同步产生的事件，只关注新投递
	for {
		select {
		case <-events:
			continue
		default:
		}
		break
	}

	sendSeed(t, "seeder@localhost", mb, "rt-new-mail", "rt-body-1")

	deadline := time.After(60 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Type == "new_mail" && ev.AccountID == acctID {
				t.Logf("收到 SSE new_mail: %+v", ev)
				// 邮件本体也应已入库
				eventually(t, 15*time.Second, 300*time.Millisecond, "新邮件入库", func() bool {
					return len(c.listMessages(inboxID)) == 2
				})
				return
			}
		case <-deadline:
			t.Fatal("60s 未收到 new_mail SSE 事件")
		}
	}
}
