package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"
)

// composerAttachment 详情里的附件项（含内联标记与 cid）。
type composerAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ContentID   string `json:"content_id"`
	IsInline    bool   `json:"is_inline"`
}

type composerDetail struct {
	Subject     string               `json:"subject"`
	FromAddr    string               `json:"from_addr"`
	FromName    string               `json:"from_name"`
	HTMLBody    string               `json:"html_body"`
	Attachments []composerAttachment `json:"attachments"`
}

// sendMultipart 走真实的 multipart/form-data 发送路径（payload + inline + attachments）。
func (c *apiClient) sendMultipart(t *testing.T, payload map[string]any,
	inline map[string][]byte, inlineOrder []string, attachments map[string][]byte,
) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("序列化 payload: %v", err)
	}
	if err := w.WriteField("payload", string(raw)); err != nil {
		t.Fatalf("写 payload: %v", err)
	}
	for _, name := range inlineOrder {
		part, err := w.CreateFormFile("inline", name)
		if err != nil {
			t.Fatalf("创建 inline 字段: %v", err)
		}
		part.Write(inline[name])
	}
	for name, data := range attachments {
		part, err := w.CreateFormFile("attachments", name)
		if err != nil {
			t.Fatalf("创建 attachments 字段: %v", err)
		}
		part.Write(data)
	}
	w.Close()

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1/send", &buf)
	if err != nil {
		t.Fatalf("构造请求: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("发送请求: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode, string(body)
}

// 1×1 透明 PNG，用作内联图。
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89,
}

// TestComposer_InlineImageRoundTrip 端到端验证内联图：
// 经 /send 发出 → GreenMail 投递 → IMAP 同步回来 → 解析出带 cid 的内联附件。
// 这是 multipart/related 结构是否真的成立的唯一硬证据。
func TestComposer_InlineImageRoundTrip(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	mb := uniqueMailbox(t)
	acctID := c.createAccount(mb)

	// 先做一次基线同步，之后投递的邮件才会被当作新邮件
	c.triggerSyncAndWait(acctID, 60*time.Second)

	const cid = "ii_e2e_inline_1"
	subject := "内联图往返 " + mb
	status, body := c.sendMultipart(t, map[string]any{
		"account_id":  acctID,
		"to":          []string{mb},
		"subject":     subject,
		"body_html":   `<p>见图：<img src="cid:` + cid + `"></p>`,
		"inline_cids": []string{cid},
	}, map[string][]byte{"pixel.png": tinyPNG}, []string{"pixel.png"},
		map[string][]byte{"note.txt": []byte("普通附件")})
	if status != http.StatusOK {
		t.Fatalf("发送应 200，实际 %d: %s", status, body)
	}

	c.triggerSyncAndWait(acctID, 60*time.Second)
	inbox := findFolder(c.listFolders(acctID), "inbox")
	var target uint
	for _, m := range c.listMessages(inbox.ID) {
		if m.Subject == subject {
			target = m.ID
			break
		}
	}
	if target == 0 {
		t.Fatalf("收件箱里没找到主题为 %q 的邮件", subject)
	}

	var d composerDetail
	c.mustJSON(http.MethodGet, "/api/v1/messages/"+utoa(target), nil, http.StatusOK, &d)

	// 正文里的 cid 引用必须原样保留——前端要靠它把内联图指到附件端点
	if !strings.Contains(d.HTMLBody, "cid:"+cid) {
		t.Errorf("正文丢失 cid 引用: %q", d.HTMLBody)
	}

	var inlineAtt, plainAtt *composerAttachment
	for i := range d.Attachments {
		switch {
		case d.Attachments[i].ContentID == cid:
			inlineAtt = &d.Attachments[i]
		case d.Attachments[i].Filename == "note.txt":
			plainAtt = &d.Attachments[i]
		}
	}
	if inlineAtt == nil {
		t.Fatalf("未解析出 cid=%s 的内联附件，实际附件：%+v", cid, d.Attachments)
	}
	if !inlineAtt.IsInline {
		t.Errorf("内联图的 is_inline 应为 true：%+v", inlineAtt)
	}
	if inlineAtt.ContentType != "image/png" {
		t.Errorf("内联图 Content-Type 应为 image/png，实际 %q", inlineAtt.ContentType)
	}
	if inlineAtt.Size != int64(len(tinyPNG)) {
		t.Errorf("内联图大小应为 %d，实际 %d", len(tinyPNG), inlineAtt.Size)
	}
	// 普通附件必须还在 mixed 层，没有被 related 吞掉
	if plainAtt == nil {
		t.Errorf("普通附件丢失，实际附件：%+v", d.Attachments)
	} else if plainAtt.IsInline {
		t.Errorf("普通附件不该被标成内联：%+v", plainAtt)
	}
}

