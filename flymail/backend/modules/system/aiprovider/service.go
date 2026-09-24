package aiprovider

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"flymail-core/logger"

	"flymail/internal/ai"

	"go.uber.org/zap"
)

// ErrNoEncryptor 表示进程未注入加密器，带密钥的配置无法保存。
// 与 setting.ErrNoEncryptor 同一个立场：宁可拒绝，也不悄悄存成明文。
var ErrNoEncryptor = errors.New("未配置加密器，无法保存密钥")

// ErrModelRequired 表示没填模型名。
var ErrModelRequired = errors.New("请填写模型名")

// Encryptor 是密钥加解密能力（*crypto.Encryptor 满足之）。
type Encryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// Service 封装 AI 配置的业务逻辑。
type Service struct {
	repo *Repository
	enc  Encryptor
	// health 与 translate.Service 共用同一份（由 app 装配注入）。
	health *ai.Health
	// newChat 可在测试里替换。生产环境恒为 ai.New。
	newChat func(ai.Config) (chatClient, error)
	// decryptWarned 记下已经报过「密钥解不开」的配置 ID。Active 每次翻译、每次查
	// 语言清单都会调，不去重的话，一条坏密钥会把日志刷满。改过密钥后清掉重报。
	decryptWarned sync.Map
}

// chatClient 是「测试连接」用到的 AI 能力（*ai.Client 满足之）。
type chatClient interface {
	Chat(ctx context.Context, msgs []ai.Message) (string, error)
}

func NewService(repo *Repository, enc Encryptor, health *ai.Health) *Service {
	if health == nil {
		health = ai.NewHealth()
	}
	return &Service{
		repo:    repo,
		enc:     enc,
		health:  health,
		newChat: func(cfg ai.Config) (chatClient, error) { return ai.New(cfg) },
	}
}

func (s *Service) toView(p Provider) View {
	st := s.health.Get(p.ID)
	return View{
		Provider: p,
		KeySet:   p.APIKey != "",
		Status:   st,
		Cooling:  st.Cooling(s.health.Now()),
	}
}

// Input 是新建/修改一条配置时的输入。
//
// 指针字段表示「没传就不改」，只对修改有意义；新建时 nil 按零值处理。
// APIKey 的三种状态必须分得开：nil = 不动，"" = 不动（界面上密钥框留空就是这个意思），
// ClearKey = 清除。不能拿空串表示清除——用户只是改了个模型名，
// 密钥框自然是空的（我们从不回显），那一下保存就会把密钥抹掉。
type Input struct {
	Name     *string `json:"name"`
	BaseURL  *string `json:"base_url"`
	Model    *string `json:"model"`
	APIKey   *string `json:"api_key"`
	ClearKey bool    `json:"clear_key"`
	Enabled  *bool   `json:"enabled"`
}

// List 按使用顺序返回全部配置。
func (s *Service) List() ([]View, error) {
	list, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	out := make([]View, len(list))
	for i, p := range list {
		out[i] = s.toView(p)
	}
	return out, nil
}

// Create 新建一条配置，排到末尾。
func (s *Service) Create(in Input) (*View, error) {
	p := Provider{Enabled: true}
	if err := s.apply(&p, in); err != nil {
		return nil, err
	}
	if err := s.repo.Create(&p); err != nil {
		return nil, err
	}
	v := s.toView(p)
	return &v, nil
}

// Update 修改一条配置。
func (s *Service) Update(id uint, in Input) (*View, error) {
	p, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	if err := s.apply(p, in); err != nil {
		return nil, err
	}
	if err := s.repo.Update(p); err != nil {
		return nil, err
	}
	s.decryptWarned.Delete(id)
	v := s.toView(*p)
	return &v, nil
}

func (s *Service) Delete(id uint) error {
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	s.health.Forget(id)
	return nil
}

// Reset 手动解除冷却（「我已经充值了」）。
func (s *Service) Reset(id uint) (*View, error) {
	p, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	s.health.Reset(id)
	v := s.toView(*p)
	return &v, nil
}

// TestResult 是一次「测试连接」的结果。
type TestResult struct {
	OK        bool    `json:"ok"`
	LatencyMS int64   `json:"latency_ms"`
	Kind      ai.Kind `json:"kind,omitempty"`
	Error     string  `json:"error,omitempty"`
	Provider  View    `json:"provider"`
}

// testTimeout 比翻译的超时短得多：只回一个词，半分钟还没回来就说明这条线路
// 此刻用不了，没必要让用户盯着转圈两分钟。
const testTimeout = 30 * time.Second

