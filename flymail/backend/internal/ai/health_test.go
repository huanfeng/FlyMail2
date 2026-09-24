package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want Kind
	}{
		{"402 一律算余额", &APIError{Status: 402, Msg: "Insufficient Balance"}, KindQuota},
		// OpenAI 余额用完也回 429，只能靠 code 与限流区分开
		{"429 insufficient_quota", &APIError{Status: 429, Code: "insufficient_quota", Msg: "You exceeded your current quota"}, KindQuota},
		{"429 中文余额不足", &APIError{Status: 429, Msg: "账户余额不足"}, KindQuota},
		{"403 billing", &APIError{Status: 403, Msg: "billing not active"}, KindQuota},
		{"错误码 billing_hard_limit_reached", &APIError{Status: 400, Code: "billing_hard_limit_reached"}, KindQuota},
		{"普通 429 是限流", &APIError{Status: 429, Msg: "Rate limit reached"}, KindRateLimit},
		// OpenAI 未绑卡账户的普通限流原文：末尾那句 billing 链接不能让它变成「余额不足」
		{"OpenAI 限流消息带 billing 链接", &APIError{Status: 429, Code: "rate_limit_exceeded", Msg: "Rate limit reached for gpt-4o-mini in organization org-xxx on requests per min (RPM): Limit 3, Used 3, Requested 1. Please try again in 20s. Visit https://platform.openai.com/account/billing to add a payment method."}, KindRateLimit},
		{"国内限流写「额度超限」", &APIError{Status: 429, Msg: "TPM 额度超限，请求过于频繁"}, KindRateLimit},
		{"带 Retry-After 的 429 是限流", &APIError{Status: 429, Msg: "please check billing", RetryAfter: 5 * time.Second}, KindRateLimit},
		{"400 消息里只有 credit 一词不算余额", &APIError{Status: 400, Msg: "invalid credit card field"}, KindRejected},
		{"错误塞在 200 里是上游问题", &APIError{Status: 200, Msg: "server overloaded"}, KindUpstream},
		{"错误塞在 200 里的余额不足", &APIError{Status: 200, Msg: "Insufficient Balance"}, KindQuota},
		{"401", &APIError{Status: 401, Msg: "bad key"}, KindAuth},
		{"403 非 billing", &APIError{Status: 403, Msg: "forbidden"}, KindAuth},
		{"404 模型不存在", &APIError{Status: 404, Msg: "model not found"}, KindRejected},
		{"400", &APIError{Status: 400, Msg: "invalid request"}, KindRejected},
		// 500 的消息里出现 credit 不代表余额问题
		{"5xx", &APIError{Status: 503, Msg: "credit service down"}, KindUpstream},
		{"网络错误", errors.New("dial tcp: connection refused"), KindUpstream},
		{"超时", context.DeadlineExceeded, KindUpstream},
		{"截断", ErrTruncated, KindTruncated},
		{"输出不可用", fmt.Errorf("%w，换个模型", ErrBadOutput), KindBadOutput},
		{"取消", context.Canceled, KindCanceled},
		{"包裹后仍能认出", fmt.Errorf("翻译失败：%w", &APIError{Status: 401}), KindAuth},
	}
	for _, c := range cases {
		if got := Classify(c.err); got != c.want {
			t.Errorf("%s：Classify = %s，想要 %s", c.name, got, c.want)
		}
	}
}

// 余额用完不是「稍后重试」能解决的，不能被判成可重试。
func TestQuotaNotRetryable(t *testing.T) {
	e := &APIError{Status: 429, Code: "insufficient_quota"}
	if e.Retryable() {
		t.Error("余额不足被判成可重试")
	}
}

func newTestHealth() (*Health, *time.Time) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	h := NewHealth()
	h.now = func() time.Time { return now }
	return h, &now
}

