package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// Kind 是一次调用失败的类别，决定「换下一个配置」之后这一个要冷却多久。
type Kind string

const (
	KindQuota         Kind = "quota"      // 余额/配额耗尽
	KindAuth          Kind = "auth"       // 密钥被拒
	KindRateLimit     Kind = "rate_limit" // 限流
	KindUpstream      Kind = "upstream"   // 5xx、连不上、超时、响应不合法
	KindRejected      Kind = "rejected"   // 400/404：这家不认这个模型或请求
	KindTruncated     Kind = "truncated"  // 输出被长度上限截断
	KindBadOutput     Kind = "bad_output" // 回了话，但一段可用译文都没有
	KindCanceled      Kind = "canceled"   // 调用方取消（用户关掉了邮件）
	KindNotConfigured Kind = "not_configured"
)

// ErrBadOutput 表示模型回了话，但内容不可用（不按格式、全是废话）。
//
// 这不是服务商坏了，换一个模型往往就好，所以单列一类：切换，但不冷却。
var ErrBadOutput = errors.New("AI 没有返回可用的译文")

// quotaCodes 是上游错误码里明确表示「余额/配额用完」的取值。错误码是机器可读的，
// 命中就不必再猜消息。
var quotaCodes = map[string]bool{
	"insufficient_quota":         true, // OpenAI
	"insufficient_balance":       true,
	"billing_hard_limit_reached": true, // OpenAI 账单硬上限
	"arrearage":                  true, // 阿里云百炼：欠费
}

// quotaPhrases 是消息里表示「余额/配额用完」的**完整短语**。
//
// ⚠ 不能用 billing、credit、balance、额度、配额 这类单词做子串匹配：
// OpenAI 未绑卡账户的**普通限流**消息末尾就带一句
// "Visit https://platform.openai.com/account/billing to add a payment method"，
// 国内几家的 TPM/RPM 限流消息里也常写「额度超限」。按单词匹配会把一次
// 该等 20 秒的限流判成余额不足、冷却一小时，还会把用户引去设置页查账单。
var quotaPhrases = []string{
	"exceeded your current quota",
	"insufficient quota", "insufficient_quota",
	"insufficient balance", "insufficient_balance",
	"credit balance is too low",
	"billing not active", "billing_not_active",
	"余额不足", "欠费", "账户已欠费",
}

// rateLimitMarkers 是限流消息的特征。429 上一旦出现这些，就按限流处理，
// 不再去看余额短语——限流消息里顺带提一句账单页面是常态。
var rateLimitMarkers = []string{
	"rate limit", "rate_limit", "ratelimit", "requests per", "tokens per",
	"rpm", "tpm", "try again in", "too many requests", "请求过于频繁", "频率",
}

func containsAny(text string, subs []string) bool {
	for _, m := range subs {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}

func isQuota(e *APIError) bool {
	if e.Status == http.StatusPaymentRequired {
		return true
	}
	if quotaCodes[strings.ToLower(e.Code)] {
		return true
	}
	// 只在 429/403/400 以及「错误塞在 2xx 响应体里」时认消息短语：
	// 500 的消息里出现这些词多半是别的意思。
	is2xx := e.Status >= 200 && e.Status < 300
	if e.Status != http.StatusTooManyRequests && e.Status != http.StatusForbidden &&
		e.Status != http.StatusBadRequest && !is2xx {
		return false
	}
	text := strings.ToLower(e.Code + " " + e.Msg)
	if e.Status == http.StatusTooManyRequests && (e.RetryAfter > 0 || containsAny(text, rateLimitMarkers)) {
		return false
	}
	return containsAny(text, quotaPhrases)
}

// Classify 把一次调用的错误归类。
func Classify(err error) Kind {
	if errors.Is(err, context.Canceled) {
		return KindCanceled
	}
	if errors.Is(err, ErrNotConfigured) {
		return KindNotConfigured
	}
	if errors.Is(err, ErrTruncated) {
		return KindTruncated
	}
	if errors.Is(err, ErrBadOutput) {
		return KindBadOutput
	}
	if apiErr, ok := asAPIError(err); ok {
		switch {
		case isQuota(apiErr):
			return KindQuota
		case apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden:
			return KindAuth
		case apiErr.Status == http.StatusTooManyRequests:
			return KindRateLimit
		case apiErr.Status >= 500:
			return KindUpstream
		case apiErr.Status >= 200 && apiErr.Status < 300:
			// 错误塞在 200 响应体里：多是网关的临时错误（overloaded 之类），
			// 不能按「这家不认这个模型」冷却十分钟。
			return KindUpstream
		default:
			return KindRejected
		}
	}
	// 连接被拒、DNS 失败、超时（含 context.DeadlineExceeded）、JSON 解析失败……
	// 都是「这家此刻用不了」。
	return KindUpstream
}

func asAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	ok := errors.As(err, &apiErr)
	return apiErr, ok
}
