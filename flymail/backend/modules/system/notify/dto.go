package notify

import (
	"strings"
	"time"
)

// Event 是一次通知触发的数据载体（由各事件源经 emit 回调传入）。
// MessageID 仅单封新邮件事件非 0（供前端精准跳转），其余事件为 0。
type Event struct {
	Type      EventType
	AccountID uint
	MessageID uint
	Title     string
	Body      string
	// URL 是这条通知对应的「打开」链接，由 Service 在 Emit 时按对外访问地址拼出，
	// 事件源不必知道它。没配对外访问地址时为空。
	URL string
	// Mail 是邮件类事件的结构化内容，由 Service 在投递前按需补齐。
	//
	// ⚠ 有了它，Title/Body 就只是**站内通知**和纯文本回退用的成品字符串了。
	// 外发渠道要按自己那一档决定带多少内容、按自己的排版重新组织，拿成品字符串
	// 是做不到的——这也是模板功能的前提：模板得有字段才能填。
	Mail *MailData
}

// MailData 是邮件类事件的结构化内容。
//
// Body（全文）只有在**确实有渠道要全文**时才会被填上：取全文要多查一次
// message_bodies，而绝大多数渠道停在 snippet 档，没必要为它们付这个代价。
type MailData struct {
	From    string
	Subject string
	Date    time.Time
	Snippet string
	Body    string
	// Links 是正文里的 http/https 链接，已去重并限量。
	//
	// 单独带出来而不是让渲染层从 Body 里扫：Body 是已经降级过的纯文本，
	// <a href="…">点这里</a> 到这一步只剩「点这里」，地址早没了。
	Links []string
}

// ChannelInput 是创建/更新渠道的入参。
type ChannelInput struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`  // 留空表示更新时不改密钥
	Events  []string `json:"events"`  // 订阅的事件类型
	Enabled *bool    `json:"enabled"` // 指针以区分未传
	// ContentLevel 带多少邮件内容（basic / snippet / full）；留空表示按默认。
	ContentLevel string `json:"content_level"`
}

// ChannelDTO 是渠道的对外表示（不含密钥明文，含 has_secret 与 events 数组）。
type ChannelDTO struct {
	ID           uint     `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	URL          string   `json:"url"`
	HasSecret    bool     `json:"has_secret"`
	Events       []string `json:"events"`
	Enabled      bool     `json:"enabled"`
	ContentLevel string   `json:"content_level"`
	CreatedAt    string   `json:"created_at"`
}

func splitEvents(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return []string{}
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func joinEvents(events []string) string {
	valid := make([]string, 0, len(events))
	for _, e := range events {
		if ValidEvent(e) {
			valid = append(valid, e)
		}
	}
	return strings.Join(valid, ",")
}

func toChannelDTO(c *Channel) ChannelDTO {
	return ChannelDTO{
		ID:        c.ID,
		Name:      c.Name,
		Kind:      c.Kind,
		URL:       c.URL,
		HasSecret: c.Secret != "",
		Events:    splitEvents(c.Events),
		Enabled:   c.Enabled,
		// 回落到默认，前端拿到的永远是三档之一而不是空串——
		// 否则老渠道在界面上会显示成「没选」，用户一保存就真的变了。
		ContentLevel: string(c.contentLevel()),
		CreatedAt:    c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// subscribes 判断渠道是否订阅了某事件类型。
func (c *Channel) subscribes(t EventType) bool {
	for _, e := range splitEvents(c.Events) {
		if EventType(e) == t {
			return true
		}
	}
	return false
}
