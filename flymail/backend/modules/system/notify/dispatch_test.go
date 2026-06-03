package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendWebhook(t *testing.T) {
	var gotMethod, gotCT, gotSecret string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		gotSecret = r.Header.Get("X-Webhook-Secret")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := &Channel{Kind: "webhook", URL: srv.URL, Secret: "s3cr3t"}
	if err := sendWebhook(ch, Event{Type: EventMailNew, Title: "新邮件", Body: "1 封", AccountID: 7}); err != nil {
		t.Fatalf("sendWebhook: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if !strings.Contains(gotCT, "application/json") {
		t.Errorf("content-type = %q", gotCT)
	}
	if gotSecret != "s3cr3t" {
		t.Errorf("X-Webhook-Secret = %q, want s3cr3t", gotSecret)
	}
	if body["type"] != "mail_new" || body["title"] != "新邮件" || body["body"] != "1 封" {
		t.Errorf("payload 错误: %+v", body)
	}
	if v, _ := body["account_id"].(float64); v != 7 {
		t.Errorf("account_id = %v, want 7", body["account_id"])
	}
}

func TestSendWebhookNoSecretOmitsHeader(t *testing.T) {
	var hasHeader bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasHeader = r.Header["X-Webhook-Secret"]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if err := sendWebhook(&Channel{Kind: "webhook", URL: srv.URL}, Event{Title: "t"}); err != nil {
		t.Fatal(err)
	}
	if hasHeader {
		t.Error("无 secret 时不应带 X-Webhook-Secret 头")
	}
}

func TestSendWebhookNon2xxFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	if err := sendWebhook(&Channel{Kind: "webhook", URL: srv.URL}, Event{Title: "t"}); err == nil {
		t.Error("非 2xx 应返回错误")
	}
}

func TestSendFeishuWithSign(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	const secret = "feishu-secret"
	ch := &Channel{Kind: "feishu", URL: srv.URL, Secret: secret}
	if err := sendFeishu(ch, Event{Title: "标题", Body: "正文"}); err != nil {
		t.Fatalf("sendFeishu: %v", err)
	}
	if body["msg_type"] != "text" {
		t.Errorf("msg_type = %v, want text", body["msg_type"])
	}
	content, _ := body["content"].(map[string]any)
	if content == nil || content["text"] != "标题\n正文" {
		t.Errorf("content 错误: %+v", body["content"])
	}
	ts, _ := body["timestamp"].(string)
	sign, _ := body["sign"].(string)
	if ts == "" || sign == "" {
		t.Fatalf("应包含 timestamp 与 sign: %+v", body)
	}
	// 独立按飞书算法重算签名核对
	key := ts + "\n" + secret
	h := hmac.New(sha256.New, []byte(key))
	want := base64.StdEncoding.EncodeToString(h.Sum(nil))
	if sign != want {
		t.Errorf("sign = %q, want %q", sign, want)
	}
}

func TestSendFeishuNoSecretNoSign(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if err := sendFeishu(&Channel{Kind: "feishu", URL: srv.URL}, Event{Title: "t"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["sign"]; ok {
		t.Error("无 secret 时不应带 sign")
	}
}

func TestFeishuSignDeterministic(t *testing.T) {
	a, err := feishuSign("1700000000", "k")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := feishuSign("1700000000", "k")
	if a == "" || a != b {
		t.Errorf("签名应确定且非空: %q vs %q", a, b)
	}
	if c, _ := feishuSign("1700000001", "k"); c == a {
		t.Error("不同 timestamp 应得不同签名")
	}
}
