package send_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"flymail/modules/email/send"
)

func TestBuildRFC5322(t *testing.T) {
	from := "sender@example.com"
	req := send.SendRequest{
		AccountID:  1,
		To:         []string{"alice@example.com", "bob@example.com"},
		Cc:         []string{"carol@example.com"},
		Bcc:        []string{"hidden@example.com"},
		Subject:    "测试邮件主题",
		BodyHTML:   "<html><body><p>你好，世界！</p></body></html>",
		InReplyTo:  "original-msg-id@example.com",
		References: "<original-msg-id@example.com>",
	}
	msgID := "20260601.abc123@example.com"
	date := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	raw, err := send.BuildRFC5322(from, req, msgID, date)
	if err != nil {
		t.Fatalf("BuildRFC5322 error: %v", err)
	}

	rawStr := string(raw)

	// 检查 From
	if !strings.Contains(rawStr, "From: ") {
		t.Error("missing From header")
	}
	if !strings.Contains(rawStr, "sender@example.com") {
		t.Error("From should contain sender@example.com")
	}

	// 检查 To
	if !strings.Contains(rawStr, "To: ") {
		t.Error("missing To header")
	}
	if !strings.Contains(rawStr, "alice@example.com") {
		t.Error("To should contain alice@example.com")
	}
	if !strings.Contains(rawStr, "bob@example.com") {
		t.Error("To should contain bob@example.com")
	}

	// 检查 Cc
	if !strings.Contains(rawStr, "Cc: ") {
		t.Error("missing Cc header")
	}
	if !strings.Contains(rawStr, "carol@example.com") {
		t.Error("Cc should contain carol@example.com")
	}

	// 检查 Subject 是 B-encoding 编码（中文）
	if !strings.Contains(rawStr, "Subject: =?UTF-8?b?") {
		t.Errorf("Subject should be B-encoded, got: %s", extractHeader(rawStr, "Subject"))
	}

	// 检查 Content-Transfer-Encoding
	if !strings.Contains(rawStr, "Content-Transfer-Encoding: base64") {
		t.Error("missing Content-Transfer-Encoding: base64")
	}

	// 检查 MIME-Version
	if !strings.Contains(rawStr, "MIME-Version: 1.0") {
		t.Error("missing MIME-Version: 1.0")
	}

	// 检查 Content-Type
	if !strings.Contains(rawStr, "Content-Type: text/html; charset=UTF-8") {
		t.Error("missing Content-Type: text/html; charset=UTF-8")
	}

	// 检查 Message-ID
	if !strings.Contains(rawStr, "Message-ID: <"+msgID+">") {
		t.Errorf("missing Message-ID, raw: %s", rawStr)
	}

	// 检查 In-Reply-To（值不重复添加尖括号）
	if !strings.Contains(rawStr, "In-Reply-To: <original-msg-id@example.com>") {
		t.Errorf("missing or wrong In-Reply-To, raw: %s", rawStr)
	}

	// 检查 References
	if !strings.Contains(rawStr, "References: ") {
		t.Error("missing References header")
	}

	// 检查 Bcc 不出现在头部
	if strings.Contains(rawStr, "Bcc:") {
		t.Error("Bcc should NOT appear in raw headers")
	}
	if strings.Contains(rawStr, "hidden@example.com") {
		t.Error("Bcc address should NOT appear in raw message")
	}

	// 检查 body 是合法 base64，解码回原 HTML
	parts := strings.SplitN(rawStr, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatal("raw message missing blank line between headers and body")
	}
	bodyB64 := strings.ReplaceAll(parts[1], "\r\n", "")
	decoded, err := base64.StdEncoding.DecodeString(bodyB64)
	if err != nil {
		t.Fatalf("body base64 decode error: %v", err)
	}
	if string(decoded) != req.BodyHTML {
		t.Errorf("body mismatch: got %q, want %q", string(decoded), req.BodyHTML)
	}
}

// TestBuildRFC5322_NoInReplyTo 验证无 InReplyTo 时不输出该头。
func TestBuildRFC5322_NoInReplyTo(t *testing.T) {
	req := send.SendRequest{
		AccountID: 1,
		To:        []string{"alice@example.com"},
		Subject:   "Hello",
		BodyHTML:  "<p>hi</p>",
	}
	raw, err := send.BuildRFC5322("sender@example.com", req, "msgid@x.com", time.Now())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	rawStr := string(raw)
	if strings.Contains(rawStr, "In-Reply-To:") {
		t.Error("In-Reply-To should not appear when empty")
	}
	if strings.Contains(rawStr, "References:") {
		t.Error("References should not appear when empty")
	}
}

// TestBuildRFC5322_InReplyToWithBrackets 验证已带尖括号时不重复添加。
func TestBuildRFC5322_InReplyToWithBrackets(t *testing.T) {
	req := send.SendRequest{
		AccountID: 1,
		To:        []string{"alice@example.com"},
		Subject:   "Reply",
		BodyHTML:  "<p>re</p>",
		InReplyTo: "<already-bracketed@example.com>",
	}
	raw, err := send.BuildRFC5322("sender@example.com", req, "msgid@x.com", time.Now())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	rawStr := string(raw)
	// 应只有一对尖括号
	if strings.Contains(rawStr, "<<") || strings.Contains(rawStr, ">>") {
		t.Errorf("double brackets found in In-Reply-To: %s", extractHeader(rawStr, "In-Reply-To"))
	}
	if !strings.Contains(rawStr, "In-Reply-To: <already-bracketed@example.com>") {
		t.Errorf("In-Reply-To wrong: %s", extractHeader(rawStr, "In-Reply-To"))
	}
}

func extractHeader(raw, name string) string {
	for _, line := range strings.Split(raw, "\r\n") {
		if strings.HasPrefix(line, name+": ") {
			return line
		}
	}
	return ""
}
