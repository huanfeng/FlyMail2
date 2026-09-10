package send_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"flymail-core/types"

	"github.com/gin-gonic/gin"

	"flymail/modules/email/send"
)

// newSendServer 装配一条只到 sendFn 为止的发送链路，raw 经 capture 回传以便断言 MIME。
func newSendServer(t *testing.T, capture *[]byte, envelopeFrom *string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	accounts := defaultAccounts()
	accounts.acct.Name = "主账户"
	accounts.aliases = map[string]string{"sales@example.com": "销售部"}
	svc := newTestService(accounts, &fakeFolders{})
	svc.SetSenders(
		func(cfg types.SMTPConfig, from string, recipients []string, raw []byte) error {
			*capture = append([]byte(nil), raw...)
			*envelopeFrom = from
			return nil
		},
		func(cfg types.IMAPConfig, mailbox string, raw []byte) error { return nil },
	)
	r := gin.New()
	send.RegisterRoutes(r.Group(""), svc)
	return r
}

// postMultipart 拼一个带 payload / inline / attachments 三类字段的表单请求。
func postMultipart(t *testing.T, r *gin.Engine, payload map[string]any,
	inline []struct{ name, ctype, data string }, attachments []struct{ name, data string },
) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("序列化 payload 失败: %v", err)
	}
	if err := w.WriteField("payload", string(raw)); err != nil {
		t.Fatalf("写 payload 字段失败: %v", err)
	}
	for _, f := range inline {
		// 必须用 CreatePart 手工写 Content-Type：CreateFormFile 一律写 application/octet-stream，
		// 那样 ctype 就是死字段，"客户端声明了类型"这条分支根本没被测到。
		part, err := w.CreatePart(fileHeader("inline", f.name, f.ctype))
		if err != nil {
			t.Fatalf("创建 inline 字段失败: %v", err)
		}
		part.Write([]byte(f.data))
	}
	for _, f := range attachments {
		part, err := w.CreateFormFile("attachments", f.name)
		if err != nil {
			t.Fatalf("创建 attachments 字段失败: %v", err)
		}
		part.Write([]byte(f.data))
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/send", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// fileHeader 拼一个带指定 Content-Type 的文件字段头。ctype 为空时按浏览器的常见行为
// 写 application/octet-stream（那正是内联图需要服务端嗅探的场景）。
func fileHeader(field, filename, ctype string) textproto.MIMEHeader {
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filename))
	h.Set("Content-Type", ctype)
	return h
}

