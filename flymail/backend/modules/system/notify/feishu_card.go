package notify

// 飞书消息卡片（msg_type = interactive）。
//
// 比纯文本多出来的是：标题栏能按事件类型配色、发件人/主题分栏、以及一个真正的
// 「打开邮件」按钮——纯文本那边只能把 URL 单独放一行，指望聊天客户端把它识别成
// 可点区域。
//
// ⚠⚠ 这里有一条必须守住的安全边界：**用户可控的内容一律用 plain_text，
// 绝不放进 lark_md**。
//
// lark_md 会解析 markdown：`**x**` 加粗、`[文字](url)` 变成超链接。而主题和
// 发件人完全由发信方控制，飞书又**不支持反斜杠转义**（没有办法把一段文本标成
// 「按字面显示」）。一旦把主题塞进 lark_md 字段，一封主题为
// `[点此查收](https://evil.example.com)` 的邮件就能在卡片里渲染出一条以假乱真的
// 超链接，而且它紧挨着我们自己的「打开邮件」按钮，可信度比纯文本里的假链接更高。
//
// 这和之前那条「主题里塞换行伪造链接行」是同一类问题，换了个渲染器而已：
// 只要某种标记语言由用户内容驱动，就得假设它会被用来伪造我们自己的 UI 元素。
// 固定文案（字段标签之类）是我们自己写的，用 lark_md 无妨；值不行。

// 标题栏配色：一眼区分这条通知要不要现在处理。
func cardHeaderTemplate(t EventType) string {
	switch t {
	case EventSyncFailed:
		return "red"
	case EventAccountStatus:
		return "orange"
	case EventMailRule:
		return "turquoise"
	default:
		return "blue"
	}
}

// cardHeaderTitleRunes 是标题栏的字符上限。
//
// 飞书的标题栏就一行，超出直接截掉（连省略号都没有）。主题动辄很长，
// 自己截至少能留个「后面还有」的记号。
const cardHeaderTitleRunes = 36

// cardHeaderTitle 决定标题栏那一行放什么。
//
// ⚠ 这一行是**唯一保证能被看到**的内容：飞书的会话列表预览、手机推送横幅、
// 消息免打扰时的角标提示，都只取标题。放一个固定的「新邮件」等于把这块地方
// 浪费掉——用户得点开卡片才知道是谁的什么事。
//
// 所以邮件类事件放**主题**：它是「这封邮件是什么」的答案，信息密度最高。
// 发件人和时间移到卡片正文的分栏里，那里本来就看得见。
// 事件类型由标题栏配色承担，不必再占用文字。
//
// 非邮件事件（同步失败、账户状态）没有主题，用事件源拼好的标题——
// 它已经带上了账户名（「同步失败 · 163」），本身就是有信息的。
func cardHeaderTitle(evt Event) string {
	if _, subject, ok := mailFields(evt.Mail); ok {
		return truncateRunes(subject, cardHeaderTitleRunes)
	}
	if title := OneLine(evt.Title); title != "" {
		return truncateRunes(title, cardHeaderTitleRunes)
	}
	return fallbackTitle(evt.Type)
}

// fallbackTitle 是连标题都没有时的兜底，至少说清这是什么事件。
func fallbackTitle(t EventType) string {
	switch t {
	case EventSyncFailed:
		return "同步失败"
	case EventAccountStatus:
		return "账户状态变化"
	case EventMailRule:
		return "规则命中"
	default:
		return "新邮件"
	}
}

// plainText 构造一个纯文本节点。所有用户可控的值都走这里。
func plainText(content string) map[string]any {
	return map[string]any{"tag": "plain_text", "content": content}
}

// feishuCard 把事件渲染成卡片消息体。
func feishuCard(evt Event, level ContentLevel) map[string]any {
	elements := make([]map[string]any, 0, 4)

	from, _, ok := mailFields(evt.Mail)
	if ok {
		// 两栏并排。标签和值放在同一个 plain_text 里用换行分隔——把标签单独做成
		// lark_md 节点会好看一点，但那样值就得挨着它进同一个字符串，前功尽弃。
		//
		// 主题不在这里：它已经上了标题栏，再列一遍是把两行里最值钱的那行浪费掉。
		fields := make([]map[string]any, 0, 3)
		if from != "" {
			fields = append(fields, map[string]any{"is_short": true, "text": plainText("发件人\n" + from)})
		}
		if !evt.Mail.Date.IsZero() {
			fields = append(fields, map[string]any{
				"is_short": true,
				"text":     plainText("时间\n" + evt.Mail.Date.Local().Format("01-02 15:04")),
			})
		}
		// 规则命中的「哪条规则」只在事件源拼的标题里有，而标题栏现在让给了主题。
		// 不说清楚的话，用户分不出这条是新邮件提醒还是自己配的规则触发的。
		if evt.Type == EventMailRule {
			if title := OneLine(evt.Title); title != "" {
				fields = append(fields, map[string]any{"is_short": false, "text": plainText("触发\n" + title)})
			}
		}
		if len(fields) > 0 {
			elements = append(elements, map[string]any{"tag": "div", "fields": fields})
		}
	}

	if body := cardBody(evt, level, ok); body != "" {
		elements = append(elements, map[string]any{"tag": "hr"})
		elements = append(elements, map[string]any{"tag": "div", "text": plainText(body)})
	}

	if evt.URL != "" {
		elements = append(elements, map[string]any{
			"tag": "action",
			"actions": []map[string]any{{
				"tag":  "button",
				"text": plainText(cardButtonLabel(evt)),
				// 这个 URL 是我们自己按已校验的对外地址拼的，不是用户内容
				"url":  evt.URL,
				"type": "primary",
			}},
		})
	}

	return map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"config": map[string]any{"wide_screen_mode": true},
			"header": map[string]any{
				"title":    plainText(cardHeaderTitle(evt)),
				"template": cardHeaderTemplate(evt.Type),
			},
			"elements": elements,
		},
	}
}

// cardBody 决定卡片正文那一段放什么。
//
// 有结构化字段时按内容级别取（basic 档就是不放）；没有时（非邮件事件）
// 回落到事件自带的 Body，那是同步失败原因之类的固定文案，不受级别影响——
// 内容级别管的是**邮件正文**带多少，不该把故障原因也一起掐掉。
func cardBody(evt Event, level ContentLevel, structured bool) string {
	if structured {
		return bodyFor(evt.Mail, level, feishuBodyRunes)
	}
	return truncateRunes(evt.Body, feishuBodyRunes)
}

func cardButtonLabel(evt Event) string {
	if evt.MessageID != 0 {
		return "打开邮件"
	}
	return "打开 FlyMail"
}
