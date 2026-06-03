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
	switch ChannelKind(ch.Kind) {
	case KindFeishu:
		return sendFeishu(ch, evt)
	default:
		return sendWebhook(ch, evt)
	}
}

// sendWebhook 向通用 webhook POST 结构化 JSON；有 secret 时附 X-Webhook-Secret 头。
func sendWebhook(ch *Channel, evt Event) error {
	payload, _ := json.Marshal(map[string]any{
		"type":       string(evt.Type),
		"title":      evt.Title,
		"body":       evt.Body,
		"account_id": evt.AccountID,
		"time":       time.Now().Format(time.RFC3339),
	})
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

// sendFeishu 向飞书自定义机器人发送文本消息；有 secret 时按飞书规则做时间戳签名。
func sendFeishu(ch *Channel, evt Event) error {
	text := evt.Title
	if evt.Body != "" {
		text += "\n" + evt.Body
	}
	body := map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": text},
	}
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
