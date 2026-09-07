package sync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	gosync "sync"
	"time"

	"flymail-core/logger"
	"flymail/internal/fts"
	"flymail/modules/email/folder"

	coreimap "flymail-core/imap"

	imapv2 "github.com/emersion/go-imap/v2"
	"go.uber.org/zap"
)

// ── 服务端搜索兜底（IMAP SEARCH）────────────────────────────────────────────
//
// 本地全文索引只覆盖已同步到本地的内容：元数据按同步深度截断，正文更是按需预取。
// 用户明确要求「在服务器上继续找」时，把同一份查询翻译成 IMAP SEARCH 发给每个启用账户，
// 命中而本地搜不到的邮件（本地无此行，或有行但无正文）连正文一起补抓入库，
// 之后前端重跑本地搜索即可看到它们——服务端结果不单独成一个列表，
// 避免两套结果集在分页/筛选/去重上各说各话。
//
// 全部经账户 runner 的前台队列执行，与打开邮件/下载附件同级：不新建连接、
// 不与后台全量同步抢同一条连接。

// Searcher 是 Session 的可选能力：支持 UID SEARCH。
// 单独成接口而不并入 Session：既有的测试假会话不必都实现它，
// 不支持的会话（理论上不存在，真实 *coreimap.Session 一定支持）会被明确报出来。
type Searcher interface {
	UIDSearch(criteria *imapv2.SearchCriteria) ([]imapv2.UID, error)
}

// EnabledAccountLister 是 AccountConfigProvider 的可选能力：列出启用账户。
// account.Service 已实现；同样不并入必选接口，理由同上。
type EnabledAccountLister interface {
	ListEnabledIDs() ([]uint, error)
}

// AccountIdentifier 是 AccountConfigProvider 的可选能力：取账户的名称与邮箱，
// 供 account: 限定符在服务端搜索里筛账户（与本地 searchScope 的 LIKE 口径一致：子串、不分大小写）。
type AccountIdentifier interface {
	AccountIdentity(id uint) (name, email string, err error)
}

// RemoteSearchResult 是一次服务端搜索的汇总。
type RemoteSearchResult struct {
	// 补抓入库的邮件数（此前本地搜不到、现在可以了）
	Fetched int `json:"fetched"`
	// 服务器命中总数（含本地已有的）。按文件夹累加：Gmail 的「所有邮件」与各标签文件夹
	// 会把同一封重复计入，只作展示参考。
	Matched int `json:"matched"`
	// 实际搜过（至少有一个文件夹执行了 SEARCH）的账户数 / 文件夹数
	Accounts int `json:"accounts"`
	Folders  int `json:"folders"`
	// 各账户的失败原因（部分失败不让整个搜索失败：能补多少是多少）
	Errors []string `json:"errors,omitempty"`
}

// 每个文件夹最多补抓的命中数：服务器上一个词命中几千封是常事，
// 全抓回来既慢又没意义——用户要的是「找到那封」，不是把归档整个搬回来。
const remoteSearchFetchCap = 200

// 一次远程搜索的总时限：账户并行、文件夹串行，每个 SEARCH+FETCH 在慢服务器上都可能上秒。
const remoteSearchTimeout = 90 * time.Second

// ErrRemoteSearchUnsupported 表示装配的会话/账户源不支持远程搜索（只会在测试装配下出现）。
var ErrRemoteSearchUnsupported = errors.New("remote search not supported by this session")

