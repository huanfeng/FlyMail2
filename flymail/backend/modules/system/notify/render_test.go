package notify

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func mailEvent(mail *MailData) Event {
	return Event{
		Type: EventMailNew, AccountID: 1, MessageID: 42,
		Title: "新邮件 · Alice", Body: "会议变更",
		URL:  "https://mail.example.com/?account=1&folder=7&message=42",
		Mail: mail,
	}
}

func sampleMail() *MailData {
	return &MailData{
		From: "Alice", Subject: "会议变更", Date: time.Now(),
		Snippet: "下周三上午十点，会议室 A",
		Body:    "下周三上午十点，会议室 A，请各自准备进度。\n另外记得带上上周的纪要。",
	}
}

// 三档内容各自带多少正文。
//
// ── 为什么要分级 ─────────────────────────────────────────────────────────────
//
// 外发通知会进到第三方（飞书群、任意 webhook），而「主题不敏感、正文敏感」的邮件
// 很常见：验证码、密码重置、财务金额。推到多人群里的摘要是撤不回来的。
// 反过来接私人 webhook 做自动化的人又要全文。所以按渠道各配一档。
func TestBodyForLevels(t *testing.T) {
	m := sampleMail()

	// ⚠ 这条是这个功能的全部意义所在：配了基本信息就一个字正文都不能出去
	if got := bodyFor(m, LevelBasic, 1000); got != "" {
		t.Errorf("basic 档漏出了正文：%q", got)
	}
	if got := bodyFor(m, LevelSnippet, 1000); got != m.Snippet {
		t.Errorf("snippet 档想要摘要，拿到 %q", got)
	}
	if got := bodyFor(m, LevelFull, 1000); got != m.Body {
		t.Errorf("full 档想要全文，拿到 %q", got)
	}
}

// 全文没取到时回落到摘要，而不是变成一条没有正文的通知。
func TestBodyForFullFallsBackToSnippet(t *testing.T) {
	m := sampleMail()
	m.Body = "" // 正文还没落库
	if got := bodyFor(m, LevelFull, 1000); got != m.Snippet {
		t.Errorf("全文缺失时应回落到摘要，拿到 %q", got)
	}
}

// 超长要截断，而且要按字符截。
//
// ⚠ 按字节截会把多字节字符劈成两半，中文邮件必然踩到——那会产生一个非法 UTF-8
// 序列，JSON 序列化之后接收端看到的是替换字符或者直接解析失败。
func TestTruncateRunesCutsOnCharBoundary(t *testing.T) {
	got := truncateRunes(strings.Repeat("中", 100), 10)
	if r := []rune(got); len(r) != 11 { // 10 + 省略号
		t.Errorf("截断长度不对：%d 个字符", len(r))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("截断了却没有省略号，看不出后面还有内容：%q", got)
	}
	// 劈开多字节字符的写法会在这里暴露
	for _, r := range got {
		if r == '�' {
			t.Fatalf("出现了替换字符，说明是按字节截的：%q", got)
		}
	}
	// 没超长的不动它
	if got := truncateRunes("短", 10); got != "短" {
		t.Errorf("没超长却被改了：%q", got)
	}
}

// 各渠道上限不同：飞书卡片超长会变成一堵墙（而且飞书对整个卡片 JSON 有大小限制），
// webhook 那头多半是程序消费，容忍度高。
func TestChannelBodyLimitsDiffer(t *testing.T) {
	if feishuBodyRunes >= webhookBodyRunes {
		t.Errorf("飞书的上限（%d）不该不低于 webhook（%d）——卡片是给人看的",
			feishuBodyRunes, webhookBodyRunes)
	}
	m := &MailData{From: "A", Subject: "S", Body: strings.Repeat("字", 50000)}
	if r := []rune(bodyFor(m, LevelFull, feishuBodyRunes)); len(r) > feishuBodyRunes+1 {
		t.Errorf("飞书正文没截到上限：%d", len(r))
	}
}

// ── 飞书卡片 ────────────────────────────────────────────────────────────────