// Test 用这条配置发一次极短的对话，并把结果记进健康状态。
//
// 记进去是有意的：测试通过就清掉冷却（用户刚充了值、点一下测试，翻译马上能用），
// 测试失败也照实冷却——那和一次真实调用失败没有区别。
func (s *Service) Test(ctx context.Context, id uint) (*TestResult, error) {
	p, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	cfg := ai.Config{BaseURL: p.BaseURL, APIKey: s.decrypt(*p), Model: p.Model, Timeout: testTimeout}
	res := &TestResult{}
	start := time.Now()
	cli, err := s.newChat(cfg)
	if err == nil {
		_, err = cli.Chat(ctx, []ai.Message{
			{Role: "system", Content: "Reply with the single word: OK"},
			{Role: "user", Content: "ping"},
		})
	}
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		res.Kind = s.health.RecordFail(p.ID, err)
		res.Error = err.Error()
	} else {
		s.health.RecordOK(p.ID)
		res.OK = true
	}
	res.Provider = s.toView(*p)
	return res, nil
}

func (s *Service) Reorder(ids []uint) error { return s.repo.Reorder(ids) }

// InvalidError 表示输入不合法（地址形状、模型为空等），消息是写给人看的。
type InvalidError struct{ Err error }

func (e *InvalidError) Error() string { return e.Err.Error() }
func (e *InvalidError) Unwrap() error { return e.Err }

// apply 把输入校验、归一后写进 p。校验失败返回 *InvalidError。
func (s *Service) apply(p *Provider, in Input) error {
	if in.BaseURL != nil {
		// 地址在进门时就归一成完整的 chat/completions 地址（见 ai.Endpoint），
		// 调用那侧拿到的永远可以直接发请求。
		endpoint, err := ai.Endpoint(*in.BaseURL)
		if err != nil {
			return &InvalidError{err}
		}
		p.BaseURL = endpoint
	}
	if in.Model != nil {
		// 模型名只裁空白不校验格式：什么形状都有（qwen2.5:7b、accounts/x/y），
		// 而从文档里复制极容易带上尾随空格，带空格的模型名只会换来一个无关的 404。
		p.Model = strings.TrimSpace(*in.Model)
	}
	if in.Name != nil {
		p.Name = strings.TrimSpace(*in.Name)
	}
	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}
	switch {
	case in.ClearKey:
		p.APIKey = ""
	case in.APIKey != nil && strings.TrimSpace(*in.APIKey) != "":
		if s.enc == nil {
			return ErrNoEncryptor
		}
		ct, err := s.enc.Encrypt(strings.TrimSpace(*in.APIKey))
		if err != nil {
			return err
		}
		p.APIKey = ct
	}

	if p.BaseURL == "" {
		return &InvalidError{ai.ErrNotConfigured}
	}
	if p.Model == "" {
		return &InvalidError{ErrModelRequired}
	}
	// 名字可以不填：列表里总得有个能认出来的标签，用主机名顶上。
	if p.Name == "" {
		p.Name = hostOf(p.BaseURL)
	}
	return nil
}

func hostOf(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		return u.Host
	}
	return endpoint
}

// Active 是一条可以直接拿去调用的配置（密钥已解密）。
type Active struct {
	ID     uint
	Name   string
	Config ai.Config
}

// Active 按使用顺序返回启用中的配置。
//
// 密钥解不开（最常见是换过 FLYMAIL_CRYPTO_ENCRYPTION_KEY）时**不丢掉这一条**，
// 而是按无密钥处理：上游会回一个 401，切换逻辑会如实报出「密钥被拒」，
// 用户就知道该去重填密钥。悄悄跳过的话，用户只会看到翻译莫名其妙走了备用线路。
func (s *Service) Active() ([]Active, error) {
	list, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	out := make([]Active, 0, len(list))
	for _, p := range list {
		if !p.Enabled {
			continue
		}
		out = append(out, Active{
			ID:   p.ID,
			Name: p.Name,
			Config: ai.Config{
				BaseURL: p.BaseURL,
				APIKey:  s.decrypt(p),
				Model:   p.Model,
			},
		})
	}
	return out, nil
}

func (s *Service) decrypt(p Provider) string {
	if p.APIKey == "" || s.enc == nil {
		return ""
	}
	plain, err := s.enc.Decrypt(p.APIKey)
	if err != nil {
		if _, seen := s.decryptWarned.LoadOrStore(p.ID, true); !seen {
			logger.Warn("aiprovider: 密钥解密失败，按无密钥处理（常见原因是换过 FLYMAIL_CRYPTO_ENCRYPTION_KEY，请重填密钥）",
				zap.Uint("provider_id", p.ID), zap.Error(err))
		}
		return ""
	}
	return plain
}
