package notify

import (
	"strings"
	"sync"
)

// 外发消息的排版。
//
// ── 为什么排版要推迟到这一层 ─────────────────────────────────────────────────
//
// 事件源（sync / rule）只管「发生了什么」，不该知道飞书长什么样、webhook 要哪些
// 字段、这个渠道配的是摘要还是全文。Event 因此带的是结构化字段（MailData），
// 由这里按 (渠道类型, 内容级别) 组织成各自的形状。
//
// 模板功能将来接在同一个位置：Channel.Template 非空时换一套渲染，而不必再动
// 事件源和投递层。内置排版就是「模板为空」时的那一套。

// 各渠道的正文上限（字符数）。
//
// 全文动辄几十上百 KB，直接塞过去的后果各不相同：飞书卡片会变成一堵墙（而且
// 飞书对整个卡片 JSON 有大小限制，超了整条发不出去），webhook 那边多半是程序
// 在消费，容忍度高但也不该无限。
const (
	// feishuBodyRunes 是飞书正文的默认字符上限（用户未配置时）。
	//
	// 这个数字只是「排版上想放多少」，不是安全上限——真正兜底的是
	// feishuPayloadBudget 那道按字节算的裁剪（见 fitFeishuCard）。
	// 按字符算永远不可能准：中文一个字 UTF-8 占 3 字节，JSON 转义还会再膨胀。
	feishuBodyRunes  = 8000
	webhookBodyRunes = 20000
)

// feishuPayloadBudget 是整个卡片 JSON 的字节预算。
//
// 飞书对卡片消息请求体的硬限制是 30KB，超了整条发不出去（而且是静默失败的
// 那种失败——用户只会发现"有几封邮件没推送"）。留 4KB 余量给签名字段、
// 标题栏、按钮，以及我们没预料到的转义膨胀。
const feishuPayloadBudget = 26 * 1024

// bodyRunesProvider 供 app 注入「当前配置的正文字符上限」。
//
// 取的是函数而不是一个值：设置页改完要立刻生效，存值就得再搭一套变更通知。
// 这也是仓里既有的做法（SetSyncDepthProvider、SetPollIntervalProvider）。
//
// 不塞进 Event 或 Channel：它是**部署级的排版偏好**，既不属于"发生了什么"
// （Event），也不属于"发去哪里"（Channel）。
// 投递 worker 与设置页改配置分属不同 goroutine，所以要加锁。
var (
	bodyRunesMu       sync.RWMutex
	bodyRunesProvider func() int
)

// SetBodyRunesProvider 注入正文字符上限的取数函数；返回 <= 0 表示用默认值。
func SetBodyRunesProvider(fn func() int) {
	bodyRunesMu.Lock()
	defer bodyRunesMu.Unlock()
	bodyRunesProvider = fn
}

// configuredBodyRunes 返回当前生效的飞书正文字符上限。
func configuredBodyRunes() int {
	bodyRunesMu.RLock()
	fn := bodyRunesProvider
	bodyRunesMu.RUnlock()
	if fn == nil {
		return feishuBodyRunes
	}
	if n := fn(); n > 0 {
		return n
	}
	return feishuBodyRunes
}

// bodyFor 按内容级别取出该带的正文，并截到上限。
//
// basic 档返回空串——它的定义就是「一个字正文都不带」，这是配了它的人唯一在意
// 的事（验证码、密码重置这类邮件的正文不该进第三方聊天记录）。
func bodyFor(mail *MailData, level ContentLevel, limit int) string {
	if mail == nil || !level.wantsBody() {
		return ""
	}
	text := mail.Snippet
	if level == LevelFull && mail.Body != "" {
		text = mail.Body
	}
	return truncateRunes(strings.TrimSpace(text), limit)
}

// truncateRunes 按字符（不是字节）截断，截掉了就留个省略号。
//
// 按字节截会把多字节字符劈成两半，中文邮件必然踩到。
func truncateRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "…"
}

// mailFields 返回卡片/正文里要展示的「发件人 / 主题」两项。
//
// 取自结构化字段而不是从 Title/Body 里回解：Title 是拼好的成品
// （「新邮件 · Alice」），拆它既脆弱又会把事件源的排版决定带进来。
// 没有 MailData 时（非邮件事件、或取数器没装配）返回空，调用方回落到 Title/Body。
func mailFields(mail *MailData) (from, subject string, ok bool) {
	if mail == nil {
		return "", "", false
	}
	from = OneLine(mail.From)
	subject = OneLine(mail.Subject)
	if from == "" && subject == "" {
		return "", "", false
	}
	if subject == "" {
		subject = "（无主题）"
	}
	return from, subject, true
}