func TestHealthCooldownByKind(t *testing.T) {
	cases := []struct {
		err  error
		want time.Duration
	}{
		{&APIError{Status: 402}, time.Hour},
		{&APIError{Status: 401}, 30 * time.Minute},
		{&APIError{Status: 404}, 10 * time.Minute},
		{&APIError{Status: 429}, time.Minute},
		{&APIError{Status: 429, RetryAfter: 5 * time.Second}, 5 * time.Second},
		{&APIError{Status: 503}, 30 * time.Second},
		{ErrTruncated, 0},
		{ErrBadOutput, 0},
	}
	for _, c := range cases {
		h, now := newTestHealth()
		h.RecordFail(1, c.err)
		got := h.Get(1).CooldownUntil
		var d time.Duration
		if !got.IsZero() {
			d = got.Sub(*now)
		}
		if d != c.want {
			t.Errorf("%v：冷却 %v，想要 %v", c.err, d, c.want)
		}
	}
}

func TestHealthBackoffGrowsAndResetsOnOK(t *testing.T) {
	h, now := newTestHealth()
	var last time.Duration
	for i := 0; i < 8; i++ {
		h.RecordFail(1, &APIError{Status: 502})
		d := h.Get(1).CooldownUntil.Sub(*now)
		if d < last {
			t.Fatalf("第 %d 次失败冷却反而变短：%v < %v", i+1, d, last)
		}
		last = d
	}
	if last != 10*time.Minute {
		t.Errorf("退避应封顶 10 分钟，得到 %v", last)
	}
	h.RecordOK(1)
	st := h.Get(1)
	if st.Failures != 0 || st.Cooling(*now) {
		t.Errorf("成功后应清零：%+v", st)
	}
	if st.LastError == "" {
		t.Error("成功后仍应保留最后一次错误供参考")
	}
}

func TestHealthIgnoresCancel(t *testing.T) {
	h, _ := newTestHealth()
	h.RecordFail(1, context.Canceled)
	if st := h.Get(1); st.Failures != 0 || st.LastError != "" {
		t.Errorf("用户取消不该记成服务商失败：%+v", st)
	}
}

func TestHealthOrder(t *testing.T) {
	h, _ := newTestHealth()
	// 1 冷却 1 小时，3 冷却 30 秒，2/4 正常
	h.RecordFail(1, &APIError{Status: 402})
	h.RecordFail(3, &APIError{Status: 503})
	got := h.Order([]uint{1, 2, 3, 4})
	want := []uint{2, 4, 3, 1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Order = %v，想要 %v（正常的保持原序在前，冷却的按解冻先后在后，但不跳过）", got, want)
	}
}

func TestHealthResetAndExpiry(t *testing.T) {
	h, now := newTestHealth()
	h.RecordFail(1, &APIError{Status: 402})
	h.Reset(1)
	if h.Get(1).Cooling(*now) {
		t.Error("手动解除后仍在冷却")
	}
	h.RecordFail(2, &APIError{Status: 503})
	*now = now.Add(31 * time.Second)
	if got := h.Order([]uint{2, 3}); got[0] != 2 {
		t.Errorf("冷却到期后应回到原位：%v", got)
	}
}

// 余额不足的判定依赖错误码，所以错误码和 Retry-After 都得真的从 HTTP 响应里解出来。
func TestChatParsesCodeAndRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"You exceeded your current quota","type":"insufficient_quota","code":"insufficient_quota"}}`))
	}))
	defer srv.Close()

	c, _ := New(Config{BaseURL: srv.URL, Model: "m"})
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	apiErr, ok := asAPIError(err)
	if !ok {
		t.Fatalf("应当是 *APIError：%v", err)
	}
	if apiErr.Code != "insufficient_quota" || apiErr.RetryAfter != 7*time.Second {
		t.Errorf("Code=%q RetryAfter=%v", apiErr.Code, apiErr.RetryAfter)
	}
	if Classify(err) != KindQuota {
		t.Errorf("应判为余额不足，得到 %s", Classify(err))
	}
}

// 有的服务 code 是数字，不能因此让整个错误体解析失败、丢掉消息。
func TestErrMessageNumericCode(t *testing.T) {
	msg, code := errMessage([]byte(`{"error":{"message":"余额不足","code":1113}}`))
	if msg != "余额不足" || code != "1113" {
		t.Errorf("msg=%q code=%q", msg, code)
	}
}