// ⚠⚠ 这是这批改动里最要紧的一条：用户可控的内容绝不能进 lark_md。
//
// lark_md 会解析 markdown（`[文字](url)` 渲染成超链接），而主题与发件人完全由
// 发信方控制，飞书**不支持反斜杠转义**——没有办法把一段文本标成「按字面显示」。
// 一旦主题进了 lark_md 字段，一封主题为 `[点此查收](https://evil…)` 的邮件就能在
// 卡片里渲染出一条以假乱真的超链接，紧挨着我们自己的「打开邮件」按钮。
//
// 这和「主题里塞换行伪造链接行」是同一类问题，换了个渲染器而已。
func TestFeishuCardKeepsUserContentOutOfMarkdown(t *testing.T) {
	m := sampleMail()
	m.Subject = "[点此查收](https://evil.example.com)"
	m.From = "**Alice**"
	card := feishuCard(mailEvent(m), LevelSnippet, feishuBodyRunes)

	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("卡片序列化失败：%v", err)
	}
	// 遍历整张卡片：凡是 tag 为 lark_md 的节点，content 里都不该出现用户的内容
	for _, node := range collectTextNodes(t, card) {
		if node.tag != "lark_md" {
			continue
		}
		if strings.Contains(node.content, "evil.example.com") || strings.Contains(node.content, "Alice") {
			t.Errorf("用户内容落进了 lark_md 节点，可以伪造超链接：%q", node.content)
		}
	}
	// 内容本身还是要在的——不能靠「把主题丢掉」来通过上面那条
	if !strings.Contains(string(raw), "evil.example.com") {
		t.Error("主题整个不见了：应当按字面展示，而不是丢弃")
	}
}

// basic 档的卡片里不能出现正文元素。
func TestFeishuCardBasicHasNoBody(t *testing.T) {
	m := sampleMail()
	card := feishuCard(mailEvent(m), LevelBasic, feishuBodyRunes)
	raw, _ := json.Marshal(card)

	if strings.Contains(string(raw), "会议室 A") {
		t.Errorf("basic 档的卡片里出现了正文：%s", raw)
	}
	// 发件人和主题仍然要有，否则这一档就没用了
	if !strings.Contains(string(raw), "Alice") || !strings.Contains(string(raw), "会议变更") {
		t.Errorf("basic 档丢了发件人或主题：%s", raw)
	}
}

// 卡片的基本结构：标题栏 + 「打开邮件」按钮。
func TestFeishuCardStructure(t *testing.T) {
	card := feishuCard(mailEvent(sampleMail()), LevelSnippet, feishuBodyRunes)
	if card["msg_type"] != "interactive" {
		t.Fatalf("msg_type = %v", card["msg_type"])
	}
	inner, ok := card["card"].(map[string]any)
	if !ok {
		t.Fatalf("没有 card 字段")
	}
	header, _ := inner["header"].(map[string]any)
	if header == nil || header["template"] != "blue" {
		t.Errorf("新邮件的标题栏配色不对：%+v", header)
	}

	raw, _ := json.Marshal(card)
	if !strings.Contains(string(raw), `"tag":"button"`) {
		t.Errorf("没有「打开邮件」按钮：%s", raw)
	}
	if !strings.Contains(string(raw), "message=42") {
		t.Errorf("按钮没带上链接：%s", raw)
	}
}

// ⚠ 标题栏必须带上这封邮件自己的信息，不能是一个固定词。
//
// ── 缘起（用户看了真实卡片之后提的） ────────────────────────────────────────
//
// 标题栏是**唯一保证能被看到**的一行：飞书的会话列表预览、手机推送横幅、
// 免打扰时的角标提示，全都只取标题。放「新邮件」三个字等于把这块地方浪费掉——
// 用户必须点开卡片才知道是谁的什么事，而通知的意义恰恰是不点开就能判断。
//
// 这条要钉的是「标题随邮件变化」，而不是「标题等于某个字符串」：
// 后者换个写法（比如改成「新邮件 · 发件人」）也能过，但信息量还是不够。
func TestFeishuCardHeaderCarriesSubject(t *testing.T) {
	m := sampleMail()
	m.Subject = "季度复盘会议安排"
	title := cardHeaderTitle(mailEvent(m))

	if !strings.Contains(title, "季度复盘会议安排") {
		t.Errorf("标题栏没带主题：%q", title)
	}

	// 换一封邮件，标题必须跟着变——固定词的实现会在这里暴露
	m2 := sampleMail()
	m2.Subject = "发票已开具"
	if other := cardHeaderTitle(mailEvent(m2)); other == title {
		t.Errorf("两封不同主题的邮件拿到了同一个标题 %q，标题栏没有承载信息", title)
	}
}

