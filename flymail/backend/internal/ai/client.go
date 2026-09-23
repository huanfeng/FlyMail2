// Package ai 是 OpenAI 兼容 Chat Completions 接口的最小客户端。
//
// ── 为什么只认 OpenAI 兼容这一种形状 ───────────────────────────────────────
//
// 它已经是事实标准：OpenAI、DeepSeek、智谱、月之暗面、硅基流动、OpenRouter，
// 以及本地跑的 Ollama / LM Studio / vLLM，全都提供 /chat/completions。
// 用户只要填三样东西——地址、密钥、模型名——就能接上其中任何一个。
// 为 Anthropic / Gemini 各写一套原生协议，换来的只是少填一个 base_url，
// 却要多维护两份请求构造与两份错误分类。
//
// ── 这里刻意不做的事 ───────────────────────────────────────────────────────
//
// 不发 temperature / max_tokens。新一代推理模型（o 系列、gpt-5 系列）对这两个
// 参数要么不接受、要么语义变了，发过去直接 400；而它们的默认值对翻译足够好。
// 少发一个参数，就少一类"换个模型就报错"的故障。
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrNotConfigured 表示 AI 接口尚未配置（地址或模型为空）。
// 调用方据此给出"去设置页配置"的提示，而不是一句网络错误。
var ErrNotConfigured = errors.New("未配置 AI 接口")

// APIError 是上游返回的非 2xx。Status 保留原始状态码，让调用方能区分
// "密钥错了"（401，重试无用）与"限流"（429，稍后可再试）。
type APIError struct {
	Status int
	Msg    string
}

func (e *APIError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "AI 接口拒绝了密钥（" + strconv.Itoa(e.Status) + "）：" + e.Msg
	case http.StatusTooManyRequests:
		return "AI 接口限流（429），请稍后再试：" + e.Msg
	case http.StatusNotFound:
		return "AI 接口地址或模型不存在（404）：" + e.Msg
	}
	return fmt.Sprintf("AI 接口返回 %d：%s", e.Status, e.Msg)
}

// Retryable 报告这个错误是否值得重试。
func (e *APIError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// ErrTruncated 表示模型在输出中途被 max_tokens 截断。
//
// 必须当错误报出去：翻译被截断的表现是"译文只有前半篇"，而前半篇看起来
// 完全正常，用户不会意识到后面丢了。
var ErrTruncated = errors.New("AI 返回被长度上限截断")

// Config 是一次调用所需的全部配置。
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	// Timeout 为零时用 DefaultTimeout。
	Timeout time.Duration
}

// DefaultTimeout 是单次请求的超时。
//
// 两分钟不是网络意义上的超时，是"模型吐完一批译文要多久"：本地 Ollama 跑
// 7B 模型翻一屏文字要几十秒，云端大模型排队时也能到这个量级。
const DefaultTimeout = 120 * time.Second

// Message 是一条对话消息。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Client 调用 OpenAI 兼容接口。零值不可用，用 New 构造。
type Client struct {
	endpoint string
	apiKey   string
	model    string
	hc       *http.Client
}

// New 构造客户端。地址或模型为空时返回 ErrNotConfigured。
func New(cfg Config) (*Client, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	model := strings.TrimSpace(cfg.Model)
	if base == "" || model == "" {
		return nil, ErrNotConfigured
	}
	endpoint, err := Endpoint(base)
	if err != nil {
		return nil, err
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Client{
		endpoint: endpoint,
		apiKey:   strings.TrimSpace(cfg.APIKey),
		model:    model,
		hc:       &http.Client{Timeout: timeout},
	}, nil
}

// Model 返回本次调用使用的模型名（供调用方记录进缓存行）。
func (c *Client) Model() string { return c.model }

// reVersionSeg 匹配 /v1、/v4 这类版本段。
var reVersionSeg = regexp.MustCompile(`^v\d+$`)

// Endpoint 把用户填的 base_url 归一成 chat/completions 的完整地址。
//
// 用户会填进来的四种形状都要认：
//
//	https://api.openai.com                     → 补 /v1/chat/completions
//	https://api.openai.com/v1                  → 补 /chat/completions
//	https://open.bigmodel.cn/api/paas/v4       → 补 /chat/completions（版本段不都叫 v1）
//	https://xxx/v1/chat/completions            → 原样（网关/代理可能是别的路径形状）
//
// 认不出来就当成"根地址"补全整段——猜错了用户会立刻看到 404，
// 而不是一个更难懂的 JSON 解析失败。
func Endpoint(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", ErrNotConfigured
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return "", errors.New("AI 接口地址必须以 http:// 或 https:// 开头")
	}
	trimmed := strings.TrimRight(base, "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed, nil
	}
	segs := strings.Split(trimmed, "/")
	if len(segs) > 0 && reVersionSeg.MatchString(segs[len(segs)-1]) {
		return trimmed + "/chat/completions", nil
	}
	return trimmed + "/v1/chat/completions", nil
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// maxErrBody 限制读取错误响应体的长度：网关出错时回的可能是整页 HTML，
// 原样塞进错误消息会把日志和界面都刷爆。
const maxErrBody = 2 << 10

// Chat 发一次对话请求，返回首个 choice 的内容。
func (c *Client) Chat(ctx context.Context, msgs []Message) (string, error) {
	payload, err := json.Marshal(chatRequest{Model: c.model, Messages: msgs})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	// 本地模型（Ollama / LM Studio）通常不需要密钥，留空就不发这个头——
	// 发一个 "Bearer " 空值反而会被某些服务判成非法凭据。
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBody))
		return "", &APIError{Status: resp.StatusCode, Msg: errMessage(snippet)}
	}

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("AI 返回的不是合法 JSON：%w", err)
	}
	// 有些服务把错误塞进 200 响应体里。
	if out.Error != nil && out.Error.Message != "" {
		return "", &APIError{Status: resp.StatusCode, Msg: out.Error.Message}
	}
	if len(out.Choices) == 0 {
		return "", errors.New("AI 没有返回任何内容")
	}
	if out.Choices[0].FinishReason == "length" {
		return "", ErrTruncated
	}
	return out.Choices[0].Message.Content, nil
}

// errMessage 从错误响应体里挑出人能看懂的一句。
//
// OpenAI 兼容服务的错误形状是 {"error":{"message":"..."}}，但网关、反向代理
// 和本地服务各有各的写法，解析不出来就退回原始文本（已限长）。
func errMessage(body []byte) string {
	var wrapped struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &wrapped) == nil {
		if wrapped.Error.Message != "" {
			return wrapped.Error.Message
		}
		if wrapped.Message != "" {
			return wrapped.Message
		}
	}
	return strings.TrimSpace(string(body))
}
