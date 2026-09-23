package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEndpoint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://api.openai.com", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/v1/", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/chat/completions"},
		// 版本段不都叫 v1：智谱是 v4，补全时不能硬写 /v1
		{"https://open.bigmodel.cn/api/paas/v4", "https://open.bigmodel.cn/api/paas/v4/chat/completions"},
		{"http://127.0.0.1:11434/v1", "http://127.0.0.1:11434/v1/chat/completions"},
		// 自建网关的非常规路径：认不出版本段就整段补全，用户会立刻看到 404
		{"https://gw.example.com/openai", "https://gw.example.com/openai/v1/chat/completions"},
	}
	for _, c := range cases {
		got, err := Endpoint(c.in)
		if err != nil {
			t.Fatalf("Endpoint(%q) 报错：%v", c.in, err)
		}
		if got != c.want {
			t.Errorf("Endpoint(%q) = %q，想要 %q", c.in, got, c.want)
		}
	}
}

func TestEndpointRejectsBad(t *testing.T) {
	if _, err := Endpoint(""); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("空地址应返回 ErrNotConfigured，得到 %v", err)
	}
	// 少写 scheme 是最常见的手误，不能当相对路径悄悄拼上去
	if _, err := Endpoint("api.openai.com/v1"); err == nil {
		t.Error("缺少 scheme 的地址应当被拒绝")
	}
}

func TestNewRequiresBaseAndModel(t *testing.T) {
	if _, err := New(Config{Model: "gpt-4o-mini"}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("缺地址应返回 ErrNotConfigured，得到 %v", err)
	}
	if _, err := New(Config{BaseURL: "https://x/v1"}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("缺模型应返回 ErrNotConfigured，得到 %v", err)
	}
}

func TestChatOK(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"你好"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, APIKey: "sk-test", Model: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if out != "你好" {
		t.Errorf("内容 = %q", out)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	// 不发 temperature / max_tokens 是刻意的（见包注释），回归时要拦住
	if strings.Contains(gotBody, "temperature") || strings.Contains(gotBody, "max_tokens") {
		t.Errorf("请求体不应带 temperature/max_tokens：%s", gotBody)
	}
}

func TestChatOmitsEmptyAuth(t *testing.T) {
	// 本地模型不要密钥：发一个空的 "Bearer " 会被某些服务判成非法凭据
	var hasAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasAuth = r.Header["Authorization"]
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c, _ := New(Config{BaseURL: srv.URL, Model: "m1"})
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatal(err)
	}
	if hasAuth {
		t.Error("密钥为空时不应发 Authorization 头")
	}
}

func TestChatAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided"}}`))
	}))
	defer srv.Close()

	c, _ := New(Config{BaseURL: srv.URL, APIKey: "bad", Model: "m1"})
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("应当是 *APIError，得到 %v", err)
	}
	if apiErr.Status != http.StatusUnauthorized {
		t.Errorf("Status = %d", apiErr.Status)
	}
	if !strings.Contains(apiErr.Error(), "Incorrect API key") {
		t.Errorf("错误消息应带上游原话：%s", apiErr.Error())
	}
	if apiErr.Retryable() {
		t.Error("401 不该被判为可重试")
	}
}

func TestChatErrorInside200(t *testing.T) {
	// 有些网关把错误塞进 200 响应体，不认就会当成"AI 返回了空译文"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"message":"model not loaded"}}`))
	}))
	defer srv.Close()

	c, _ := New(Config{BaseURL: srv.URL, Model: "m1"})
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("200 里带 error 也必须报错")
	}
}

func TestChatTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"前半篇"},"finish_reason":"length"}]}`))
	}))
	defer srv.Close()

	c, _ := New(Config{BaseURL: srv.URL, Model: "m1"})
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("截断必须报错（否则用户只看到前半篇却以为翻完了），得到 %v", err)
	}
}

func TestChatRetryableOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`<html>502 Bad Gateway</html>`))
	}))
	defer srv.Close()

	c, _ := New(Config{BaseURL: srv.URL, Model: "m1"})
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !apiErr.Retryable() {
		t.Fatalf("502 应当是可重试的 APIError，得到 %v", err)
	}
}
