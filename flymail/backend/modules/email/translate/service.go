package translate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"flymail-core/logger"

	"flymail/internal/ai"
	"flymail/internal/lang"
	"flymail/internal/mailtext"
	"flymail/modules/email/message"

	"go.uber.org/zap"
	"golang.org/x/net/html"
)

// ErrNoContent 表示这封邮件没有可翻译的内容（正文与主题都是空的，或只有图片）。
var ErrNoContent = errors.New("这封邮件没有可翻译的文字内容")

// Settings 是每次翻译前现取的配置。
//
// 现取而不是构造时定死：用户在设置页换了模型或密钥，应当下一次点翻译就生效，
// 而不是等重启。这与 OAuth 凭据的做法一致（见 app 装配）。
type Settings struct {
	BaseURL string
	APIKey  string
	Model   string
	// DefaultTarget 是请求未指定语言时用的目标语言。
	DefaultTarget string
}

// DetailFunc 取一封邮件的详情（含正文）。
//
// 用函数而不是接口，是因为这里只需要一个动作，而提供它的 sync.Service
// 是个很大的东西——把整个服务作为依赖传进来，会让这个包在测试里
// 不得不搭起一套 IMAP 装配。
type DetailFunc func(messageID uint) (*message.MessageDetail, error)

// MetaFunc 取邮件的元数据行，**不会**触发正文抓取。
//
// 与 DetailFunc 分开的理由是代价差了几个数量级：DetailFunc 在正文尚未同步时
// 要连一次 IMAP，而判断"这封信的发件人在不在远程图信任名单里"只要一次主键查询。
// 译文命中缓存时走的正是后者，不该因为要判个信任状态就去连服务器。
type MetaFunc func(messageID uint) (*message.Message, error)

// Service 负责翻译与译文缓存。
type Service struct {
	repo     *Repository
	detail   DetailFunc
	meta     MetaFunc
	settings func() Settings
	// trusted 报告发件人是否在远程图片信任名单里；未注入时一律不信任。
	trusted func(addr string) bool
	// newClient 可在测试里替换成假客户端。生产环境恒为 ai.New。
	newClient func(ai.Config) (chatter, error)
}

// chatter 是服务用到的 AI 能力（*ai.Client 满足之）。
type chatter interface {
	Chat(ctx context.Context, msgs []ai.Message) (string, error)
	Model() string
}

func NewService(repo *Repository, detail DetailFunc, meta MetaFunc, settings func() Settings) *Service {
	return &Service{
		repo:     repo,
		detail:   detail,
		meta:     meta,
		settings: settings,
		newClient: func(cfg ai.Config) (chatter, error) {
			return ai.New(cfg)
		},
	}
}

// SetTrustedSenderCheck 注入「发件人是否在远程图片信任名单」的查询。
//
// 与详情接口用的是同一个判断（见 sync.Service.SetTrustedSenderCheck）：
// 译文与原文必须对同一封信给出同样的远程图策略，否则用户会看到
// 原文里图片正常、一按翻译图片全变占位符。
func (s *Service) SetTrustedSenderCheck(fn func(addr string) bool) { s.trusted = fn }

// AllowRemoteFor 报告这封邮件的正文是否应保留远程引用。
func (s *Service) AllowRemoteFor(messageID uint) bool {
	if s.trusted == nil || s.meta == nil {
		return false
	}
	m, err := s.meta(messageID)
	if err != nil || m == nil {
		return false
	}
	return s.trusted(m.FromAddr)
}

// Enabled 报告 AI 接口是否已配置到可用程度。
func (s *Service) Enabled() bool {
	cfg := s.settings()
	return strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != ""
}

// DefaultTarget 返回默认目标语言（配置为空或不认识时退回内置默认）。
func (s *Service) DefaultTarget() string {
	t := strings.TrimSpace(s.settings().DefaultTarget)
	if !lang.IsSupported(t) {
		return lang.DefaultTarget
	}
	return t
}

// ResolveTarget 把请求里的语言参数归一成一个可用的目标语言。
func (s *Service) ResolveTarget(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return s.DefaultTarget(), nil
	}
	if !lang.IsSupported(requested) {
		return "", fmt.Errorf("不支持的目标语言：%s", requested)
	}
	return requested, nil
}

// Cached 只查缓存，不调用 AI。没有译文时返回 (nil, nil)。
func (s *Service) Cached(messageID uint, target string) (*Translation, error) {
	return s.repo.Get(messageID, target)
}