// RemoteSearch 把查询翻译成 IMAP SEARCH，对所有启用账户执行并补抓本地搜不到的命中。
func (s *Service) RemoteSearch(ctx context.Context, q string) (*RemoteSearchResult, error) {
	parsed := fts.Parse(q)
	if parsed.Empty() {
		return &RemoteSearchResult{}, nil
	}
	criteria := imapCriteria(parsed)
	// has: / account: / in: 都翻译不成 IMAP 条件。只有这类限定符时 criteria 是空的，
	// 而 go-imap 会把空条件编码成 SEARCH ALL——等于把每个文件夹整个搜一遍再补抓 200 封回来。
	// 服务器上没有可执行的条件，就没有去服务器的意义。
	if criteriaEmpty(criteria) {
		return &RemoteSearchResult{}, nil
	}

	lister, ok := s.accounts.(EnabledAccountLister)
	if !ok {
		return nil, ErrRemoteSearchUnsupported
	}
	accountIDs, err := lister.ListEnabledIDs()
	if err != nil {
		return nil, err
	}
	// account: 限定符：只搜名称/邮箱匹配的账户。本地搜索会按同样口径筛结果，
	// 若这里不筛，会把用户明确排除的账户的邮件也抓进本地库。
	if parsed.Account != "" {
		accountIDs = s.filterAccounts(accountIDs, parsed.Account)
	}

	ctx, cancel := context.WithTimeout(ctx, remoteSearchTimeout)
	defer cancel()

	res := &RemoteSearchResult{}
	var mu gosync.Mutex
	var wg gosync.WaitGroup
	for _, id := range accountIDs {
		wg.Add(1)
		// 账户之间并行：每个账户有自己的 runner 与连接，互不排队
		go func(accountID uint) {
			defer wg.Done()
			part, err := s.remoteSearchAccount(ctx, accountID, parsed, criteria)
			mu.Lock()
			defer mu.Unlock()
			res.Fetched += part.Fetched
			res.Matched += part.Matched
			res.Folders += part.Folders
			if part.Folders > 0 {
				res.Accounts++
			}
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("account %d: %v", accountID, err))
			}
		}(id)
	}
	wg.Wait()
	logger.Info("remote-search: 完成",
		zap.String("q", q), zap.Int("accounts", res.Accounts), zap.Int("folders", res.Folders),
		zap.Int("matched", res.Matched), zap.Int("fetched", res.Fetched), zap.Int("errors", len(res.Errors)))
	return res, nil
}

// criteriaEmpty 判断 IMAP 条件是否一个都没有（空条件会被编码成 SEARCH ALL）。
func criteriaEmpty(c *imapv2.SearchCriteria) bool {
	return len(c.Text) == 0 && len(c.Header) == 0 && len(c.Flag) == 0 && len(c.NotFlag) == 0 &&
		c.SentBefore.IsZero() && c.SentSince.IsZero()
}

// filterAccounts 按 account: 取值筛账户；账户源不支持取名称/邮箱时保守地一个都不搜。
func (s *Service) filterAccounts(ids []uint, needle string) []uint {
	ident, ok := s.accounts.(AccountIdentifier)
	if !ok {
		return nil
	}
	lower := strings.ToLower(needle)
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		name, email, err := ident.AccountIdentity(id)
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(name), lower) || strings.Contains(strings.ToLower(email), lower) {
			out = append(out, id)
		}
	}
	return out
}

// remoteProgress 是单账户搜索的进度计数，闭包与外层共享。
//
// 必须加锁：ForegroundOp 在 ctx 超时时会先行返回 ctx.Err()，而任务本身仍在 runner 的
// goroutine 里继续跑并继续计数——外层此时若直接读裸字段就是数据竞争。
// 外层只在锁内做一次快照，之后闭包再怎么写都与返回值无关。
type remoteProgress struct {
	mu   gosync.Mutex
	part RemoteSearchResult
}

func (p *remoteProgress) add(folders, matched, fetched int) {
	p.mu.Lock()
	p.part.Folders += folders
	p.part.Matched += matched
	p.part.Fetched += fetched
	p.mu.Unlock()
}

func (p *remoteProgress) snapshot() RemoteSearchResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.part
}

