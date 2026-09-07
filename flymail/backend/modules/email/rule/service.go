package rule

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"flymail-core/logger"
	"flymail-core/types"

	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	"flymail/modules/system/notify"

	"go.uber.org/zap"
)

// Actor 是规则动作需要的邮件操作能力，由 sync.Service 满足（本地先改 + 合并入回写队列）。
// 用接口而不是直接依赖 sync：database.Migrate 要引用本包的模型，而 database 不能反向依赖 sync。
type Actor interface {
	BatchMove(ids []uint, targetFolderID uint) error
	BatchDelete(ids []uint) error
	BatchSetRead(ids []uint, read bool) error
	BatchSetFlagged(ids []uint, flagged bool) error
}

// EmitFunc 与 sync / notify 的通知回调同签名。
type EmitFunc func(eventType string, accountID uint, messageID uint, title, body string)

const (
	eventMailRule = string(notify.EventMailRule)
	// notifyPerRuleCap：同一规则同轮命中超过这个数就合并成一条通知，避免刷屏
	notifyPerRuleCap = 3
	// testMatchCap 试运行最多回传的命中条数；超过时置 Truncated
	testMatchCap = 50
	// runRetention 执行日志保留期：幂等只需覆盖「同一封邮件再次入库」的窗口
	runRetention = 180 * 24 * time.Hour
)

type Service struct {
	repo     *Repository
	folders  *folder.Service
	messages *message.Service
	actor    Actor
	emit     EmitFunc
	// selfAddrs 返回本地各账户的邮箱（小写），黑名单拒绝这些地址：右键「屏蔽发件人」点在自己发的邮件上
	// 会把自己拉黑，之后所有自发自收 / 抄送自己的邮件都进垃圾箱
	selfAddrs func() []string
	// lastPrune 是上次清理执行日志的 unix 秒；Apply 由各账户 runner 并发调用，用原子操作
	lastPrune atomic.Int64
}

func NewService(repo *Repository, folders *folder.Service, messages *message.Service) *Service {
	return &Service{repo: repo, folders: folders, messages: messages}
}

func (s *Service) SetActor(a Actor)                    { s.actor = a }
func (s *Service) SetEmitter(fn EmitFunc)              { s.emit = fn }
func (s *Service) SetSelfAddresses(fn func() []string) { s.selfAddrs = fn }

// ── CRUD ─────────────────────────────────────────────────────────────────────

func (s *Service) List() ([]RuleDTO, error) {
	rules, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	out := make([]RuleDTO, 0, len(rules))
	for _, r := range rules {
		out = append(out, toDTO(r))
	}
	return out, nil
}

// validate 编译规则并在能校验时校验移动目标：限定了账户的规则，目标文件夹必须在该账户里存在，
// 否则保存成功却永远不动，用户只能去翻日志。全账户规则的目标按账户各自解析，保存时不校验。
func (s *Service) validate(r Rule) (*Compiled, error) {
	c, err := Compile(r)
	if err != nil {
		return nil, err
	}
	if r.AccountID > 0 {
		for _, a := range c.Actions {
			if a.Type == ActionMove && s.resolveFolder(r.AccountID, a.Value) == nil {
				return nil, fmt.Errorf("%w: 账户里没有文件夹 %q", ErrInvalid, a.Value)
			}
		}
	}
	return c, nil
}

func (s *Service) Create(in RuleInput) (*RuleDTO, error) {
	r := in.toRule()
	if _, err := s.validate(r); err != nil {
		return nil, err
	}
	if err := s.repo.Create(&r); err != nil {
		return nil, err
	}
	d := toDTO(r)
	return &d, nil
}

func (s *Service) Update(id uint, in RuleInput) (*RuleDTO, error) {
	r := in.toRule()
	r.ID = id
	if _, err := s.validate(r); err != nil {
		return nil, err
	}
	if err := s.repo.Update(&r); err != nil {
		return nil, err
	}
	saved, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	d := toDTO(*saved)
	return &d, nil
}

