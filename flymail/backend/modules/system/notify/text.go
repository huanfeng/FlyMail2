package notify

import "strings"

// SnippetRunes 是通知正文里摘要部分的字符上限。
//
// 库里的 snippet 是 150 字，而通知要经过飞书气泡、系统横幅这些窄容器。太长的话
// 它们会按自己的规则乱截（有的直接砍掉整段），不如我们截在一个看得出
// 「后面还有」的位置。
const SnippetRunes = 80

// OneLine 把字符串里的所有空白折叠成单个空格。
//
// ⚠ 这不是排版，是一道安全边界。
//
// 通知的纯文本形态（飞书、系统横幅）靠**换行**区分字段，而 feishuText 更进一步：
// 它把「独占一行的裸 URL」约定成官方的「打开邮件」链接，因为聊天客户端正是靠
// 整行是 URL 来识别可点区域的。
//
// 主题和发件人**完全由发件人控制**，而 MIME 编码字解出来是可以带换行的
// （`=?utf-8?B?...?=` 里塞 \n 就行）。不折叠的话，一封主题为
// `会议变更\nhttps://evil.example.com/x` 的邮件，在飞书里长这样：
//
//	新邮件 · Alice
//	会议变更
//	https://evil.example.com/x   ← 用户眼里这就是「打开邮件」
//	下周三上午十点…
//	https://mail.example.com/?…  ← 真链接反而在下面
//
// JSON 那一路是安全的（json.Marshal 会转义），所以这纯粹是渲染后的视觉欺骗，
// 但成功率不低——链接行刚好是我们自己刚建立起来的那个「可信元素」。
//
// 折叠在**构造**时做而不是在出口做：Body 里的换行是我们自己有意拼进去的
// （主题一行、摘要一行），出口无差别折叠会把它一起抹掉。
func OneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// MailBody 拼邮件类通知的正文：主题一行，正文摘要一行。
//
// 只给主题的话，很多通知（验证码、工单、告警、会议变更）光看标题根本判断不出
// 要不要现在处理，还得点进去看一眼。摘要那一行就是为了让人在通知里直接做这个判断。
//
// 放在 notify 包是因为它是「通知长什么样」的一部分：新邮件与规则命中两条路径
// 都要用，格式不一致的话同一个通知中心里会出现两种样子，看起来像功能时灵时不灵。
func MailBody(subject, snippet string) string {
	subject = OneLine(subject)
	snippet = OneLine(snippet)
	if snippet == "" {
		return subject
	}
	if r := []rune(snippet); len(r) > SnippetRunes {
		snippet = string(r[:SnippetRunes]) + "…"
	}
	return subject + "\n" + snippet
}
