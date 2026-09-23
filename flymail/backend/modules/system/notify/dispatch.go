package notify

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// httpClient 带超时，避免外发卡住投递 worker。
var httpClient = &http.Client{Timeout: 10 * time.Second}

// dispatch 按渠道类型把事件发往外部，返回错误（nil 表示成功）。
func dispatch(ch *Channel, evt Event) error {
	level := ch.contentLevel()
	switch ChannelKind(ch.Kind) {
	case KindFeishu:
		return sendFeishu(ch, evt, level)
	default:
		return sendWebhook(ch, evt, level)
	}
}

// sendWebhook 向通用 webhook POST 结构化 JSON；有 secret 时附 X-Webhook-Secret 头。
func sendWebhook(ch *Channel, evt Event, level ContentLevel) error {
	out := map[string]any{
		"type":       string(evt.Type),
		"title":      evt.Title,
		"body":       plainBody(evt, level, webhookBodyRunes),
		"account_id": evt.AccountID,
		"message_id": evt.MessageID,
		"url":        evt.URL,
		// 告诉消费者这条是按哪一档发的——收到没有正文的通知时，
		// 才分得清是「这封邮件没正文」还是「这个渠道配的是基本信息」。
		"content_level": string(level),
		"time":          time.Now().Format(time.RFC3339),
	}
	// 结构化字段：webhook 那头多半是程序在消费，给它字段比给它一段拼好的文本好用。
	// 按级别裁剪后再给，basic 档不会从这里泄出正文。
	if from, subject, ok := mailFields(evt.Mail); ok {
		mail := map[string]any{"from": from, "subject": subject}
		if !evt.Mail.Date.IsZero() {
			mail["date"] = evt.Mail.Date.Format(time.RFC3339)
		}
		if body := bodyFor(evt.Mail, level, webhookBodyRunes); body != "" {
			mail["body"] = body
		}
		// 链接单独给一份：body 是已经剥成纯文本的正文，<a href> 的地址在那一步
		// 就没了，消费方从 body 里再也解不出来。走与正文同一道级别闸门——
		// basic 档一个字正文都不带，重置链接之类自然也不能漏。
		if level.wantsBody() && len(evt.Mail.Links) > 0 {
			mail["links"] = evt.Mail.Links
		}
		out["mail"] = mail
	}
	payload, _ := json.Marshal(out)
	req, err := http.NewRequest(http.MethodPost, ch.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if ch.Secret != "" {
		req.Header.Set("X-Webhook-Secret", ch.Secret)
	}
	return doRequest(req)
}

// plainBody 把事件拼成一段可读的纯文本，用于 webhook 载荷里的 body 字段。
//
// webhook 那头常常是把 body 原样转发到别处显示（自建 bot、短信、日志），
// 所以它得是「人能直接看懂的一段」，而不是需要再拼一次的碎片。
//
// ⚠ Title 在这里折叠成一行是出口防线：它带着发件人名，而发件人由对方控制。
// 构造侧已经折叠过（notify.OneLine），这里兜住将来新增的事件源——Title 在
// 语义上永远是一行，折叠不会误伤。正文不能这样折叠：它的换行是有意义的。
func plainBody(evt Event, level ContentLevel, limit int) string {
	_, subject, ok := mailFields(evt.Mail)
	if !ok {
		// 非邮件事件（同步失败、账户状态）：内容级别管的是邮件正文，
		// 不该把故障原因也一起掐掉——否则配了基本信息的人只能收到一句
		// 「同步失败」而不知道为什么。
		return truncateRunes(evt.Body, limit)
	}
	// ⚠ 不重复 Title：载荷里已经有独立的 title 字段（「新邮件 · Alice」），
	// 再拼一遍只会让转发出去的文本多出一行。这里只负责「正文那部分」。
	text := subject
	if body := bodyFor(evt.Mail, level, limit); body != "" {
		text += "\n" + body
	}
	return text
}

// sendFeishu 向飞书自定义机器人发送消息卡片；有 secret 时按飞书规则做时间戳签名。
//
// 签名字段挂在**最外层**（与 msg_type / card 平级），所以渲染与签名分开：
// 渲染只管消息体长什么样，签名在这里补。
func sendFeishu(ch *Channel, evt Event, level ContentLevel) error {
	body := fitFeishuCard(evt, level)
	if ch.Secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		sign, err := feishuSign(ts, ch.Secret)
		if err != nil {
			return err
		}
		body["timestamp"] = ts
		body["sign"] = sign
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, ch.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doRequest(req)
}

// feishuSign 飞书签名：HMAC-SHA256(key = "{timestamp}\n{secret}", data 为空) 再 base64。
func feishuSign(timestamp, secret string) (string, error) {
	key := timestamp + "\n" + secret
	h := hmac.New(sha256.New, []byte(key))
	if _, err := h.Write([]byte{}); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}

// doRequest 发送并把非 2xx 视为失败（读取少量响应体用于报错）。
func doRequest(req *http.Request) error {
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(snippet))
	}
	return nil
}