func (s *Service) Delete(id uint) error                  { return s.repo.Delete(id) }
func (s *Service) Reorder(ids []uint) error              { return s.repo.Reorder(ids) }
func (s *Service) ListRuns(limit int) ([]RuleRun, error) { return s.repo.ListRuns(limit) }

func (s *Service) ListBlocks() ([]BlockEntry, error) { return s.repo.ListBlocks() }

// AddBlock 归一化后入库；非法返回 ErrInvalid，重复返回 ErrDuplicate。
func (s *Service) AddBlock(pattern, note string) (*BlockEntry, error) {
	p := NormalizePattern(pattern)
	if p == "" {
		return nil, fmt.Errorf("%w: 黑名单条目必须是邮箱地址或域名", ErrInvalid)
	}
	if s.selfAddrs != nil {
		for _, self := range s.selfAddrs() {
			if strings.EqualFold(strings.TrimSpace(self), p) {
				return nil, fmt.Errorf("%w: 不能屏蔽自己的账户地址", ErrInvalid)
			}
		}
	}
	e := &BlockEntry{Pattern: p, Note: strings.TrimSpace(note)}
	if err := s.repo.AddBlock(e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Service) DeleteBlock(id uint) error { return s.repo.DeleteBlock(id) }

// ── 求值输入 ──────────────────────────────────────────────────────────────────

// view 把存储模型转成求值视图；正文与附件名按需加载（只有含相应条件的规则才需要）。
func (s *Service) view(m *message.Message, needBody, needAtts bool) *MessageView {
	to, toAddrs := joinAddrs(m.ToJSON)
	cc, ccAddrs := joinAddrs(m.CcJSON)
	v := &MessageView{
		From: formatAddr(m.FromName, m.FromAddr), FromAddr: m.FromAddr,
		To: to, ToAddrs: toAddrs,
		Cc: cc, CcAddrs: ccAddrs,
		Subject:       m.Subject,
		HasAttachment: m.HasAttachment,
		BodyKnown:     m.BodySynced,
	}
	if m.BodySynced {
		if needBody {
			v.Body, v.BodyKnown = s.messages.BodyText(m.ID)
		}
		if needAtts {
			v.Attachments = s.messages.AttachmentNames(m.ID)
		}
	}
	return v
}

func formatAddr(name, addr string) string {
	if name == "" {
		return addr
	}
	return name + " <" + addr + ">"
}

// joinAddrs 返回「名 <地址>」列表与纯地址列表（都用 ", " 连接）。
func joinAddrs(raw string) (full, addrs string) {
	if raw == "" {
		return "", ""
	}
	var list []types.Address
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return "", ""
	}
	fulls := make([]string, 0, len(list))
	pure := make([]string, 0, len(list))
	for _, a := range list {
		fulls = append(fulls, formatAddr(a.Name, a.Email))
		pure = append(pure, a.Email)
	}
	return strings.Join(fulls, ", "), strings.Join(pure, ", ")
}

// messageKey 是幂等键：RFC Message-ID，缺则 (folder, uid)。
func messageKey(m *message.Message) string {
	if m.MessageID != "" {
		return m.MessageID
	}
	return fmt.Sprintf("u%d-%d", m.FolderID, m.UID)
}

// ── 试运行 ────────────────────────────────────────────────────────────────────

// Test 对规则作用范围内收件箱最近的邮件求值，只读。
func (s *Service) Test(in RuleInput, limit int) (*TestResult, error) {
	c, err := Compile(in.toRule())
	if err != nil {
		return nil, err
	}
	rows, err := s.messages.ListRecentInbox(in.AccountID, limit)
	if err != nil {
		return nil, err
	}
	res := &TestResult{Matched: []message.MessageListItem{}, Scanned: len(rows)}
	needBody, needAtts := c.NeedsBody(), c.NeedsAttachments()
	for i := range rows {
		m := &rows[i]
		hit, unknown := c.Evaluate(s.view(m, needBody, needAtts))
		if unknown {
			res.WithoutBody++
		}
		if hit {
			if len(res.Matched) < testMatchCap {
				res.Matched = append(res.Matched, message.ToListItem(m))
			} else {
				res.Truncated = true
			}
		}
	}
	return res, nil
}

