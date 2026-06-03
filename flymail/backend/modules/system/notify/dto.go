package notify

import "strings"

// Event 是一次通知触发的数据载体（由各事件源经 emit 回调传入）。
type Event struct {
	Type      EventType
	AccountID uint
	Title     string
	Body      string
}

// ChannelInput 是创建/更新渠道的入参。
type ChannelInput struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`  // 留空表示更新时不改密钥
	Events  []string `json:"events"`  // 订阅的事件类型
	Enabled *bool    `json:"enabled"` // 指针以区分未传
}

// ChannelDTO 是渠道的对外表示（不含密钥明文，含 has_secret 与 events 数组）。
type ChannelDTO struct {
	ID        uint     `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	URL       string   `json:"url"`
	HasSecret bool     `json:"has_secret"`
	Events    []string `json:"events"`
	Enabled   bool     `json:"enabled"`
	CreatedAt string   `json:"created_at"`
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
		CreatedAt: c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
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