// 主题已经上了标题栏，就不该在分栏里再列一遍。
//
// 卡片可见的行数很有限，重复一遍等于把两行里最值钱的那行浪费掉。
func TestFeishuCardDoesNotRepeatSubject(t *testing.T) {
	m := sampleMail()
	m.Subject = "季度复盘会议安排"
	card := feishuCard(mailEvent(m), LevelBasic, feishuBodyRunes)

	n := 0
	for _, node := range collectTextNodes(t, card) {
		if strings.Contains(node.content, "季度复盘会议安排") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("主题在卡片里出现了 %d 次，应当只在标题栏出现一次", n)
	}
}

// 标题栏放得下才行：飞书那一行超出会被直接截掉，连省略号都没有。
func TestFeishuCardHeaderTruncates(t *testing.T) {
	m := sampleMail()
	m.Subject = strings.Repeat("长", 200)
	title := cardHeaderTitle(mailEvent(m))

	if r := []rune(title); len(r) > cardHeaderTitleRunes+1 {
		t.Errorf("标题栏没截断，%d 个字符", len(r))
	}
	if !strings.HasSuffix(title, "…") {
		t.Errorf("截断了却没有省略号：%q", title)
	}
}

// 规则命中要说得出是哪条规则。
//
// 标题栏让给主题之后，「这条是我自己配的规则触发的」这个信息就没地方了——
// 不补的话用户分不出它和普通新邮件提醒的区别。
func TestFeishuCardRuleEventNamesTheRule(t *testing.T) {
	evt := mailEvent(sampleMail())
	evt.Type = EventMailRule
	evt.Title = "规则命中 · 老板来信"

	raw, _ := json.Marshal(feishuCard(evt, LevelBasic, feishuBodyRunes))
	if !strings.Contains(string(raw), "老板来信") {
		t.Errorf("规则命中的卡片没说是哪条规则：%s", raw)
	}
	// 配色也要和普通新邮件区分开
	card := feishuCard(evt, LevelBasic, feishuBodyRunes)["card"].(map[string]any)
	if h, _ := card["header"].(map[string]any); h["template"] == "blue" {
		t.Error("规则命中和普通新邮件用了同一个配色，看不出区别")
	}
}

// 非邮件事件没有主题，标题栏回落到事件源拼好的那一句——它自带账户名。
func TestFeishuCardNonMailHeader(t *testing.T) {
	evt := Event{Type: EventSyncFailed, Title: "同步失败 · 163", Body: "连接超时"}
	if got := cardHeaderTitle(evt); got != "同步失败 · 163" {
		t.Errorf("标题栏 = %q，想要带上账户名的那一句", got)
	}
	// 连标题都没有时至少说清是什么事件
	if got := cardHeaderTitle(Event{Type: EventSyncFailed}); got != "同步失败" {
		t.Errorf("兜底标题 = %q", got)
	}
}

// 没有链接时不要放一个点不动的按钮。
func TestFeishuCardWithoutURLHasNoButton(t *testing.T) {
	evt := mailEvent(sampleMail())
	evt.URL = ""
	raw, _ := json.Marshal(feishuCard(evt, LevelSnippet, feishuBodyRunes))
	if strings.Contains(string(raw), `"tag":"button"`) {
		t.Errorf("没配对外地址却放了按钮：%s", raw)
	}
}

// 故障类事件不受内容级别影响。
//
// 内容级别管的是**邮件正文**带多少。把同步失败的原因也一起掐掉的话，
// 配了基本信息的人就只能收到一句「同步失败」而不知道为什么。
func TestNonMailEventKeepsItsBodyAtAnyLevel(t *testing.T) {
	evt := Event{Type: EventSyncFailed, Title: "同步失败 · 163", Body: "连接超时"}
	for _, level := range []ContentLevel{LevelBasic, LevelSnippet, LevelFull} {
		raw, _ := json.Marshal(feishuCard(evt, level, feishuBodyRunes))
		if !strings.Contains(string(raw), "连接超时") {
			t.Errorf("%s 档把故障原因掐掉了：%s", level, raw)
		}
		if got := plainBody(evt, level, 1000); !strings.Contains(got, "连接超时") {
			t.Errorf("%s 档的纯文本丢了故障原因：%q", level, got)
		}
	}
	// 故障类事件的标题栏要显眼
	card := feishuCard(evt, LevelBasic, feishuBodyRunes)["card"].(map[string]any)
	if h, _ := card["header"].(map[string]any); h["template"] != "red" {
		t.Errorf("同步失败的标题栏配色 = %v，想要 red", h["template"])
	}
}