// ── 实跑 ──────────────────────────────────────────────────────────────────────

// plan 汇总一轮里要执行的动作，按类型合并成尽量少的 Batch* 调用。
type plan struct {
	read, star, del []uint
	move            map[uint][]uint // 目标文件夹 id → ids（按 id 归并：「归档」与「Archive」是同一个文件夹）
	moved           map[uint]uint   // 已计划移动的邮件 → 目标；一封邮件只能去一个地方，优先级高的规则胜出
	notify          map[uint][]*message.Message
	notifyName      map[uint]string // 规则 id → 名称（发通知时用，不再回库查）
	runs            []RuleRun
}

// Handled 返回这批邮件里已被规则或黑名单处理过的 id（按幂等键查执行日志）。
// 同步侧用它抑制 Gmail 标签文件夹里的重复提醒：INBOX 那份被规则移走/标读后，
// 标签文件夹同步时同一封仍是「新未读」，不查日志就会照常发一条「新邮件」。
func (s *Service) Handled(accountID uint, msgs []message.Message) map[uint]bool {
	out := map[uint]bool{}
	if len(msgs) == 0 {
		return out
	}
	keys := make([]string, 0, len(msgs))
	for i := range msgs {
		keys = append(keys, messageKey(&msgs[i]))
	}
	ran, err := s.repo.RanRules(accountID, keys)
	if err != nil {
		return out
	}
	for i := range msgs {
		if len(ran[keys[i]]) > 0 {
			out[msgs[i].ID] = true
		}
	}
	return out
}

// Apply 对一批新到收件箱的邮件执行黑名单与规则。返回是否改动过任何邮件（调用方据此重算计数）。
// 调用方保证：同账户串行、只传收件箱、不传基线导入。
func (s *Service) Apply(accountID uint, f *folder.Folder, msgs []message.Message) (bool, error) {
	if len(msgs) == 0 || s.actor == nil {
		return false, nil
	}
	patterns, err := s.repo.BlockPatterns()
	if err != nil {
		return false, err
	}
	rules, err := s.repo.ListEnabledFor(accountID)
	if err != nil {
		return false, err
	}
	if len(patterns) == 0 && len(rules) == 0 {
		return false, nil
	}
	compiled := make([]*Compiled, 0, len(rules))
	needBody, needAtts := false, false
	for _, r := range rules {
		c, err := Compile(r)
		if err != nil {
			logger.Warn("rules: 规则无法编译，跳过", zap.Uint("rule_id", r.ID), zap.Error(err))
			continue
		}
		compiled = append(compiled, c)
		needBody = needBody || c.NeedsBody()
		needAtts = needAtts || c.NeedsAttachments()
	}
	keys := make([]string, 0, len(msgs))
	for i := range msgs {
		keys = append(keys, messageKey(&msgs[i]))
	}
	ran, err := s.repo.RanRules(accountID, keys)
	if err != nil {
		return false, err
	}
	// 目标文件夹解析按名字缓存：同一轮里同名只查一次
	resolved := map[string]*folder.Folder{}
	resolve := func(name string) *folder.Folder {
		key := strings.ToLower(strings.TrimSpace(name))
		if dst, ok := resolved[key]; ok {
			return dst
		}
		dst := s.resolveFolder(accountID, name)
		resolved[key] = dst
		return dst
	}

	p := &plan{move: map[uint][]uint{}, moved: map[uint]uint{}, notify: map[uint][]*message.Message{}, notifyName: map[uint]string{}}
	blocked := map[uint]bool{}
	for i := range msgs {
		m := &msgs[i]
		key := keys[i]
		done := ran[key]
		if pat, hit := Blocked(m.FromAddr, patterns); hit {
			if !done[0] {
				blocked[m.ID] = true
				p.runs = append(p.runs, RuleRun{AccountID: accountID, MessageKey: key, RuleID: 0, RuleName: "黑名单", Action: "block:" + pat})
			}
			continue
		}
		// 视图每封只构造一次（正文 / 附件名各一次查询），不随规则数放大——这段跑在账户 runner 里
		v := s.view(m, needBody, needAtts)
		for _, c := range compiled {
			if done[c.Rule.ID] {
				// 上次就是在这条规则停下的：再次入库时同样在这里停，否则被它挡掉的后续规则会趁机执行
				if c.Rule.StopProcessing {
					break
				}
				continue
			}
			hit, _ := c.Evaluate(v)
			if !hit {
				continue
			}
			p.runs = append(p.runs, RuleRun{AccountID: accountID, MessageKey: key, RuleID: c.Rule.ID, RuleName: c.Rule.Name, Action: s.collect(p, c, m, f, resolve)})
			if c.Rule.StopProcessing {
				break
			}
		}
	}
	changed := false
	// 黑名单：移到 junk（无则 trash）；两者都没有只标已读。失败只记日志——这批邮件下一轮不会再进引擎，
	// 一次文件夹查询失败不能连带把规则动作也丢掉
	if len(blocked) > 0 {
		ids := make([]uint, 0, len(blocked))
		for id := range blocked {
			ids = append(ids, id)
		}
		if err := s.applyBlock(accountID, ids); err != nil {
			logger.Warn("rules: 黑名单处置失败", zap.Uint("account_id", accountID), zap.Error(err))
		} else {
			changed = true
		}
	}
	changed = s.execute(accountID, p) || changed
	if err := s.repo.RecordRuns(p.runs); err != nil {
		logger.Warn("rules: 记录执行日志失败", zap.Error(err))
	}
	s.maybePrune()
	return changed, nil
}

