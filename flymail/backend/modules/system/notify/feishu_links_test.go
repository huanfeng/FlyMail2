package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cardJSON 把卡片序列化，便于整体搜字符串。
func cardJSON(t *testing.T, card map[string]any) string {
	t.Helper()
	b, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func mailWithLinks(links ...string) *MailData {
	m := sampleMail()
	m.Links = links
	return m
}

func TestLinksRenderedAsClickable(t *testing.T) {
	card := feishuCard(mailEvent(mailWithLinks("https://example.com/a")), LevelFull, feishuBodyRunes)
	js := cardJSON(t, card)

	if !strings.Contains(js, "lark_md") {
		t.Error("链接区应走 lark_md，否则点不动")
	}
	if !strings.Contains(js, `[https://example.com/a](https://example.com/a)`) {
		t.Errorf("链接未按 markdown 渲染：%s", js)
	}
}

// ⚠ 这条是链接功能的安全支点。
//
// 显示文本必须与跳转目标**逐字节相同**。一旦允许两者分家，一封正文写着
// 「请登录 [icbc.com.cn](http://evil.com) 处理」的钓鱼邮件，在卡片里就是一条
// 看起来完全正经的银行链接——而且紧挨着我们自己的「打开邮件」按钮，
// 比在邮件客户端里更可信。
func TestLinkDisplayAlwaysEqualsTarget(t *testing.T) {
	for name, u := range map[string]string{
		"普通":      "https://example.com/a",
		"带查询串":    "https://example.com/a?x=1&y=2",
		"带右括号":    "https://zh.wikipedia.org/wiki/Go_(编程语言)",
		"带方括号":    "https://example.com/a[1]",
		"整条都是括号":  "https://example.com/)](",
		"看着像另一个站": "https://evil.com/?fake=icbc.com.cn",
	} {
		t.Run(name, func(t *testing.T) {
			md := markdownLink(u)
			// 形如 [显示](目标)，取出两段比对
			if !strings.HasPrefix(md, "[") || !strings.HasSuffix(md, ")") {
				t.Fatalf("形状不对：%q", md)
			}
			mid := strings.Index(md, "](")
			if mid < 0 {
				t.Fatalf("形状不对：%q", md)
			}
			display, target := md[1:mid], md[mid+2:len(md)-1]
			if display != target {
				t.Errorf("显示与目标分家了：显示 %q，实际 %q", display, target)
			}
			// 而且中间不能再出现能提前终结构造的字符，否则飞书会把后面的内容
			// 解析成别的东西——那正是「显示与目标分家」的实现路径。
			if strings.ContainsAny(display, "()[]") {
				t.Errorf("未转义的括号会提前终结 markdown 构造：%q", display)
			}
		})
	}
}

// 链接是正文内容，必须和正文走同一道闸门：配了「基本信息」的渠道
// 一个字正文都不带，链接自然也不能漏出去——不然一封密码重置邮件的
// 重置链接就进了多人群，比正文泄漏更严重。
func TestLinksRespectContentLevel(t *testing.T) {
	evt := mailEvent(mailWithLinks("https://example.com/reset?token=SECRET"))
	for level, want := range map[ContentLevel]bool{
		LevelBasic:   false,
		LevelSnippet: true,
		LevelFull:    true,
	} {
		t.Run(string(level), func(t *testing.T) {
			js := cardJSON(t, feishuCard(evt, level, feishuBodyRunes))
			if got := strings.Contains(js, "SECRET"); got != want {
				t.Errorf("level=%s 链接出现=%v，期望 %v", level, got, want)
			}
		})
	}
}

func TestNoLinkSectionWhenNoLinks(t *testing.T) {
	js := cardJSON(t, feishuCard(mailEvent(sampleMail()), LevelFull, feishuBodyRunes))
	if strings.Contains(js, "正文中的链接") {
		t.Error("没有链接时不该出现这个区块")
	}
}

// ── 字节预算 ────────────────────────────────────────────────────────────────

// 超了预算飞书直接拒收整条消息，用户看到的是「有几封邮件没推送」，
// 而不是「正文被截短了」——所以这道裁剪不能漏。
func TestPayloadFitsBudget(t *testing.T) {
	for name, body := range map[string]string{
		// 中文最能撑爆预算：一个字 UTF-8 占 3 字节，按字符数算会严重低估
		"大段中文":  strings.Repeat("这是一封很长的邮件，", 20000),
		"大段英文":  strings.Repeat("this is a long mail. ", 20000),
		"emoji": strings.Repeat("🎉", 20000),
		// JSON 转义会把控制字符变成 6 字节的 \uXXXX
		"控制字符": strings.Repeat("a\x01\x02\x03", 20000),
	} {
		t.Run(name, func(t *testing.T) {
			m := sampleMail()
			m.Body = body
			payload, err := json.Marshal(fitFeishuCard(mailEvent(m), LevelFull))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if len(payload) > feishuPayloadBudget {
				t.Errorf("超出预算：%d > %d 字节", len(payload), feishuPayloadBudget)
			}
		})
	}
}

// 正常长度的邮件不该被这道裁剪碰到——它是安全网，不是又一层截断。
func TestShortMailNotTrimmed(t *testing.T) {
	m := sampleMail()
	js := cardJSON(t, fitFeishuCard(mailEvent(m), LevelFull))
	if !strings.Contains(js, "请各自准备进度") {
		t.Errorf("短邮件被截了：%s", js)
	}
	if strings.Contains(js, "…") {
		t.Error("短邮件不该出现截断省略号")
	}
}

func TestConfiguredBodyLimit(t *testing.T) {
	t.Cleanup(func() { SetBodyRunesProvider(nil) })

	m := sampleMail()
	m.Body = strings.Repeat("字", 5000)

	SetBodyRunesProvider(func() int { return 100 })
	short := cardJSON(t, fitFeishuCard(mailEvent(m), LevelFull))

	SetBodyRunesProvider(nil) // 恢复默认（更大）
	long := cardJSON(t, fitFeishuCard(mailEvent(m), LevelFull))

	if len(short) >= len(long) {
		t.Errorf("配置的上限没生效：短 %d，默认 %d", len(short), len(long))
	}
	// 负数与 0 都该回落到默认，而不是把正文截成空
	SetBodyRunesProvider(func() int { return -5 })
	if configuredBodyRunes() != feishuBodyRunes {
		t.Errorf("负数应回落到默认，got %d", configuredBodyRunes())
	}
}

// webhook 消费方拿到的 body 已经是剥成纯文本的正文，<a href> 的地址在那一步
// 就没了——不单独给一份，它们再也解不出来。
func TestWebhookCarriesLinks(t *testing.T) {
	evt := mailEvent(mailWithLinks("https://example.com/reset?token=SECRET"))

	for level, want := range map[ContentLevel]bool{
		LevelBasic:   false,
		LevelSnippet: true,
		LevelFull:    true,
	} {
		t.Run(string(level), func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				got = string(b)
			}))
			defer srv.Close()

			if err := sendWebhook(&Channel{URL: srv.URL}, evt, level); err != nil {
				t.Fatalf("sendWebhook: %v", err)
			}
			if has := strings.Contains(got, "SECRET"); has != want {
				t.Errorf("level=%s 链接出现=%v，期望 %v；载荷=%s", level, has, want, got)
			}
			if want && !strings.Contains(got, `"links"`) {
				t.Errorf("应有 links 字段：%s", got)
			}
		})
	}
}