// ── 纯文本（webhook 的 body 字段） ──────────────────────────────────────────

func TestPlainBodyLevels(t *testing.T) {
	evt := mailEvent(sampleMail())

	basic := plainBody(evt, LevelBasic, 1000)
	if strings.Contains(basic, "会议室 A") {
		t.Errorf("basic 档漏出了正文：%q", basic)
	}
	if !strings.Contains(basic, "会议变更") {
		t.Errorf("basic 档应当保留主题：%q", basic)
	}
	if full := plainBody(evt, LevelFull, 1000); !strings.Contains(full, "上周的纪要") {
		t.Errorf("full 档没带全文：%q", full)
	}
}

// 主题要折叠成一行。
//
// ⚠ webhook 的 body 常被原样转发到别处显示（自建 bot、短信）。主题由发信方控制、
// MIME 编码字解得出换行，不折叠的话它能在转发出去的文本里伪造出独立的一行链接——
// 和飞书那边是同一类问题，只是渲染器换了。
func TestPlainBodyFoldsSubject(t *testing.T) {
	m := sampleMail()
	m.Subject = "中奖通知\nhttps://evil.example.com/claim"
	got := plainBody(mailEvent(m), LevelBasic, 1000)

	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "http") {
			t.Errorf("主题伪造出了一整行链接：%q\n完整正文：%q", line, got)
		}
	}
	// 主题内容本身还要在，不能靠丢弃来通过上面那条
	if !strings.Contains(got, "中奖通知") {
		t.Errorf("主题不见了：%q", got)
	}
}

// basic 档只有主题，不该多出空行或残留的正文分隔符。
func TestPlainBodyBasicIsJustSubject(t *testing.T) {
	got := plainBody(mailEvent(sampleMail()), LevelBasic, 1000)
	if got != "会议变更" {
		t.Errorf("basic 档的正文应当只有主题，拿到 %q", got)
	}
}

// ── 级别回落 ────────────────────────────────────────────────────────────────

// ⚠ 老渠道在库里是空串，必须回落到 snippet（= 历史行为）。
//
// 回落到别的档会在用户毫不知情的情况下改变他已有渠道的推送内容：
// 往上是突然把正文全文推进飞书群，往下是突然收不到摘要。
func TestChannelContentLevelFallback(t *testing.T) {
	if got := (&Channel{}).contentLevel(); got != LevelSnippet {
		t.Errorf("没配的渠道回落到 %q，想要 snippet（历史行为）", got)
	}
	if got := (&Channel{ContentLevel: "nonsense"}).contentLevel(); got != LevelSnippet {
		t.Errorf("非法值回落到 %q，想要 snippet", got)
	}
	if got := (&Channel{ContentLevel: "full"}).contentLevel(); got != LevelFull {
		t.Errorf("配了 full 却拿到 %q", got)
	}
}

// ── 辅助 ────────────────────────────────────────────────────────────────────

type textNode struct{ tag, content string }

// collectTextNodes 递归收集卡片里所有 {tag, content} 形状的节点。
func collectTextNodes(t *testing.T, v any) []textNode {
	t.Helper()
	var out []textNode
	switch node := v.(type) {
	case map[string]any:
		tag, hasTag := node["tag"].(string)
		content, hasContent := node["content"].(string)
		if hasTag && hasContent {
			out = append(out, textNode{tag: tag, content: content})
		}
		for _, child := range node {
			out = append(out, collectTextNodes(t, child)...)
		}
	case []map[string]any:
		for _, child := range node {
			out = append(out, collectTextNodes(t, child)...)
		}
	case []any:
		for _, child := range node {
			out = append(out, collectTextNodes(t, child)...)
		}
	}
	return out
}
