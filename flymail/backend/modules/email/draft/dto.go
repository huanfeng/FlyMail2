package draft

import "strings"

// DraftRequest 创建/更新草稿的请求体。
type DraftRequest struct {
	AccountID  uint     `json:"account_id"`
	To         []string `json:"to"`
	Cc         []string `json:"cc"`
	Bcc        []string `json:"bcc"`
	Subject    string   `json:"subject"`
	BodyHTML   string   `json:"body_html"`
	InReplyTo  string   `json:"in_reply_to"`
	References string   `json:"references"`
}

// DraftResponse 草稿响应体（收件人还原为数组）。
type DraftResponse struct {
	ID         uint     `json:"id"`
	AccountID  uint     `json:"account_id"`
	To         []string `json:"to"`
	Cc         []string `json:"cc"`
	Bcc        []string `json:"bcc"`
	Subject    string   `json:"subject"`
	BodyHTML   string   `json:"body_html"`
	InReplyTo  string   `json:"in_reply_to"`
	References string   `json:"references"`
}

// splitAddrs 将逗号分隔的地址字符串拆分为切片；空字符串返回空切片。
func splitAddrs(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// joinAddrs 将地址切片合并为逗号分隔字符串。
func joinAddrs(addrs []string) string {
	return strings.Join(addrs, ",")
}

// toResponse 将 Draft 模型转换为 DraftResponse。
func toResponse(d *Draft) *DraftResponse {
	return &DraftResponse{
		ID:         d.ID,
		AccountID:  d.AccountID,
		To:         splitAddrs(d.ToStr),
		Cc:         splitAddrs(d.CcStr),
		Bcc:        splitAddrs(d.BccStr),
		Subject:    d.Subject,
		BodyHTML:   d.BodyHTML,
		InReplyTo:  d.InReplyTo,
		References: d.References,
	}
}