// collect 把一条命中规则的动作记进计划，返回动作摘要（写执行日志）。
// 目标文件夹解析不到、或就是当前文件夹时，移动记为 skipped 并进日志摘要，用户能看出规则没生效的原因。
func (s *Service) collect(p *plan, c *Compiled, m *message.Message, f *folder.Folder, resolve func(string) *folder.Folder) string {
	parts := make([]string, 0, len(c.Actions))
	for _, a := range c.Actions {
		switch a.Type {
		case ActionMarkRead:
			p.read = append(p.read, m.ID)
			parts = append(parts, a.Type)
		case ActionStar:
			p.star = append(p.star, m.ID)
			parts = append(parts, a.Type)
		case ActionDelete:
			p.del = append(p.del, m.ID)
			parts = append(parts, a.Type)
		case ActionNotify:
			p.notify[c.Rule.ID] = append(p.notify[c.Rule.ID], m)
			p.notifyName[c.Rule.ID] = c.Rule.Name
			parts = append(parts, a.Type)
		case ActionMove:
			dst := resolve(a.Value)
			switch {
			case dst == nil:
				logger.Warn("rules: 目标文件夹不存在，跳过移动", zap.Uint("rule_id", c.Rule.ID), zap.String("folder", a.Value))
				parts = append(parts, "move:"+a.Value+"(missing)")
			case dst.ID == f.ID:
				parts = append(parts, "move:"+a.Value+"(same)")
			case p.moved[m.ID] != 0:
				// 更高优先级的规则已经决定了去向
				parts = append(parts, "move:"+a.Value+"(skipped)")
			default:
				p.moved[m.ID] = dst.ID
				p.move[dst.ID] = append(p.move[dst.ID], m.ID)
				parts = append(parts, "move:"+a.Value)
			}
		}
	}
	return strings.Join(parts, ",")
}

