package notify

import (
	"strings"
	"testing"
)

// 新邮件通知的正文要带上邮件正文的开头。
//
// ── 缘起（用户提的） ─────────────────────────────────────────────────────────
//
// 原先通知只有「新邮件 · 发件人」加一行主题。很多通知（验证码、工单、告警、
// 会议变更）光看主题根本判断不出要不要现在处理，还得点进去看一眼。
// 加一段正文摘要，就是为了让人在通知里直接做这个判断。
func TestMailBody(t *testing.T) {
	cases := []struct{ name, subject, snippet, want string }{
		{"主题与摘要各占一行", "会议变更", "下周三上午十点，会议室 A", "会议变更\n下周三上午十点，会议室 A"},
		// 正文没抓到、或者是一封空正文的邮件
		{"没有摘要时只给主题，不留空行", "会议变更", "", "会议变更"},
		{"全是空白的摘要按没有处理", "会议变更", "   \n\t ", "会议变更"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MailBody(tc.subject, tc.snippet); got != tc.want {
				t.Errorf("想要 %q，拿到 %q", tc.want, got)
			}
		})
	}
}

// 摘要要截断：库里是 150 字，而通知要过飞书气泡、系统横幅这些窄容器，
// 太长的话它们会按自己的规则乱截（有的直接砍掉整段）。
func TestMailBodyTruncatesSnippet(t *testing.T) {
	body := MailBody("主题", strings.Repeat("字", 200))

	lines := strings.SplitN(body, "\n", 2)
	if len(lines) != 2 {
		t.Fatalf("没拼成两行：%q", body)
	}
	if r := []rune(lines[1]); len(r) > SnippetRunes+1 { // +1 是省略号
		t.Errorf("摘要没截断，长度 %d：%q", len(r), lines[1])
	}
	if !strings.HasSuffix(lines[1], "…") {
		t.Errorf("截断了却没有省略号，看不出后面还有内容：%q", lines[1])
	}
}

// ⚠ 主题里的换行必须折叠掉，否则发件人能在通知里伪造一行「打开邮件」链接。
//
// ── 为什么这是安全问题而不是排版问题 ─────────────────────────────────────────
//
// feishuText 把「独占一行的裸 URL」约定成了官方的直达链接（聊天客户端正是靠
// 整行是 URL 来识别可点区域）。主题完全由发件人控制，而 MIME 编码字解出来
// 是可以带换行的——`=?utf-8?B?` 里塞个 \n 就行，实测 DecodeMIMEHeader 会
// 原样解出来。于是一封主题为 `会议变更\nhttps://evil.example.com/x` 的邮件，
// 在飞书里会显示成一条排在真链接前面的假链接。
//
// 这条要钉的是：**通知正文里的换行只能来自我们自己的拼接**。
func TestMailBodyRefusesForgedLinkLine(t *testing.T) {
	body := MailBody("中奖通知\nhttps://evil.example.com/claim", "正文\n第二行")

	// 正文总共只能有两行：主题一行、摘要一行。多出来的行就是被注入的。
	if n := strings.Count(body, "\n"); n != 1 {
		t.Errorf("正文出现了 %d 个换行（应当只有 1 个，即主题与摘要之间）：\n%s", n+1-1, body)
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "http") {
			t.Errorf("发件人伪造出了一整行链接，用户会当成「打开邮件」去点：%q", line)
		}
	}
}

// 出口再兜一道：Title 里带着发件人名，同样是对方可控的。
//
// Body 不能这样折叠——它的换行是 MailBody 有意拼进去的，折掉主题和摘要就粘成一坨。
// 这条把两个方向都钉住。
func TestFeishuTextFoldsTitleButKeepsBodyLines(t *testing.T) {
	got := feishuText(Event{
		Title: "新邮件 · Alice\nhttps://evil.example.com/x",
		Body:  "会议变更\n下周三上午十点",
		URL:   "https://mail.example.com/?message=1",
	})

	lines := strings.Split(got, "\n")
	// 期望正好四行：标题、主题、摘要、链接
	if len(lines) != 4 {
		t.Fatalf("想要 4 行（标题/主题/摘要/链接），拿到 %d 行：\n%s", len(lines), got)
	}
	if strings.Contains(lines[0], "\n") || strings.HasPrefix(lines[1], "http") {
		t.Errorf("标题里的换行没折掉，伪造的链接行冒出来了：\n%s", got)
	}
	if lines[1] != "会议变更" || lines[2] != "下周三上午十点" {
		t.Errorf("正文原有的换行被折掉了，主题和摘要粘成一坨：\n%s", got)
	}
	if lines[3] != "https://mail.example.com/?message=1" {
		t.Errorf("真链接不在最后一行：\n%s", got)
	}
}