// TestComposer_AliasFromHeader 别名发信：收到的 From 头是别名，而信封发件人仍是主地址。
func TestComposer_AliasFromHeader(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	mb := uniqueMailbox(t)
	acctID := c.createAccount(mb)
	c.triggerSyncAndWait(acctID, 60*time.Second)

	alias := "sales-" + mb
	c.mustJSON(http.MethodPost, "/api/v1/accounts/"+utoa(acctID)+"/aliases",
		map[string]any{"email": alias, "display_name": "销售部", "is_default": true},
		http.StatusCreated, nil)

	// 不属于本账户的别名必须被拒——服务端是防伪造的唯一关卡
	resp, data := c.do(http.MethodPost, "/api/v1/send", map[string]any{
		"account_id": acctID,
		"to":         []string{mb},
		"subject":    "越权别名",
		"body_html":  "<p>x</p>",
		"from_alias": "victim@elsewhere.test",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("越权别名应 400，实际 %d: %s", resp.StatusCode, data)
	}

	subject := "别名发信 " + mb
	c.mustJSON(http.MethodPost, "/api/v1/send", map[string]any{
		"account_id": acctID,
		"to":         []string{mb},
		"subject":    subject,
		"body_html":  "<p>正文</p>",
		"from_alias": alias,
	}, http.StatusOK, nil)

	c.triggerSyncAndWait(acctID, 60*time.Second)
	inbox := findFolder(c.listFolders(acctID), "inbox")
	var target uint
	for _, m := range c.listMessages(inbox.ID) {
		if m.Subject == subject {
			target = m.ID
			break
		}
	}
	if target == 0 {
		t.Fatalf("收件箱里没找到主题为 %q 的邮件", subject)
	}

	var d composerDetail
	c.mustJSON(http.MethodGet, "/api/v1/messages/"+utoa(target), nil, http.StatusOK, &d)
	if !strings.EqualFold(d.FromAddr, alias) {
		t.Errorf("From 地址应为别名 %q，实际 %q", alias, d.FromAddr)
	}
	if d.FromName != "销售部" {
		t.Errorf("From 显示名应为「销售部」，实际 %q", d.FromName)
	}
}

// TestComposer_SignatureSanitized 签名保存时净化，读回不带脚本。
func TestComposer_SignatureSanitized(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	acctID := c.createAccount(uniqueMailbox(t))

	var sig struct {
		BodyHTML   string `json:"body_html"`
		UseOnNew   bool   `json:"use_on_new"`
		UseOnReply bool   `json:"use_on_reply"`
	}
	// 未配置时返回空签名，而不是 404
	c.mustJSON(http.MethodGet, "/api/v1/accounts/"+utoa(acctID)+"/signature", nil, http.StatusOK, &sig)
	if sig.BodyHTML != "" {
		t.Errorf("未配置时应为空签名，实际 %q", sig.BodyHTML)
	}

	c.mustJSON(http.MethodPut, "/api/v1/accounts/"+utoa(acctID)+"/signature", map[string]any{
		"body_html":    `<p>张三<script>alert(1)</script></p>`,
		"use_on_new":   true,
		"use_on_reply": true,
	}, http.StatusOK, &sig)

	c.mustJSON(http.MethodGet, "/api/v1/accounts/"+utoa(acctID)+"/signature", nil, http.StatusOK, &sig)
	if strings.Contains(strings.ToLower(sig.BodyHTML), "script") {
		t.Errorf("签名未净化: %q", sig.BodyHTML)
	}
	if !strings.Contains(sig.BodyHTML, "张三") || !sig.UseOnNew || !sig.UseOnReply {
		t.Errorf("签名读回不符: %+v", sig)
	}
}