// TestSendInlineFormPairsCIDs inline 文件按下标与 inline_cids 配对，落成 related 内的资源。
func TestSendInlineFormPairsCIDs(t *testing.T) {
	var raw []byte
	var envFrom string
	r := newSendServer(t, &raw, &envFrom)

	rec := postMultipart(t, r, map[string]any{
		"account_id":  1,
		"to":          []string{"to@example.com"},
		"subject":     "带内联图",
		"body_html":   `<p><img src="cid:ii_one"><img src="cid:ii_two"></p>`,
		"inline_cids": []string{"ii_one", "ii_two"},
	}, []struct{ name, ctype, data string }{
		{"one.png", "image/png", "ONE"},
		{"two.png", "image/png", "TWO"},
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("应 200，实际 %d: %s", rec.Code, rec.Body.String())
	}
	_, root := parseMIME(t, raw)
	if root.mediaType != "multipart/related" {
		t.Fatalf("应为 multipart/related，实际 %s", root.mediaType)
	}
	if len(root.children) != 3 {
		t.Fatalf("应为 HTML + 2 张图，实际 %d 个 part", len(root.children))
	}
	// 顺序必须与 inline_cids 严格对应，错配的结果是收件方看到裂图
	if root.children[1].contentID != "ii_one" || string(root.children[1].body) != "ONE" {
		t.Errorf("第一张图配对错误: cid=%q body=%q", root.children[1].contentID, root.children[1].body)
	}
	if root.children[2].contentID != "ii_two" || string(root.children[2].body) != "TWO" {
		t.Errorf("第二张图配对错误: cid=%q body=%q", root.children[2].contentID, root.children[2].body)
	}
}

// TestSendInlineCountMismatch 数量对不上宁可整封拒收，也不能错配 cid。
func TestSendInlineCountMismatch(t *testing.T) {
	var raw []byte
	var envFrom string
	r := newSendServer(t, &raw, &envFrom)

	rec := postMultipart(t, r, map[string]any{
		"account_id":  1,
		"to":          []string{"to@example.com"},
		"inline_cids": []string{"ii_one"},
	}, []struct{ name, ctype, data string }{
		{"one.png", "image/png", "ONE"},
		{"two.png", "image/png", "TWO"},
	}, nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("数量不匹配应 400，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if raw != nil {
		t.Error("拒收的请求不应触发 SMTP 发送")
	}
}

// TestSendInlineRejectsHeaderInjection cid 会写进 Content-ID 头，CRLF 必须在入口就拦下。
func TestSendInlineRejectsHeaderInjection(t *testing.T) {
	var raw []byte
	var envFrom string
	r := newSendServer(t, &raw, &envFrom)

	rec := postMultipart(t, r, map[string]any{
		"account_id":  1,
		"to":          []string{"to@example.com"},
		"inline_cids": []string{"ii_x\r\nBcc: attacker@evil.com"},
	}, []struct{ name, ctype, data string }{
		{"one.png", "image/png", "ONE"},
	}, nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 cid 应 400，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if raw != nil {
		t.Error("拒收的请求不应触发 SMTP 发送")
	}
}

// TestSendAliasSplitsEnvelopeAndHeader From 头用别名，SMTP 信封发件人仍是主地址。
func TestSendAliasSplitsEnvelopeAndHeader(t *testing.T) {
	var raw []byte
	var envFrom string
	r := newSendServer(t, &raw, &envFrom)

	body, _ := json.Marshal(map[string]any{
		"account_id": 1,
		"to":         []string{"to@example.com"},
		"subject":    "别名发信",
		"body_html":  "<p>x</p>",
		"from_alias": "sales@example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("应 200，实际 %d: %s", rec.Code, rec.Body.String())
	}
	// SPF 校验的是信封域，多数服务器又只接受等于认证账户的信封发件人
	if envFrom != "sender@example.com" {
		t.Errorf("信封发件人应为账户主地址，实际 %q", envFrom)
	}
	msg, _ := parseMIME(t, raw)
	if from := msg.Header.Get("From"); !strings.Contains(from, "sales@example.com") {
		t.Errorf("From 头应为别名，实际 %q", from)
	}
	// 不写 Sender 头：Gmail 会据此显示"由 … 代发"，主流客户端也都不写
	if s := msg.Header.Get("Sender"); s != "" {
		t.Errorf("不应写 Sender 头，实际 %q", s)
	}
}

// TestSendForeignAliasRejected 不属于本账户的别名必须 400，而不是 500。
func TestSendForeignAliasRejected(t *testing.T) {
	var raw []byte
	var envFrom string
	r := newSendServer(t, &raw, &envFrom)

	body, _ := json.Marshal(map[string]any{
		"account_id": 1,
		"to":         []string{"to@example.com"},
		"from_alias": "victim@other.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("越权别名应 400，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if raw != nil {
		t.Error("拒收的请求不应触发 SMTP 发送")
	}
}

// TestSendInlineSniffsContentType 表单上传常常只给 application/octet-stream，
// 那样发出去收件方不会当图片渲染。内联资源必须由服务端嗅探定型。
func TestSendInlineSniffsContentType(t *testing.T) {
	var raw []byte
	var envFrom string
	r := newSendServer(t, &raw, &envFrom)

	png := string([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d})
	gif := "GIF89a" + strings.Repeat("z", 10)

	rec := postMultipart(t, r, map[string]any{
		"account_id":  1,
		"to":          []string{"to@example.com"},
		"body_html":   `<p><img src="cid:ii_p"><img src="cid:ii_g"></p>`,
		"inline_cids": []string{"ii_p", "ii_g"},
	}, []struct{ name, ctype, data string }{
		{"a.png", "", png},
		{"b.gif", "", gif},
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("应 200，实际 %d: %s", rec.Code, rec.Body.String())
	}
	_, root := parseMIME(t, raw)
	if len(root.children) != 3 {
		t.Fatalf("应为 HTML + 2 张图，实际 %d 个 part", len(root.children))
	}
	if root.children[1].mediaType != "image/png" {
		t.Errorf("PNG 应嗅探为 image/png，实际 %q", root.children[1].mediaType)
	}
	if root.children[2].mediaType != "image/gif" {
		t.Errorf("GIF 应嗅探为 image/gif，实际 %q", root.children[2].mediaType)
	}
}

// TestSendInlineHonorsDeclaredContentType 客户端声明了非 octet-stream 的类型就照用：
// svg 是文本格式，http.DetectContentType 嗅不出来，只能信声明（否则发出去是 text/plain，图裂）。
func TestSendInlineHonorsDeclaredContentType(t *testing.T) {
	var raw []byte
	var envFrom string
	r := newSendServer(t, &raw, &envFrom)

	svg := `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`
	rec := postMultipart(t, r, map[string]any{
		"account_id":  1,
		"to":          []string{"to@example.com"},
		"body_html":   `<p><img src="cid:ii_s"></p>`,
		"inline_cids": []string{"ii_s"},
	}, []struct{ name, ctype, data string }{
		{"a.svg", "image/svg+xml", svg},
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("应 200，实际 %d: %s", rec.Code, rec.Body.String())
	}
	_, root := parseMIME(t, raw)
	if len(root.children) != 2 {
		t.Fatalf("应为 HTML + 1 张图，实际 %d 个 part", len(root.children))
	}
	if root.children[1].mediaType != "image/svg+xml" {
		t.Errorf("应沿用客户端声明的 image/svg+xml，实际 %q", root.children[1].mediaType)
	}
}

// TestSendInlineDuplicateCIDRejected 两个 part 顶着同一个 Content-ID，
// 收件方只认第一个，第二张图永远显示不出来。既然数量对不上是整封拒收，重复也一样。
func TestSendInlineDuplicateCIDRejected(t *testing.T) {
	var raw []byte
	var envFrom string
	r := newSendServer(t, &raw, &envFrom)

	rec := postMultipart(t, r, map[string]any{
		"account_id":  1,
		"to":          []string{"to@example.com"},
		"body_html":   `<p><img src="cid:ii_dup"></p>`,
		"inline_cids": []string{"ii_dup", "ii_dup"},
	}, []struct{ name, ctype, data string }{
		{"one.png", "image/png", "ONE"},
		{"two.png", "image/png", "TWO"},
	}, nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("重复 cid 应 400，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if raw != nil {
		t.Error("拒收的请求不应触发 SMTP 发送")
	}
}
