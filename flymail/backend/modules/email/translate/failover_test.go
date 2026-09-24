package translate

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"flymail/internal/ai"
	"flymail/modules/email/message"
)

// scriptedChat 按「第几次调用」决定成败，用来模拟一家服务商翻到一半出错。
type scriptedChat struct {
	model string
	mu    sync.Mutex
	calls int
	// failAt 返回第 n 次调用（从 1 起）的错误；nil 表示这次成功。
	failAt func(n int) error
}

func (s *scriptedChat) Model() string { return s.model }

func (s *scriptedChat) Chat(ctx context.Context, msgs []ai.Message) (string, error) {
	s.mu.Lock()
	s.calls++
	n := s.calls
	s.mu.Unlock()
	if s.failAt != nil {
		if err := s.failAt(n); err != nil {
			return "", err
		}
	}
	var sb strings.Builder
	for id, text := range decodeChunk(msgs[len(msgs)-1].Content) {
		sb.WriteString(markOpen + itoa(id) + markClose + "[" + s.model + "]" + text + "\n")
	}
	return sb.String(), nil
}

func (s *scriptedChat) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func always(err error) func(int) error { return func(int) error { return err } }

// newFailoverSvc 按模型名把配置路由到对应的假客户端。配置 ID 按顺序从 1 起。
func newFailoverSvc(t *testing.T, detail *message.MessageDetail, chats ...*scriptedChat) *Service {
	t.Helper()
	providers := make([]Provider, len(chats))
	byModel := map[string]*scriptedChat{}
	for i, c := range chats {
		providers[i] = Provider{ID: uint(i + 1), Name: "P" + c.model, Config: ai.Config{BaseURL: "https://x/v1", Model: c.model}}
		byModel[c.model] = c
	}
	svc := NewService(
		NewRepository(newDB(t)),
		func(id uint) (*message.MessageDetail, error) { return detail, nil },
		func(id uint) (*message.Message, error) { return &message.Message{ID: id}, nil },
		func() Settings { return Settings{Providers: providers, DefaultTarget: "zh"} },
	)
	svc.newClient = func(cfg ai.Config) (chatter, error) { return byModel[cfg.Model], nil }
	return svc
}

// bigDetail 造一封会被切成多批的信。
func bigDetail() *message.MessageDetail {
	d := &message.MessageDetail{}
	d.ID = 7
	d.Subject = "Hello"
	var sb strings.Builder
	for i := 0; i < 12; i++ {
		sb.WriteString("<p>" + strings.Repeat("word ", 150) + "</p>")
	}
	d.HTMLBody = sb.String()
	return d
}