// Translate 返回一封邮件的译文，优先用缓存。第二个返回值表示是否命中缓存。
//
// force 为真时无视缓存重译并覆盖——那是用户的显式动作（换了模型想再试一次）。
func (s *Service) Translate(ctx context.Context, messageID uint, target string, force bool) (*Translation, bool, error) {
	if !force {
		if cached, err := s.repo.Get(messageID, target); err == nil && cached != nil {
			return cached, true, nil
		} else if err != nil {
			// 缓存读不出来不该挡住翻译：退回去调一次 AI，代价是花一次钱。
			logger.Warn("translate: 读缓存失败，改为重新翻译",
				zap.Uint("message_id", messageID), zap.Error(err))
		}
	}
	if !s.Enabled() {
		return nil, false, ai.ErrNotConfigured
	}
	d, err := s.detail(messageID)
	if err != nil {
		return nil, false, err
	}
	out, err := s.translateDetail(ctx, d, target)
	if err != nil {
		return nil, false, err
	}
	if err := s.repo.Save(out); err != nil {
		// 落库失败只是"这次白花了钱"，译文本身是好的，照常返回给用户。
		logger.Warn("translate: 译文落库失败，本次结果不会被缓存",
			zap.Uint("message_id", messageID), zap.Error(err))
	}
	return out, false, nil
}

// maxTotalRunes 是一封邮件单次翻译的字符总预算。
//
// 没有它的话，一封几 MB 的营销邮件（几千个文本节点）按一下翻译就会发出
// 几十上百次请求——用户看到的是转圈好几分钟，账单上看到的是一次点击几块钱。
// 超出的部分保留原文，并在结果里标出来（Partial），由界面告诉用户。
const maxTotalRunes = 60000

