package translate

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	coredb "flymail-core/database"

	"flymail/internal/ai"
	"flymail/modules/email/message"

	"gorm.io/gorm"
)

// ── 测试脚手架 ─────────────────────────────────────────────────────────────
//
// 这里刻意不用 internal/database.Migrate：那个包 import 了本包（为了把
// Translation 加进 AutoMigrate），包内测试再反向 import 就成环了。
// 这张表只有自己一个模型，单独迁移足够。

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := coredb.OpenSQLite(coredb.Options{Path: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, e := db.DB(); e == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := db.AutoMigrate(&Translation{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// fakeChat 按编号协议把每段原文回成 "[译]原文"，并数自己被调了几次。
type fakeChat struct {
	mu    sync.Mutex
	calls int
	// err 非 nil 时所有调用都失败。
	err error
	// raw 非空时直接返回它，用来模拟模型不按格式回话。
	raw string
	// seen 记录收到的全部请求文本，供断言提示词/编号。
	seen []string
}

func (f *fakeChat) Model() string { return "fake-model" }

func (f *fakeChat) Chat(ctx context.Context, msgs []ai.Message) (string, error) {
	f.mu.Lock()
	f.calls++
	user := msgs[len(msgs)-1].Content
	f.seen = append(f.seen, user)
	err, raw := f.err, f.raw
	f.mu.Unlock()

	if err != nil {
		return "", err
	}
	if raw != "" {
		return raw, nil
	}
	var sb strings.Builder
	for id, text := range decodeChunk(user) {
		sb.WriteString(markOpen)
		sb.WriteString(itoa(id))
		sb.WriteString(markClose)
		sb.WriteString("[译]" + text + "\n")
	}
	return sb.String(), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func newSvc(t *testing.T, detail *message.MessageDetail, chat *fakeChat) *Service {
	t.Helper()
	svc := NewService(
		NewRepository(newDB(t)),
		func(id uint) (*message.MessageDetail, error) { return detail, nil },
		func(id uint) (*message.Message, error) { return &message.Message{ID: id}, nil },
		func() Settings {
			return Settings{BaseURL: "https://x/v1", APIKey: "k", Model: "m", DefaultTarget: "zh"}
		},
	)
	svc.newClient = func(ai.Config) (chatter, error) { return chat, nil }
	return svc
}

func htmlDetail() *message.MessageDetail {
	d := &message.MessageDetail{}
	d.ID = 7
	d.Subject = "Invoice ready"
	d.HTMLBody = `<div><p>Hello <b>world</b></p><a href="https://e.com/x">Pay now</a></div>`
	return d
}

// ── 用例 ───────────────────────────────────────────────────────────────────

func TestTranslateHTMLKeepsStructureAndSubject(t *testing.T) {
	chat := &fakeChat{}
	svc := newSvc(t, htmlDetail(), chat)

	got, cached, err := svc.Translate(context.Background(), 7, "zh", false)
	if err != nil {
		t.Fatal(err)
	}
	if cached {
		t.Error("首次翻译不该报告命中缓存")
	}
	if got.Subject != "[译]Invoice ready" {
		t.Errorf("主题 = %q", got.Subject)
	}
	for _, want := range []string{"[译]Hello", "[译]world", "[译]Pay now", `href="https://e.com/x"`, "<b>"} {
		if !strings.Contains(got.HTMLBody, want) {
			t.Errorf("译文缺少 %q：%s", want, got.HTMLBody)
		}
	}
	if got.Model != "fake-model" {
		t.Errorf("没记下产出译文的模型：%q", got.Model)
	}
	// 这封信的正文只有几个词，识别器按设计会拒绝下结论（见 internal/lang）。
	// 记下空串是对的：界面据此不显示"从英语翻译"，但翻译照常可用。
	if got.SourceLang != "" {
		t.Errorf("短正文不该硬猜语言，得到 %q", got.SourceLang)
	}
}

func TestTranslateRecordsSourceLang(t *testing.T) {
	d := &message.MessageDetail{}
	d.ID = 21
	d.Subject = "Invoice"
	d.HTMLBody = "<p>Hello, please find the invoice attached. Let me know if you have any questions " +
		"about this document and we will get back to you as soon as possible.</p>"
	svc := newSvc(t, d, &fakeChat{})

	got, _, err := svc.Translate(context.Background(), 21, "zh", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceLang != "en" {
		t.Errorf("源语言 = %q，想要 en", got.SourceLang)
	}
}

// 缓存是这个功能的经济基础：同一封信的同一种语言只该花一次钱。
func TestTranslateUsesCache(t *testing.T) {
	chat := &fakeChat{}
	svc := newSvc(t, htmlDetail(), chat)

	if _, _, err := svc.Translate(context.Background(), 7, "zh", false); err != nil {
		t.Fatal(err)
	}
	first := chat.calls

	got, cached, err := svc.Translate(context.Background(), 7, "zh", false)
	if err != nil {
		t.Fatal(err)
	}
	if !cached {
		t.Error("第二次应当命中缓存")
	}
	if chat.calls != first {
		t.Errorf("命中缓存却又调了 AI：%d → %d", first, chat.calls)
	}
	if !strings.Contains(got.HTMLBody, "[译]Hello") {
		t.Error("缓存里的译文不对")
	}

	// 换目标语言是另一份译文，必须真的去翻
	if _, cached, err := svc.Translate(context.Background(), 7, "ja", false); err != nil || cached {
		t.Errorf("换语言应当重新翻译：cached=%v err=%v", cached, err)
	}
}

func TestTranslateForceOverwrites(t *testing.T) {
	chat := &fakeChat{}
	svc := newSvc(t, htmlDetail(), chat)

	if _, _, err := svc.Translate(context.Background(), 7, "zh", false); err != nil {
		t.Fatal(err)
	}
	before := chat.calls

	if _, cached, err := svc.Translate(context.Background(), 7, "zh", true); err != nil || cached {
		t.Fatalf("force 应当重译：cached=%v err=%v", cached, err)
	}
	if chat.calls == before {
		t.Error("force 没有真的调用 AI")
	}

	// 覆盖而不是插新行：同一封信同一语言永远只有一条
	var n int64
	svc.repo.db.Model(&Translation{}).Where("message_id = ? AND target_lang = ?", 7, "zh").Count(&n)
	if n != 1 {
		t.Errorf("重译后留下 %d 行，应当只有一行", n)
	}
}

func TestTranslatePlainText(t *testing.T) {
	d := &message.MessageDetail{}
	d.ID = 9
	d.Subject = "Meeting notes"
	d.TextBody = "Hi team,\n\nThe meeting is moved to Friday.\n"
	svc := newSvc(t, d, &fakeChat{})

	got, _, err := svc.Translate(context.Background(), 9, "zh", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.HTMLBody != "" {
		t.Error("纯文本邮件不该产出 HTML 译文——前端按同一规则选渲染方式，给了就会走错分支")
	}
	if !strings.Contains(got.TextBody, "[译]") {
		t.Errorf("正文没翻：%q", got.TextBody)
	}
	// 结尾换行属于排版，模型会吃掉，所以由我们自己留着
	if !strings.HasSuffix(got.TextBody, "\n") {
		t.Errorf("首尾空白丢了：%q", got.TextBody)
	}
}

func TestTranslateRejectsWhenNotConfigured(t *testing.T) {
	svc := newSvc(t, htmlDetail(), &fakeChat{})
	svc.settings = func() Settings { return Settings{DefaultTarget: "zh"} }

	if _, _, err := svc.Translate(context.Background(), 7, "zh", false); !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("未配置时应返回 ErrNotConfigured，得到 %v", err)
	}
}

func TestTranslateNoContent(t *testing.T) {
	d := &message.MessageDetail{}
	d.ID = 3
	d.HTMLBody = `<div><img src="cid:a"><span>123</span></div>` // 一个字母都没有
	svc := newSvc(t, d, &fakeChat{})

	if _, _, err := svc.Translate(context.Background(), 3, "zh", false); !errors.Is(err, ErrNoContent) {
		t.Fatalf("没有文字内容时应返回 ErrNoContent，得到 %v", err)
	}
}

// 密钥错这类错误对每一批都一样，没必要让用户等完几十批再看同一句话。
func TestTranslateFatalErrorStopsEarly(t *testing.T) {
	chat := &fakeChat{err: &ai.APIError{Status: http.StatusUnauthorized, Msg: "bad key"}}
	svc := newSvc(t, htmlDetail(), chat)

	_, _, err := svc.Translate(context.Background(), 7, "zh", false)
	var apiErr *ai.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("应当把 401 原样报出去，得到 %v", err)
	}
	// 失败的结果绝不能落库：缓存里存一份"原文的副本"会把这封信永久钉在未翻译状态
	var n int64
	svc.repo.db.Model(&Translation{}).Count(&n)
	if n != 0 {
		t.Errorf("失败却写了 %d 行缓存", n)
	}
}

// 模型完全不按格式回话时，不能把它那句话当译文贴进正文。
func TestTranslateRejectsUnusableReply(t *testing.T) {
	chat := &fakeChat{raw: "抱歉，我无法翻译这段内容。"}
	svc := newSvc(t, htmlDetail(), chat)

	if _, _, err := svc.Translate(context.Background(), 7, "zh", false); err == nil {
		t.Fatal("一段都没翻出来时必须报错，而不是把原文当译文缓存下来")
	}
	var n int64
	svc.repo.db.Model(&Translation{}).Count(&n)
	if n != 0 {
		t.Errorf("没翻出来却写了 %d 行缓存", n)
	}
}

// 超长正文要截断并如实标出来，否则用户会以为后半篇"翻过了只是看起来没变"。
func TestTranslateMarksPartial(t *testing.T) {
	d := &message.MessageDetail{}
	d.ID = 11
	d.Subject = "Long one"
	d.HTMLBody = "<p>" + strings.Repeat("This is a long sentence about invoices. ", 3000) + "</p>"
	svc := newSvc(t, d, &fakeChat{})

	got, _, err := svc.Translate(context.Background(), 11, "zh", false)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Partial {
		t.Error("超过字符预算却没标记 Partial")
	}
	if !strings.Contains(got.HTMLBody, "[译]") {
		t.Error("截断之后前面的部分仍应翻好")
	}
}

func TestResolveTarget(t *testing.T) {
	svc := newSvc(t, htmlDetail(), &fakeChat{})

	if got, err := svc.ResolveTarget(""); err != nil || got != "zh" {
		t.Errorf("留空应退回默认：%q %v", got, err)
	}
	if got, err := svc.ResolveTarget("ja"); err != nil || got != "ja" {
		t.Errorf("显式指定：%q %v", got, err)
	}
	if _, err := svc.ResolveTarget("zzz"); err == nil {
		t.Error("清单外的代码会一路传进提示词，必须拒绝")
	}
}

func TestDefaultTargetFallsBack(t *testing.T) {
	svc := newSvc(t, htmlDetail(), &fakeChat{})
	svc.settings = func() Settings { return Settings{BaseURL: "x", Model: "m", DefaultTarget: "zzz"} }
	if got := svc.DefaultTarget(); got != "zh" {
		t.Errorf("库里存了非法值时应退回内置默认，得到 %q", got)
	}
}