// execute 按「先改标志位、再移动、最后删除」的顺序执行：移动 / 删除会删本地行，之后的操作找不到它。
// 同一封既要移动又要删除时删除优先（移动名单里剔除）。目标文件夹按 id 升序遍历，结果与 map 迭代顺序无关。
func (s *Service) execute(accountID uint, p *plan) bool {
	changed := false
	if len(p.read) > 0 {
		if err := s.actor.BatchSetRead(uniq(p.read), true); err != nil {
			logger.Warn("rules: 标已读失败", zap.Error(err))
		} else {
			changed = true
		}
	}
	if len(p.star) > 0 {
		if err := s.actor.BatchSetFlagged(uniq(p.star), true); err != nil {
			logger.Warn("rules: 加星失败", zap.Error(err))
		} else {
			changed = true
		}
	}
	// 通知在移动/删除之前发：动作摘要要用到的邮件行此时还在
	for ruleID, list := range p.notify {
		s.notifyMatches(accountID, p.notifyName[ruleID], list)
	}
	del := map[uint]bool{}
	for _, id := range p.del {
		del[id] = true
	}
	targets := make([]uint, 0, len(p.move))
	for id := range p.move {
		targets = append(targets, id)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i] < targets[j] })
	for _, dst := range targets {
		kept := make([]uint, 0, len(p.move[dst]))
		for _, id := range uniq(p.move[dst]) {
			if !del[id] {
				kept = append(kept, id)
			}
		}
		if len(kept) == 0 {
			continue
		}
		if err := s.actor.BatchMove(kept, dst); err != nil {
			logger.Warn("rules: 移动失败", zap.Uint("folder_id", dst), zap.Error(err))
		} else {
			changed = true
		}
	}
	if len(p.del) > 0 {
		if err := s.actor.BatchDelete(uniq(p.del)); err != nil {
			logger.Warn("rules: 删除失败", zap.Error(err))
		} else {
			changed = true
		}
	}
	return changed
}

// applyBlock 把命中黑名单的邮件移到垃圾邮件（无则回收站）；账户两者都没有时只能标已读。
func (s *Service) applyBlock(accountID uint, ids []uint) error {
	for _, typ := range []string{"junk", "trash"} {
		if dst, err := s.folders.FindByType(accountID, typ); err == nil && dst != nil {
			return s.actor.BatchMove(ids, dst.ID)
		}
	}
	logger.Warn("rules: 账户没有垃圾邮件/回收站文件夹，黑名单邮件只标已读", zap.Uint("account_id", accountID))
	return s.actor.BatchSetRead(ids, true)
}

// resolveFolder 按 display_name 或 path 在账户内找目标文件夹（大小写不敏感）。
func (s *Service) resolveFolder(accountID uint, name string) *folder.Folder {
	list, err := s.folders.List(accountID)
	if err != nil {
		return nil
	}
	want := strings.ToLower(strings.TrimSpace(name))
	for i := range list {
		if strings.ToLower(list[i].DisplayName) == want || strings.ToLower(list[i].Path) == want {
			return &list[i]
		}
	}
	return nil
}

// notifyMatches 发规则命中通知：≤ notifyPerRuleCap 封逐封发（带 message id 可跳转），更多合并成一条。
func (s *Service) notifyMatches(accountID uint, name string, list []*message.Message) {
	if s.emit == nil || len(list) == 0 {
		return
	}
	if name == "" {
		name = "规则"
	}
	if len(list) <= notifyPerRuleCap {
		for _, m := range list {
			subject := m.Subject
			if subject == "" {
				subject = "（无主题）"
			}
			s.emit(eventMailRule, accountID, m.ID, "规则命中 · "+name, subject)
		}
		return
	}
	s.emit(eventMailRule, accountID, 0, "规则命中 · "+name, fmt.Sprintf("命中 %d 封新邮件", len(list)))
}

// maybePrune 每天最多清一次过期执行日志（CAS 保证并发的 runner 里只有一个去清）。
func (s *Service) maybePrune() {
	now := time.Now().Unix()
	last := s.lastPrune.Load()
	if now-last < int64(24*time.Hour/time.Second) || !s.lastPrune.CompareAndSwap(last, now) {
		return
	}
	if err := s.repo.PruneRuns(time.Now().Add(-runRetention)); err != nil {
		logger.Warn("rules: 清理执行日志失败", zap.Error(err))
	}
}

func uniq(ids []uint) []uint {
	seen := make(map[uint]bool, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