// translateDetail 是真正干活的那段：分段 → 分批 → 并发翻译 → 回填。
func (s *Service) translateDetail(ctx context.Context, d *message.MessageDetail, target string) (*Translation, error) {
	cfg := s.settings()
	cli, err := s.newClient(ai.Config{BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}

	useHTML := strings.TrimSpace(d.HTMLBody) != ""

	var doc *html.Node
	var bodySegs []*segment
	if useHTML {
		doc, bodySegs, err = parseDocument(d.HTMLBody)
		if err != nil {
			return nil, err
		}
	} else if seg := newTextSegment(d.TextBody); seg != nil {
		bodySegs = []*segment{seg}
	}

	// 主题排在正文前面一起送：它和正文是同一封信，放在同一批里让模型
	// 能借正文的上下文翻主题（"Re: Order 1234" 该怎么翻，取决于正文在说什么）。
	subjectSeg := newTextSegment(d.Subject)
	all := bodySegs
	if subjectSeg != nil {
		all = append([]*segment{subjectSeg}, bodySegs...)
	}
	if len(all) == 0 {
		return nil, ErrNoContent
	}

	units, partial := capUnits(buildUnits(all), maxTotalRunes)
	if len(units) == 0 {
		return nil, ErrNoContent
	}
	fatal, firstErr := s.runChunks(ctx, cli, chunkUnits(units, maxChunkRunes), target)
	if fatal != nil {
		return nil, fatal
	}

	translated := 0
	for _, seg := range all {
		seg.apply()
		if seg.translated() {
			translated++
		}
	}
	// 一段都没翻出来，就不要把"原文的副本"当译文存下来——那会让缓存
	// 永久地把这封信钉在未翻译状态，用户再点多少次都是原文。
	if translated == 0 {
		// ⚠ 有过错误就报那个错误，别报一句笼统的"换个模型试试"。
		//
		// 这是真机验证时抓到的：把上游地址指向一个没人监听的端口，用户看到的是
		// "AI 没有返回可用的译文，请稍后重试或更换模型"——照着这句话换十个模型
		// 也没用，真正的原因（连不上）一个字都没露出来。
		// 部分成功时不报：那时用户手里有一份能读的译文，报错只会吓人。
		if firstErr != nil {
			return nil, fmt.Errorf("翻译失败：%w", firstErr)
		}
		return nil, errors.New("AI 没有返回可用的译文，请稍后重试或更换模型")
	}

	out := &Translation{
		MessageID:  d.ID,
		TargetLang: target,
		SourceLang: lang.Detect(mailtext.Extract(d.TextBody, d.HTMLBody).Text),
		Model:      cli.Model(),
		Partial:    partial,
	}
	if subjectSeg != nil {
		out.Subject = subjectSeg.result
	} else {
		out.Subject = d.Subject
	}
	if useHTML {
		rendered, err := renderDocument(doc)
		if err != nil {
			return nil, err
		}
		out.HTMLBody = rendered
	} else if len(bodySegs) > 0 {
		out.TextBody = bodySegs[0].result
	}
	return out, nil
}

// capUnits 按总字符预算截断待译单元，返回是否发生了截断。
func capUnits(units []unit, budget int) ([]unit, bool) {
	total := 0
	for i, u := range units {
		total += len([]rune(u.text))
		if total > budget {
			return units[:i], true
		}
	}
	return units, false
}

// chunkConcurrency 是同时在飞的请求数。
//
// 3 是在"别让用户干等"与"别把服务商的限流撞出来"之间取的：一封普通邮件
// 也就两三批，并发之后总时长约等于一次请求；真正的大邮件（几十批）也能
// 把几分钟压到一分钟内，同时又远低于各家按分钟计的请求配额。
const chunkConcurrency = 3

// runChunks 并发翻译各批，并把译文写回对应的单元。
//
// 错误的处理分两种：
//   - 不可重试（401 密钥错、400 模型名错）—— 作为 fatal 返回，整体失败。
//     后面的批必然同样失败，继续发只是把同一个错误重复几十遍，还要用户多等几十秒。
//   - 可重试/偶发 —— 记日志，并把**第一个**留作 firstErr。那一批的片段退回原文，
//     其余照常显示：一封信里几段没翻，比整封翻译失败有用得多。
//     firstErr 只在"一段都没翻出来"时才会被拿去报错（见调用方）。
func (s *Service) runChunks(ctx context.Context, cli chatter, chunks [][]unit, target string) (fatal, firstErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	sem := make(chan struct{}, chunkConcurrency)
	prompt := systemPrompt(target)

	for _, chunk := range chunks {
		wg.Add(1)
		go func(chunk []unit) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			raw, err := cli.Chat(ctx, []ai.Message{
				{Role: "system", Content: prompt},
				{Role: "user", Content: encodeChunk(chunk)},
			})
			if err != nil {
				mu.Lock()
				defer mu.Unlock()
				if firstErr == nil {
					firstErr = err
				}
				if isFatal(err) && fatal == nil {
					fatal = err
					cancel() // 其余批必然同样失败，不必让用户再等
					return
				}
				logger.Warn("translate: 一批片段翻译失败，这部分将保留原文", zap.Error(err))
				return
			}
			decoded := decodeChunk(raw)
			mu.Lock()
			defer mu.Unlock()
			for _, u := range chunk {
				if text, ok := decoded[u.id]; ok {
					u.seg.out[u.part] = text
				}
			}
		}(chunk)
	}
	wg.Wait()
	return fatal, firstErr
}

// isFatal 报告这个错误是否意味着"再试多少批都一样"。
func isFatal(err error) bool {
	if errors.Is(err, ai.ErrNotConfigured) || errors.Is(err, context.Canceled) {
		return true
	}
	var apiErr *ai.APIError
	if errors.As(err, &apiErr) {
		return !apiErr.Retryable()
	}
	return false
}

// systemPrompt 是翻译用的系统提示词。
//
// 每一条规则都对应一种实际见过的跑偏：模型会合并短片段、会给译文加编号、
// 会把 URL 也"翻译"成中文、会在最前面说一句"好的，以下是译文"。
// 编号协议本身能容忍一部分（见 decodeChunk），但少跑偏一次就少一段退回原文。
func systemPrompt(target string) string {
	name := lang.NameOf(target)
	return "You are a professional email translator. Translate each numbered segment into " + name + ".\n\n" +
		"Input format: each segment is written as ⟦N⟧followed by its text.\n" +
		"Output format: reply with every segment as ⟦N⟧followed by its translation, one per line, in the same order.\n\n" +
		"Rules:\n" +
		"- Never merge, split, reorder, add or omit segments. The set of numbers in your reply must match the input exactly.\n" +
		"- Translate only the text. Keep URLs, email addresses, numbers, dates, currency amounts, file names, code and product names unchanged.\n" +
		"- Preserve leading and trailing punctuation, and keep the tone of the original.\n" +
		"- A segment may be a fragment of a sentence (the text was split by HTML markup). Translate it as a fragment; do not complete it.\n" +
		"- If a segment is already in " + name + ", repeat it unchanged.\n" +
		"- Output nothing else: no explanations, no greetings, no code fences."
}