func TestFailoverQuotaSwitchesToNext(t *testing.T) {
	a := &scriptedChat{model: "a", failAt: always(&ai.APIError{Status: 402, Msg: "Insufficient Balance"})}
	b := &scriptedChat{model: "b"}
	svc := newFailoverSvc(t, htmlDetail(), a, b)

	got, _, err := svc.Translate(context.Background(), 7, "zh", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "Pb" || got.Model != "b" {
		t.Errorf("译文应出自 B：provider=%q model=%q", got.Provider, got.Model)
	}
	if st := svc.health.Get(1); st.LastKind != ai.KindQuota || !st.Cooling(svc.health.Now()) {
		t.Errorf("A 应因余额不足进入冷却：%+v", st)
	}
	if svc.health.Get(2).LastOKAt.IsZero() {
		t.Error("B 应记一次成功")
	}
}

// 整封重翻：A 翻到一半限流，最终译文里不能混进 A 的片段。
func TestFailoverRetranslatesWholeMessage(t *testing.T) {
	d := bigDetail()
	a := &scriptedChat{model: "a", failAt: func(n int) error {
		if n >= 2 {
			return &ai.APIError{Status: 429, Msg: "Rate limit"}
		}
		return nil
	}}
	b := &scriptedChat{model: "b"}
	svc := newFailoverSvc(t, d, a, b)

	got, _, err := svc.Translate(context.Background(), 7, "zh", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.HTMLBody, "[a]") || strings.Contains(got.Subject, "[a]") {
		t.Error("译文混进了 A 的片段：切换后必须整封重翻")
	}
	if !strings.Contains(got.HTMLBody, "[b]") {
		t.Error("译文里没有 B 的结果")
	}
	if got.Partial {
		t.Error("不该标成部分翻译")
	}
}

// 最后一个候选没有下家可换，个别批失败时仍交出部分译文。
func TestFailoverLastCandidateTolerant(t *testing.T) {
	d := bigDetail()
	a := &scriptedChat{model: "a", failAt: always(&ai.APIError{Status: 401})}
	b := &scriptedChat{model: "b", failAt: func(n int) error {
		if n == 2 {
			return &ai.APIError{Status: 503}
		}
		return nil
	}}
	svc := newFailoverSvc(t, d, a, b)

	got, _, err := svc.Translate(context.Background(), 7, "zh", false)
	if err != nil {
		t.Fatalf("最后一个候选部分成功时应返回译文：%v", err)
	}
	if !strings.Contains(got.HTMLBody, "[b]") {
		t.Error("没有 B 的译文")
	}
	if !strings.Contains(got.HTMLBody, "word word") {
		t.Error("失败的那批应保留原文")
	}
}

func TestFailoverAllFailedAggregates(t *testing.T) {
	a := &scriptedChat{model: "a", failAt: always(&ai.APIError{Status: 402, Msg: "no money"})}
	b := &scriptedChat{model: "b", failAt: always(errors.New("connection refused"))}
	svc := newFailoverSvc(t, htmlDetail(), a, b)

	_, _, err := svc.Translate(context.Background(), 7, "zh", false)
	var all *AllFailedError
	if !errors.As(err, &all) {
		t.Fatalf("应返回 *AllFailedError：%v", err)
	}
	msg := err.Error()
	for _, want := range []string{"2 个 AI 配置均失败", "Pa", "余额", "Pb", "connection refused"} {
		if !strings.Contains(msg, want) {
			t.Errorf("聚合错误缺少 %q：%s", want, msg)
		}
	}
	if all.ConfigOnly() {
		t.Error("有一家是连不上，不该判成纯配置问题")
	}
}

func TestFailoverAllConfigErrorsIs400(t *testing.T) {
	a := &scriptedChat{model: "a", failAt: always(&ai.APIError{Status: 401})}
	b := &scriptedChat{model: "b", failAt: always(&ai.APIError{Status: 402})}
	svc := newFailoverSvc(t, htmlDetail(), a, b)
	_, _, err := svc.Translate(context.Background(), 7, "zh", false)
	var all *AllFailedError
	if !errors.As(err, &all) || !all.ConfigOnly() {
		t.Fatalf("全是密钥/余额问题应判为纯配置问题：%v", err)
	}
}

// 冷却中的排到后面，但不跳过；全员冷却时照样去试。
func TestFailoverCoolingOrderedLastButTried(t *testing.T) {
	a := &scriptedChat{model: "a"}
	b := &scriptedChat{model: "b"}
	svc := newFailoverSvc(t, htmlDetail(), a, b)
	svc.health.RecordFail(1, &ai.APIError{Status: 402})

	got, _, err := svc.Translate(context.Background(), 7, "zh", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "b" || a.count() != 0 {
		t.Errorf("A 在冷却，应先用 B：model=%s A 调用 %d 次", got.Model, a.count())
	}

	svc.health.RecordFail(2, &ai.APIError{Status: 402})
	got, _, err = svc.Translate(context.Background(), 7, "zh", true)
	if err != nil {
		t.Fatalf("全员冷却时仍应去试：%v", err)
	}
	if got.Model != "a" {
		t.Errorf("A 先进入冷却、先解冻，应先试 A：%s", got.Model)
	}
}

func TestFailoverCancelDoesNotSwitch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &scriptedChat{model: "a", failAt: func(int) error { cancel(); return context.Canceled }}
	b := &scriptedChat{model: "b"}
	svc := newFailoverSvc(t, htmlDetail(), a, b)

	_, _, err := svc.Translate(ctx, 7, "zh", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("应返回取消：%v", err)
	}
	if b.count() != 0 {
		t.Error("用户已离开，不该再换下一家")
	}
	if st := svc.health.Get(1); st.Failures != 0 {
		t.Errorf("取消不算服务商失败：%+v", st)
	}
}

// 只有一个配置时保留原来的报错形状，界面上不出现「1 个配置均失败」。
func TestFailoverSingleProviderKeepsErrorShape(t *testing.T) {
	a := &scriptedChat{model: "a", failAt: always(&ai.APIError{Status: http.StatusUnauthorized, Msg: "bad key"})}
	svc := newFailoverSvc(t, htmlDetail(), a)
	_, _, err := svc.Translate(context.Background(), 7, "zh", false)
	var all *AllFailedError
	if errors.As(err, &all) {
		t.Fatal("单配置不该包成 AllFailedError")
	}
	if !strings.Contains(err.Error(), "bad key") {
		t.Errorf("应带上游原话：%v", err)
	}
}

// 备用在冷却时，主力的偶发失败不该让整封作废：保留主力的部分译文。
func TestFailoverTolerantWhenRestCooling(t *testing.T) {
	d := bigDetail()
	a := &scriptedChat{model: "a", failAt: func(n int) error {
		if n == 2 {
			return &ai.APIError{Status: 503}
		}
		return nil
	}}
	b := &scriptedChat{model: "b", failAt: always(&ai.APIError{Status: 402})}
	svc := newFailoverSvc(t, d, a, b)
	svc.health.RecordFail(2, &ai.APIError{Status: 402}) // B 余额已空、在冷却

	got, _, err := svc.Translate(context.Background(), 7, "zh", false)
	if err != nil {
		t.Fatalf("应拿到 A 的部分译文，而不是整封失败：%v", err)
	}
	if got.Model != "a" || !strings.Contains(got.HTMLBody, "[a]") {
		t.Errorf("译文应出自 A：model=%s", got.Model)
	}
	if b.count() != 0 {
		t.Error("A 已交出部分译文，不该再去撞冷却中的 B")
	}
}