// remoteSearchAccount 在一个账户的前台队列里逐文件夹执行 SEARCH + 补抓。
func (s *Service) remoteSearchAccount(ctx context.Context, accountID uint, q fts.Query, criteria *imapv2.SearchCriteria) (RemoteSearchResult, error) {
	folders, err := s.folders.List(accountID)
	if err != nil {
		return RemoteSearchResult{}, err
	}
	targets := remoteSearchFolders(folders, q.Folder)
	if len(targets) == 0 {
		return RemoteSearchResult{}, nil
	}

	prog := &remoteProgress{}
	run := func(sess Session) error {
		searcher, ok := sess.(Searcher)
		if !ok {
			return ErrRemoteSearchUnsupported
		}
		for _, f := range targets {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, err := sess.SelectFolder(f.Path); err != nil {
				return fmt.Errorf("select %s: %w", f.Path, err)
			}
			uids, err := searcher.UIDSearch(criteria)
			if err != nil {
				return fmt.Errorf("search %s: %w", f.Path, err)
			}
			prog.add(1, len(uids), 0)
			if len(uids) == 0 {
				continue
			}
			raw := make([]uint32, 0, len(uids))
			for _, u := range uids {
				raw = append(raw, uint32(u))
			}
			need, err := s.messages.UIDsNotSearchable(f.ID, raw)
			if err != nil {
				return err
			}
			if len(need) == 0 {
				continue
			}
			// 命中太多时只补最新的一批：UID 随入库单调递增，末尾即最新。
			// RFC 没规定 SEARCH 返回顺序，先排序再截尾。
			if len(need) > remoteSearchFetchCap {
				sort.Slice(need, func(i, j int) bool { return need[i] < need[j] })
				need = need[len(need)-remoteSearchFetchCap:]
			}
			fetchUIDs := make([]imapv2.UID, 0, len(need))
			for _, u := range need {
				fetchUIDs = append(fetchUIDs, imapv2.UID(u))
			}
			emails, err := sess.FetchByUIDs(fetchUIDs, coreimap.FetchOptions{FetchBody: true, FallbackHeaders: true})
			if err != nil {
				return fmt.Errorf("fetch %s: %w", f.Path, err)
			}
			for _, e := range emails {
				if _, err := s.messages.StoreFetched(accountID, f.ID, e, true); err != nil {
					return err
				}
				prog.add(0, 0, 1)
			}
		}
		return nil
	}
	if s.orch == nil {
		err = s.withDialedSession(accountID, run)
	} else {
		err = s.orch.ForegroundOp(ctx, accountID, run)
	}
	return prog.snapshot(), err
}

// remoteSearchFolders 选出要搜的文件夹：可选中的全部；带 in: 限定时按与本地检索同样的口径
// （类型精确 / 显示名或路径包含）筛选，免得 in:inbox 还去翻每个归档文件夹。
func remoteSearchFolders(all []folder.Folder, want string) []folder.Folder {
	out := make([]folder.Folder, 0, len(all))
	lower := strings.ToLower(want)
	for _, f := range all {
		if !f.Selectable {
			continue
		}
		if want != "" &&
			strings.ToLower(f.Type) != lower &&
			!strings.Contains(strings.ToLower(f.DisplayName), lower) &&
			!strings.Contains(strings.ToLower(f.Path), lower) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// imapCriteria 把解析后的查询翻译成 IMAP SEARCH 条件。
//
// 对应关系尽量贴近本地语义，但两边天然有差异，这里不追求完全一致：
//   - 自由词 → TEXT（头部 + 正文子串，服务器决定大小写/编码处理）；短语不拆词
//   - from:/to:/subject: → 对应 HEADER 子串
//   - is:unread/read/starred/unstarred → \Seen / \Flagged 正反条件
//   - before:/after: → SENTBEFORE / SENTSINCE（按 Date 头，与本地按 date 列一致）
//   - has:attachment、account: 无 IMAP 对应，忽略（补抓回来后本地搜索会再筛一遍）
//   - in: 不进条件，用来选文件夹（见 remoteSearchFolders）
func imapCriteria(q fts.Query) *imapv2.SearchCriteria {
	c := &imapv2.SearchCriteria{}
	for _, t := range q.Terms {
		c.Text = append(c.Text, t.Text)
	}
	header := func(key string, terms []fts.Term) {
		for _, t := range terms {
			c.Header = append(c.Header, imapv2.SearchCriteriaHeaderField{Key: key, Value: t.Text})
		}
	}
	header("From", q.From)
	header("To", q.To)
	header("Subject", q.Subject)
	if q.Seen != nil {
		if *q.Seen {
			c.Flag = append(c.Flag, imapv2.FlagSeen)
		} else {
			c.NotFlag = append(c.NotFlag, imapv2.FlagSeen)
		}
	}
	if q.Flagged != nil {
		if *q.Flagged {
			c.Flag = append(c.Flag, imapv2.FlagFlagged)
		} else {
			c.NotFlag = append(c.NotFlag, imapv2.FlagFlagged)
		}
	}
	if q.Before != nil {
		c.SentBefore = *q.Before
	}
	if q.After != nil {
		c.SentSince = *q.After
	}
	return c
}
