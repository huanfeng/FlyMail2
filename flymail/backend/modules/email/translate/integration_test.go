package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"flymail/modules/email/message"
)

// 走一遍真实的 HTTP 链路：service → ai.Client → OpenAI 兼容服务 → 回填。
//
// ── 为什么 service_test 的假客户端不够 ─────────────────────────────────────
//
// 那一批把 chatter 整个换掉了，于是**请求怎么拼、响应怎么解**这段全没被跑到：
// 提示词有没有带上目标语言、编号标记经过一趟 JSON 编解码还认不认得出来、
// choices[0].message.content 的路径对不对——这些恰恰是接真实服务商时会踩的地方。
// 把上游换成一个真的 HTTP 服务，这一段就跟着跑了。
func TestTranslateOverRealHTTP(t *testing.T) {
	var (
		mu       sync.Mutex
		prompts  []string
		requests int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests++
		prompts = append(prompts, req.Messages[0].Content)
		mu.Unlock()

		// 像个听话的模型那样回：原样的编号 + 加了前缀的"译文"
		var sb strings.Builder
		user := req.Messages[len(req.Messages)-1].Content
		for _, line := range strings.Split(user, "\n") {
			if !strings.HasPrefix(line, markOpen) {
				continue
			}
			end := strings.Index(line, markClose)
			if end < 0 {
				continue
			}
			sb.WriteString(line[:end+len(markClose)])
			sb.WriteString("[zh]" + line[end+len(markClose):] + "\n")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]string{"role": "assistant", "content": sb.String()},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	d := &message.MessageDetail{}
	d.ID = 42
	d.Subject = "Your order has shipped"
	d.HTMLBody = `<div><h1>Order shipped</h1><p>Hi Alice, your package is on its way.</p>` +
		`<a href="https://track.example.com/abc">Track it</a><img src="cid:logo@x"></div>`

	// ⚠ 这里**不**替换 newClient：要跑到的正是 ai.New 造出来的那条真实链路。
	svc := NewService(
		NewRepository(newDB(t)),
		func(uint) (*message.MessageDetail, error) { return d, nil },
		func(id uint) (*message.Message, error) { return &message.Message{ID: id}, nil },
		func() Settings {
			return Settings{BaseURL: srv.URL, APIKey: "sk-test", Model: "test-model", DefaultTarget: "zh"}
		},
	)

	got, cached, err := svc.Translate(context.Background(), 42, "zh", false)
	if err != nil {
		t.Fatal(err)
	}
	if cached {
		t.Error("首次翻译不该命中缓存")
	}

	if requests == 0 {
		t.Fatal("一次请求都没发出去")
	}
	// 提示词必须带上目标语言：少了它模型只能靠猜，而猜错的表现是
	// "点了翻译，结果翻成了英语"
	if !strings.Contains(prompts[0], "Simplified Chinese") {
		t.Errorf("系统提示词里没有目标语言：%s", prompts[0])
	}

	if got.Subject != "[zh]Your order has shipped" {
		t.Errorf("主题 = %q", got.Subject)
	}
	for _, want := range []string{
		"[zh]Order shipped",
		"[zh]Hi Alice, your package is on its way.",
		"[zh]Track it",
		`href="https://track.example.com/abc"`, // 链接地址一个字符都不能变
		`src="cid:logo@x"`,                     // 内联图引用同上，变了图就裂了
		"<h1>",                                 // 排版结构原样保留
	} {
		if !strings.Contains(got.HTMLBody, want) {
			t.Errorf("译文缺少 %q：%s", want, got.HTMLBody)
		}
	}
	if got.Model != "test-model" {
		t.Errorf("没记下模型名：%q", got.Model)
	}

	// 第二次走缓存，一个请求都不该再发
	before := requests
	if _, cached, err := svc.Translate(context.Background(), 42, "zh", false); err != nil || !cached {
		t.Fatalf("第二次应当命中缓存：cached=%v err=%v", cached, err)
	}
	if requests != before {
		t.Errorf("命中缓存却又发了 %d 次请求", requests-before)
	}
}

// 长正文会拆成多批并发发出去，每批各自编号、各自回填，最后拼成一份完整译文。
func TestTranslateSplitsIntoConcurrentChunks(t *testing.T) {
	var mu sync.Mutex
	var batches int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		batches++
		mu.Unlock()

		var sb strings.Builder
		for id, text := range decodeChunk(req.Messages[len(req.Messages)-1].Content) {
			sb.WriteString(markOpen)
			sb.WriteString(itoa(id))
			sb.WriteString(markClose)
			sb.WriteString("[zh]" + text + "\n")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]string{"content": sb.String()},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	// 40 个段落，足够超过一批的字符预算
	var html strings.Builder
	html.WriteString("<div>")
	for i := 0; i < 40; i++ {
		html.WriteString("<p>This paragraph talks about invoices, shipping and customer support in some detail.</p>")
	}
	html.WriteString("</div>")

	d := &message.MessageDetail{}
	d.ID = 43
	d.HTMLBody = html.String()

	svc := NewService(
		NewRepository(newDB(t)),
		func(uint) (*message.MessageDetail, error) { return d, nil },
		func(id uint) (*message.Message, error) { return &message.Message{ID: id}, nil },
		func() Settings {
			return Settings{BaseURL: srv.URL, Model: "m", DefaultTarget: "zh"}
		},
	)

	got, _, err := svc.Translate(context.Background(), 43, "zh", false)
	if err != nil {
		t.Fatal(err)
	}
	if batches < 2 {
		t.Fatalf("40 段应当拆成多批，只发了 %d 次", batches)
	}
	// 每一段都要翻到：漏掉的段会退回原文，而那是"翻译了个寂寞"最常见的形态
	if n := strings.Count(got.HTMLBody, "[zh]"); n != 40 {
		t.Errorf("翻到的段数 = %d，想要 40", n)
	}
}

// 上游把错误塞进 200 响应体是常见的（网关、本地推理服务），不能当成空译文。
func TestTranslateSurfacesUpstreamErrorIn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"message":"model not loaded"}}`))
	}))
	defer srv.Close()

	d := &message.MessageDetail{}
	d.ID = 44
	d.HTMLBody = "<p>Hello there, this is a test message.</p>"

	svc := NewService(
		NewRepository(newDB(t)),
		func(uint) (*message.MessageDetail, error) { return d, nil },
		func(id uint) (*message.Message, error) { return &message.Message{ID: id}, nil },
		func() Settings { return Settings{BaseURL: srv.URL, Model: "m", DefaultTarget: "zh"} },
	)

	_, _, err := svc.Translate(context.Background(), 44, "zh", false)
	if err == nil {
		t.Fatal("上游报错却当成了翻译成功")
	}
	if !strings.Contains(err.Error(), "model not loaded") {
		t.Errorf("没把上游的原话带回来：%v", err)
	}
	var n int64
	svc.repo.db.Model(&Translation{}).Count(&n)
	if n != 0 {
		t.Errorf("失败却写了 %d 行缓存", n)
	}
}

// 连不上上游时，报出来的必须是"连不上"，而不是一句"换个模型试试"。
//
// ── 缘起（真机验证） ───────────────────────────────────────────────────────
//
// 把接口地址指向一个没人监听的端口，用户看到的原本是
// "AI 没有返回可用的译文，请稍后重试或更换模型"。照着这句话换十个模型也没用，
// 而真正的原因（端口拒绝连接）一个字都没露出来。
//
// 这条路径与"上游 200 但内容不可用"走的是同一个出口，区别只在有没有留住
// 那个被降级成日志的错误——正因为只差这一点，它很容易在重构中被抹掉。
func TestTranslateReportsConnectionFailure(t *testing.T) {
	// 起一个服务再立刻关掉，拿到一个确定没人监听的地址
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	dead := srv.URL
	srv.Close()

	d := &message.MessageDetail{}
	d.ID = 45
	d.HTMLBody = "<p>Hello there, this is a test message.</p>"

	svc := NewService(
		NewRepository(newDB(t)),
		func(uint) (*message.MessageDetail, error) { return d, nil },
		func(id uint) (*message.Message, error) { return &message.Message{ID: id}, nil },
		func() Settings { return Settings{BaseURL: dead, Model: "m", DefaultTarget: "zh"} },
	)

	_, _, err := svc.Translate(context.Background(), 45, "zh", false)
	if err == nil {
		t.Fatal("连不上上游却当成了翻译成功")
	}
	msg := err.Error()
	if strings.Contains(msg, "更换模型") {
		t.Errorf("把连接失败说成了模型问题：%s", msg)
	}
	if !strings.Contains(msg, "connect") && !strings.Contains(msg, "refused") && !strings.Contains(msg, "dial") {
		t.Errorf("没把真正的原因带出来：%s", msg)
	}
}
