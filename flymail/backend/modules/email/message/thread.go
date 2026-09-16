package message

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"flymail-core/logger"
	"flymail/internal/fts"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ── 线程 id ──────────────────────────────────────────────────────────────────

// threadKey 给一封邮件一个「自己开线程」时用的 id：按账户隔离（两个账户收到同一条讨论不合并，
// 否则会话级移动会撞上跨账户限制），主体用 Message-ID；没有 Message-ID 的用 (folder, uid) 兜底。
func threadKey(m *Message) string {
	if m.MessageID != "" {
		return fmt.Sprintf("%d:%s", m.AccountID, m.MessageID)
	}
	return fmt.Sprintf("%d:u%d-%d", m.AccountID, m.FolderID, m.UID)
}

// linkedIDs 返回这封邮件在头里提到的全部 Message-ID：References、In-Reply-To、以及自己的 Message-ID
// （Gmail 副本要落到同一线程）。去重、去空；顺序无意义，查询用 IN。
func linkedIDs(m *Message) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	refs := strings.Fields(m.References)
	for i := len(refs) - 1; i >= 0; i-- {
		add(refs[i])
	}
	add(m.InReplyTo)
	add(m.MessageID)
	return out
}

// ── 写入路径：逐批归属 ────────────────────────────────────────────────────────

// threadMu 让整库重建与在线归属互斥：重建是「读全表快照 → 内存计算 → 事务回写」，快照之后入库的
// 新邮件若被在线归属挂到某条线程，回写会把它的兄弟行改名而它自己保持旧 id，线程一分为二且不会自愈。
// 在线归属（同步 runner / 正文落库）持读锁，可以并行；重建持写锁。
var threadMu sync.RWMutex

// LoadByFolderUIDs 取某文件夹下给定 UID 的行（一次 IN 查询，分块防绑定变量上限）。
// 同步每批 upsert 之后用它把带主键与既有 thread_id 的行捞回来做线程归属。
func (r *Repository) LoadByFolderUIDs(folderID uint, uids []uint32) ([]Message, error) {
	var out []Message
	for start := 0; start < len(uids); start += uidChunk {
		end := start + uidChunk
		if end > len(uids) {
			end = len(uids)
		}
		var rows []Message
		if err := r.db.Where("folder_id = ? AND uid IN ?", folderID, uids[start:end]).Find(&rows).Error; err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

// SetThreadHeaders 回填一封邮件的 In-Reply-To / References（正文解析出来、元数据阶段没拿到时）。
func (r *Repository) SetThreadHeaders(id uint, inReplyTo, references string) error {
	return r.db.Model(&Message{}).Where("id = ?", id).
		Updates(map[string]any{"in_reply_to": inReplyTo, "references_hdr": references}).Error
}

// AssignThreads 给一批刚入库的邮件定线程。逐封：
//  1. 正向：头里提到的 Message-ID（References / In-Reply-To / 自己）在同账户里已有线程 → 沿用；
//  2. 反向：同账户里有人 In-Reply-To 指向自己 → 认领那条线程（回复比原信先入库的场景）；
//  3. 两边各自命中且不同，或自己原本已在别的线程 → 全部并入第一个命中的线程（一条 UPDATE）；
//  4. 都没命中：已有 thread_id 的保留（重复 upsert 不拆散已归并的），否则自己开线程。
//
// 不做主题匹配：没有索引可用，同步路径上每封一次扫表不可接受；主题兜底只在整库重建里做。
// References 里引用了自己的那些「后代」这里也不查（要 LIKE），同样交给重建。
func (r *Repository) AssignThreads(rows []Message) error {
	threadMu.RLock()
	defer threadMu.RUnlock()
	for i := range rows {
		m := &rows[i]
		var found []string
		if ids := linkedIDs(m); len(ids) > 0 {
			var tids []string
			err := r.db.Model(&Message{}).Distinct("thread_id").
				Where("account_id = ? AND message_id IN ? AND thread_id <> '' AND id <> ?", m.AccountID, ids, m.ID).
				Pluck("thread_id", &tids).Error
			if err != nil {
				return err
			}
			found = append(found, tids...)
		}
		if m.MessageID != "" {
			var tids []string
			err := r.db.Model(&Message{}).Distinct("thread_id").
				Where("account_id = ? AND in_reply_to = ? AND thread_id <> '' AND id <> ?", m.AccountID, m.MessageID, m.ID).
				Pluck("thread_id", &tids).Error
			if err != nil {
				return err
			}
			found = append(found, tids...)
		}

		target := m.ThreadID
		if len(found) == 1 {
			target = found[0]
		} else if len(found) > 1 {
			// 多条线程要合并时取「最早成员所在」的那条：与整库重建的选择规则一致，重建才不会把 id 改一遍
			// （前端的展开态 / 选中 / 游标都挂在 thread_id 上）。IN + Pluck 本身没有顺序保证，不能拿 found[0]。
			var earliest string
			err := r.db.Model(&Message{}).Select("thread_id").
				Where("account_id = ? AND thread_id IN ?", m.AccountID, found).
				Order("date ASC").Order("id ASC").Limit(1).Scan(&earliest).Error
			if err != nil {
				return err
			}
			target = earliest
			if target == "" {
				target = found[0]
			}
		} else if target == "" {
			target = threadKey(m)
		}

		// 其它命中的线程 + 自己原来的线程，全部并入 target
		var merge []string
		for _, tid := range found {
			if tid != target {
				merge = append(merge, tid)
			}
		}
		if m.ThreadID != "" && m.ThreadID != target {
			merge = append(merge, m.ThreadID)
		}
		if len(merge) > 0 {
			if err := r.db.Model(&Message{}).
				Where("account_id = ? AND thread_id IN ?", m.AccountID, merge).
				Update("thread_id", target).Error; err != nil {
				return err
			}
		}
		if m.ThreadID != target {
			if err := r.db.Model(&Message{}).Where("id = ?", m.ID).Update("thread_id", target).Error; err != nil {
				return err
			}
			m.ThreadID = target
		}
	}
	return nil
}

// ── 整库重建 ─────────────────────────────────────────────────────────────────

// subjectFallbackWindow 是主题兜底允许的最大时间跨度：同主题但相隔太久的多半是另一件事。
const subjectFallbackWindow = 90 * 24 * time.Hour

// replyPrefixes 是主题里表示「这是回复/转发」的前缀（小写比较）。只有带这些前缀的主题才参与
// 主题兜底——两封各自独立的「周报」不该被并成一条。
// 转发（Fw:/转发:）与回复归到同一主题：主流客户端（Gmail / Outlook）也把转发挂在原讨论下，有意为之。
// 前缀全是 ASCII 或 CJK，ToLower 不改变字节长度，按 len(p) 截取安全。
var replyPrefixes = []string{"re:", "fw:", "fwd:", "回复:", "回复：", "回覆:", "回覆：", "答复:", "答复：", "答復:", "答復：", "转发:", "转发：", "轉發:", "轉發："}

// normalizeSubject 去掉回复/转发前缀（可叠加，如 "Re: Fw: Re: x"）、压空白、折小写。
// 返回归一化后的主题与「是否带过前缀」。
func normalizeSubject(s string) (string, bool) {
	s = strings.TrimSpace(s)
	hadPrefix := false
	for {
		low := strings.ToLower(s)
		stripped := false
		for _, p := range replyPrefixes {
			if strings.HasPrefix(low, p) {
				s = strings.TrimSpace(s[len(p):])
				hadPrefix, stripped = true, true
				break
			}
		}
		// "Re[2]:" / "RE(3):" 这类带计数的变体
		if !stripped && len(s) > 3 && (low[:2] == "re" || low[:2] == "fw") {
			if i := strings.IndexAny(s, ":："); i > 0 && i < 8 {
				if strings.Trim(s[2:i], "[]()0123456789 ") == "" {
					s = strings.TrimSpace(s[i+1:])
					hadPrefix, stripped = true, true
				}
			}
		}
		if !stripped {
			break
		}
	}
	return strings.ToLower(strings.Join(strings.Fields(s), " ")), hadPrefix
}

// threadRow 是重建时每封邮件需要的最小字段。
type threadRow struct {
	ID        uint
	AccountID uint
	FolderID  uint
	UID       uint32
	MessageID string
	InReplyTo string
	Refs      string // references_hdr；REFERENCES 是 SQL 关键字，别名避开
	Subject   string
	Date      time.Time
	ThreadID  string
}

// unionFind 是重建用的并查集：节点是 Message-ID（含头里提到但库里没有的）。
// find 迭代 + 路径减半、union 按大小合并：几十万节点的长链不会把递归栈打穿。
type unionFind struct {
	parent map[string]string
	size   map[string]int
}

func (u *unionFind) find(x string) string {
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
		u.size[x] = 1
		return x
	}
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	if u.size[ra] < u.size[rb] {
		ra, rb = rb, ra
	}
	u.parent[rb] = ra
	u.size[ra] += u.size[rb]
}

// RebuildThreads 按账户把全部邮件读进内存重放归属规则，并做主题兜底，然后回写 thread_id。
// 返回线程总数。老库（同步时还没抓线程头）第一次启动时会自动跑一次；也是运维入口。
//
// 与写入路径的差别：这里用并查集，头里提到的 Message-ID 即使库里没有也当节点——两封回复指向同一封
// 缺失的原信，照样并成一条；此外做主题兜底（带 Re:/回复: 前缀 + 归一化主题相同 + 90 天内）。
func RebuildThreads(db *gorm.DB) (int, error) {
	threadMu.Lock()
	defer threadMu.Unlock()
	var accountIDs []uint
	if err := db.Model(&Message{}).Distinct("account_id").Pluck("account_id", &accountIDs).Error; err != nil {
		return 0, err
	}
	total := 0
	var firstErr error
	for _, aid := range accountIDs {
		n, err := rebuildAccountThreads(db, aid)
		if err != nil {
			// 单个账户失败不中断其它账户；错误最后一起报。启动路径（EnsureThreads）会把它记日志而不是让服务起不来。
			logger.Warn("threads: 账户重建失败", zap.Uint("account", aid), zap.Error(err))
			if firstErr == nil {
				firstErr = fmt.Errorf("rebuild threads account %d: %w", aid, err)
			}
			continue
		}
		total += n
	}
	return total, firstErr
}

func rebuildAccountThreads(db *gorm.DB, accountID uint) (int, error) {
	var rows []threadRow
	err := db.Model(&Message{}).
		Select("id, account_id, folder_id, uid, message_id, in_reply_to, references_hdr AS refs, subject, date, thread_id").
		Where("account_id = ?", accountID).
		Order("date ASC").Order("id ASC").Scan(&rows).Error
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	uf := &unionFind{parent: make(map[string]string, len(rows)*2), size: make(map[string]int, len(rows)*2)}
	node := func(r *threadRow) string {
		if r.MessageID != "" {
			return r.MessageID
		}
		return fmt.Sprintf("u%d-%d", r.FolderID, r.UID)
	}
	// 第一遍：按头连边
	for i := range rows {
		r := &rows[i]
		self := node(r)
		uf.find(self)
		for _, id := range strings.Fields(r.Refs) {
			uf.union(self, id)
		}
		if r.InReplyTo != "" {
			uf.union(self, r.InReplyTo)
		}
	}
	// 第二遍：主题兜底。只对「头里没有任何线索」的回复做——有头的以头为准，主题相同也不硬并。
	// bySubject 记每个归一化主题最近一次出现的 (节点, 日期)，按日期顺序扫，天然只会向前并。
	type last struct {
		node string
		date time.Time
	}
	bySubject := map[string]last{}
	for i := range rows {
		r := &rows[i]
		norm, hadPrefix := normalizeSubject(r.Subject)
		if norm == "" {
			continue
		}
		self := node(r)
		if r.Refs == "" && r.InReplyTo == "" && hadPrefix {
			if prev, ok := bySubject[norm]; ok && r.Date.Sub(prev.date) <= subjectFallbackWindow {
				uf.union(prev.node, self)
			}
		}
		bySubject[norm] = last{node: self, date: r.Date}
	}
	// 线程 id：集合里最早那封（rows 已按日期升序，首次遇到的就是最早的）已有 thread_id 就沿用，
	// 没有才用它的 threadKey。沿用是为了稳定——在线归属让原信认领回复的线程时 id 是回复的 key，
	// 重建若一律改成原信的 key，每次重建都会大面积改名，前端挂在 thread_id 上的展开态 / 选中 / 游标全失效。
	rootOf := map[string]string{}
	updates := map[string][]uint{} // thread_id → 需要改的行
	threads := 0
	for i := range rows {
		r := &rows[i]
		set := uf.find(node(r))
		tid, ok := rootOf[set]
		if !ok {
			tid = r.ThreadID
			if tid == "" {
				tid = threadKey(&Message{AccountID: r.AccountID, FolderID: r.FolderID, UID: r.UID, MessageID: r.MessageID})
			}
			rootOf[set] = tid
			threads++
		}
		if r.ThreadID != tid {
			updates[tid] = append(updates[tid], r.ID)
		}
	}
	changed := 0
	err = db.Transaction(func(tx *gorm.DB) error {
		for tid, ids := range updates {
			for start := 0; start < len(ids); start += uidChunk {
				end := start + uidChunk
				if end > len(ids) {
					end = len(ids)
				}
				if err := tx.Model(&Message{}).Where("id IN ?", ids[start:end]).Update("thread_id", tid).Error; err != nil {
					return err
				}
			}
			changed += len(ids)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	logger.Info("threads: 重建完成", zap.Uint("account", accountID),
		zap.Int("messages", len(rows)), zap.Int("threads", threads), zap.Int("updated", changed))
	return threads, nil
}

// EnsureThreads 在启动时检查是否有还没归属线程的邮件（老库升级），有则整库重建一次。
// 新库每封入库时就已归属，这里只是一次 COUNT。
func EnsureThreads(db *gorm.DB) error {
	var n int64
	if err := db.Model(&Message{}).Where("thread_id = ''").Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	// 同步阻塞启动：12.8k 封实测 0.8s，几十万封也只是几秒 + 几十 MB 峰值，且只在升级后跑一次。
	// 单账户失败只记日志——线程归属缺失不该让服务起不来，下次启动会再试。
	logger.Info("threads: 发现未归属线程的邮件，整库重建", zap.Int64("unassigned", n))
	if _, err := RebuildThreads(db); err != nil {
		logger.Warn("threads: 整库重建未完全成功", zap.Error(err))
	}
	return nil
}

// ── 会话列表查询 ─────────────────────────────────────────────────────────────

// ThreadListItem 是会话列表行。主题/摘要/日期/latest_* 取范围内最新一封；
// count/unread/flagged/has_attachment/participants 按账户内整条会话统计（跨文件夹，去重副本）。
type ThreadListItem struct {
	ThreadID       string    `json:"thread_id"`
	AccountID      uint      `json:"account_id"`
	Count          int       `json:"count"`
	Unread         int       `json:"unread"`
	Flagged        bool      `json:"flagged"`
	HasAttachment  bool      `json:"has_attachment"`
	Subject        string    `json:"subject"`
	Snippet        string    `json:"snippet"`
	Date           string    `json:"date"`
	LatestID       uint      `json:"latest_id"`
	LatestFolderID uint      `json:"latest_folder_id"`
	Participants   []Contact `json:"participants"`
}

// ThreadCursor 是会话列表的翻页游标：范围内最新一封的全精度日期 + thread_id。
type ThreadCursor struct {
	BeforeDate   string `json:"before_date"`
	BeforeThread string `json:"before_thread"`
}

// threadHead 是分组查询的一行：thread_id + 范围内最新一封的 id 与日期，再补上那封的展示列。
// LastDate 先按字符串扫：MAX(date) 没有列声明类型，驱动不会替我们解析成 time.Time。
type threadHead struct {
	ThreadID string
	LastKey  string // date || '#' || 12 位 id，见 listThreads
	LastDate string // SQL 里从 LastKey 截出的日期文本（排序与游标键）
	ID       uint
	Latest   *Message `gorm:"-"` // 第二步回表填入；不加 "-" GORM 会把它当关联字段解析
}

// dbTimeLayouts 是 sqlite 驱动（modernc）存/读 time.Time 用的文本布局，首个是写入格式。
var dbTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
}

// parseDBTime 解析驱动落库的时间文本。解析不了返回零值——列表照样能出，只是该行日期显示为零值、
// 以它做游标时下一页为空（等于提前到底）。
func parseDBTime(s string) time.Time {
	for _, l := range dbTimeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// maxParticipants 限制列表行携带的参与者数，长会话不必把几十个人都送到前端。
const maxParticipants = 8

// listThreads 在给定范围（已带 Model + WHERE 的查询）上分组出会话页。
//
// 两步走：先在覆盖索引 idx_msg_folder_thread 内分组，只取 thread_id / MAX(date) / id；
// 再按这 50 个 id 回表取展示列。分组阶段带上 subject/snippet 会逼着 SQLite 逐行回表，
// 实测慢 3～4 倍。
//
// 「最新一封的 id」依赖 SQLite 的特性——聚合查询里出现 MAX() 时，结果集中的裸列取自 MAX 命中的
// 那一行（https://sqlite.org/lang_select.html#bareagg）。本项目只跑 SQLite，换库时改成窗口函数。
//
// 游标：每页都在**整个范围**上分组再用 HAVING 过滤，不做 date <= before_date 预筛。
// 预筛看似能少扫行，实则错误：一条会话最新一封在游标之上、其余成员在游标之下时，预筛砍掉最新
// 那封后重算的 MAX 落到游标下方，这条会话会在下一页再出现一次（多封会话是会话视图的常态，
// 真实收件箱上几乎必现）。排序键与 HAVING 的比较键必须同源：都是 (日期文本, thread_id)。
func (r *Repository) listThreads(scope *gorm.DB, beforeDate *time.Time, beforeThread string, limit int) ([]threadHead, error) {
	if limit <= 0 || limit > 201 {
		limit = 51
	}
	// 「最新一封」按 (date, id) 复合键取而不是单看 date：同一秒到达的多封（自动通知、GreenMail 的
	// INTERNALDATE 只到秒）MAX(date) 平局时裸列会落到任意一行，列表就可能显示原信而不是最新回复。
	// 键 = date 文本 || '#' || 12 位零填充 id；'#' 排在数字前，前缀比较与 ORDER BY date 的字节序一致。
	// last_date 是从键里截回的日期文本（尾部固定 13 字节：'#' + 12 位 id），游标与排序都用它。
	// date 与 rowid 都在覆盖索引里，整个表达式仍不必回表。
	const key = "MAX(messages.date || '#' || printf('%012d', messages.id))"
	q := scope.Where("messages.thread_id <> ''").
		Select("messages.thread_id AS thread_id, " + key + " AS last_key, " +
			"substr(" + key + ", 1, length(" + key + ") - 13) AS last_date, messages.id AS id").
		Group("messages.thread_id")
	if beforeDate != nil {
		q = q.Having("last_date < ? OR (last_date = ? AND messages.thread_id < ?)", dbTime(*beforeDate), dbTime(*beforeDate), beforeThread)
	}
	var heads []threadHead
	if err := q.Order("last_date DESC").Order("messages.thread_id DESC").Limit(limit).Scan(&heads).Error; err != nil {
		return nil, err
	}
	if len(heads) == 0 {
		return heads, nil
	}
	ids := make([]uint, 0, len(heads))
	for i := range heads {
		ids = append(ids, heads[i].ID)
	}
	var latest []Message
	if err := r.db.Where("id IN ?", ids).Find(&latest).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]*Message, len(latest))
	for i := range latest {
		byID[latest[i].ID] = &latest[i]
	}
	for i := range heads {
		heads[i].Latest = byID[heads[i].ID]
	}
	return heads, nil
}

// countThreads 数范围内的会话数（列表标题「共 N 个会话」）。
func (r *Repository) countThreads(scope *gorm.DB) (int64, error) {
	var n int64
	err := scope.Where("messages.thread_id <> ''").Distinct("messages.thread_id").Count(&n).Error
	return n, err
}

// threadSummaryRow 是汇总查询的一行：整条会话内每封（去重后）的状态与发件人。
type threadSummaryRow struct {
	ThreadID      string
	Seen          bool
	Flagged       bool
	HasAttachment bool
	FromName      string
	FromAddr      string
}

// summarize 给一页会话补上账户内整条会话的封数/未读/星标/附件/参与者。
// 一条查询取回全部成员行在 Go 里聚合：一页 50 条会话通常也就一两百行，比 GROUP_CONCAT 再拆字符串清楚。
func (r *Repository) summarize(heads []threadHead) ([]ThreadListItem, error) {
	items := make([]ThreadListItem, 0, len(heads))
	if len(heads) == 0 {
		return items, nil
	}
	tids := make([]string, 0, len(heads))
	for _, h := range heads {
		tids = append(tids, h.ThreadID)
	}
	var rows []threadSummaryRow
	err := dedupeSameMessage(r.db.Model(&Message{})).
		Select("messages.thread_id AS thread_id, messages.seen, messages.flagged, messages.has_attachment, messages.from_name, messages.from_addr").
		Where("messages.thread_id IN ?", tids).
		Order("messages.date ASC").Order("messages.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	agg := make(map[string]*ThreadListItem, len(heads))
	seenAddr := make(map[string]map[string]bool, len(heads))
	for _, h := range heads {
		it := &ThreadListItem{
			ThreadID:     h.ThreadID,
			Date:         parseDBTime(h.LastDate).Format("2006-01-02T15:04:05Z07:00"),
			LatestID:     h.ID,
			Participants: []Contact{},
		}
		// 分组与回表之间那封被删了（并发同步）：行保留，展示列为空，下一次刷新就正常
		if h.Latest != nil {
			it.AccountID, it.LatestFolderID = h.Latest.AccountID, h.Latest.FolderID
			it.Subject, it.Snippet = h.Latest.Subject, h.Latest.Snippet
		}
		agg[h.ThreadID] = it
		seenAddr[h.ThreadID] = map[string]bool{}
	}
	for _, row := range rows {
		it := agg[row.ThreadID]
		if it == nil {
			continue
		}
		it.Count++
		if !row.Seen {
			it.Unread++
		}
		it.Flagged = it.Flagged || row.Flagged
		it.HasAttachment = it.HasAttachment || row.HasAttachment
		key := strings.ToLower(row.FromAddr)
		if key == "" {
			key = row.FromName
		}
		if key != "" && !seenAddr[row.ThreadID][key] && len(it.Participants) < maxParticipants {
			seenAddr[row.ThreadID][key] = true
			it.Participants = append(it.Participants, Contact{Name: row.FromName, Email: row.FromAddr})
		}
	}
	for _, h := range heads {
		items = append(items, *agg[h.ThreadID])
	}
	return items, nil
}

// ThreadPage 是三个会话列表接口共用的结果。
type ThreadPage struct {
	Threads    []ThreadListItem
	NextCursor *ThreadCursor
	Total      int64
}

// threadPage 在范围上完成 分组 → 汇总 → 游标 → 首页总数 的整套流程。
// scopeFn 每次调用都要返回一个新的查询：GORM 的链式调用会污染同一个 *gorm.DB。
func (r *Repository) threadPage(scopeFn func() *gorm.DB, beforeDate *time.Time, beforeThread string, limit int) (*ThreadPage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// 多取一行判断有没有下一页：最后一页正好填满时不再给游标，省掉前端一次空请求
	heads, err := r.listThreads(scopeFn(), beforeDate, beforeThread, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(heads) > limit
	if hasMore {
		heads = heads[:limit]
	}
	items, err := r.summarize(heads)
	if err != nil {
		return nil, err
	}
	page := &ThreadPage{Threads: items}
	if hasMore {
		last := heads[len(heads)-1]
		page.NextCursor = &ThreadCursor{BeforeDate: parseDBTime(last.LastDate).Format(time.RFC3339Nano), BeforeThread: last.ThreadID}
	}
	if beforeDate == nil {
		if page.Total, err = r.countThreads(scopeFn()); err != nil {
			return nil, err
		}
	}
	return page, nil
}

// FolderThreads 单文件夹会话页：范围 = 文件夹 + 筛选。
func (r *Repository) FolderThreads(folderID uint, f Filter, beforeDate *time.Time, beforeThread string, limit int) (*ThreadPage, error) {
	return r.threadPage(func() *gorm.DB { return r.folderScope(folderID, f) }, beforeDate, beforeThread, limit)
}

// dedupeIf 在范围按「每份副本各自可变的状态」（已读 / 星标）筛选时附加副本去重。
//
// 只有这时才需要：Gmail 的标签副本与收件箱副本的已读状态会短暂不一致（本地标了收件箱那份已读，
// 标签文件夹那份要等下次同步），不去重的话「全部未读」里会冒出一条 unread=0 的会话——单封视图
// （总是去重）不会显示它，两种视图对不上。而不按状态筛选时，副本只会让同一线程多命中一次，
// 分组后结果相同；去重的相关子查询每行几微秒，收件箱聚合 7k 行实测 5ms → 38ms，能省就省。
func dedupeIf(q *gorm.DB, need bool) *gorm.DB {
	if need {
		return dedupeSameMessage(q)
	}
	return q
}

// AggregateThreads 聚合视图会话页：范围 = 视图条件 + 筛选（unread/starred 视图或按状态筛选时去重）。
func (r *Repository) AggregateThreads(view string, f Filter, beforeDate *time.Time, beforeThread string, limit int) (*ThreadPage, error) {
	need := view == "unread" || view == "starred" || f.Seen != nil || f.Flagged != nil
	return r.threadPage(func() *gorm.DB {
		return f.apply(dedupeIf(aggregateScope(r.db.Model(&Message{}), view), need))
	}, beforeDate, beforeThread, limit)
}

// SearchThreads 搜索结果会话页：范围 = 搜索条件 + 筛选 + 副本去重。
// 搜索跨全部文件夹，Gmail 副本命中时不去重的话 latest_id 可能落到「所有邮件」那份，点开的文件夹
// 上下文就变了；与单封搜索同样无条件去重（from:github 1.8k 条命中实测 11ms → 23ms，可以接受）。
func (r *Repository) SearchThreads(q fts.Query, f Filter, beforeDate *time.Time, beforeThread string, limit int) (*ThreadPage, error) {
	return r.threadPage(func() *gorm.DB { return f.apply(dedupeSameMessage(r.searchScope(q))) }, beforeDate, beforeThread, limit)
}

// threadMessagesCap 是一次展开返回的成员上限：邮件列表讨论能到几百封，手风琴一次渲染这么多也没意义。
const threadMessagesCap = 500

// ThreadMessages 返回一条会话的成员（账户内跨文件夹，去重副本），日期升序，最多 limit 封——Reader 手风琴用。
func (r *Repository) ThreadMessages(threadID string, limit int) ([]Message, error) {
	if limit <= 0 || limit > threadMessagesCap {
		limit = threadMessagesCap
	}
	var list []Message
	err := dedupeSameMessage(r.db.Model(&Message{})).
		Where("messages.thread_id = ?", threadID).
		Order("messages.date ASC").Order("messages.id ASC").Limit(limit).Find(&list).Error
	return list, err
}

// ThreadMembers 返回若干会话的全部成员行（不去重：改标志位要连副本一起改，本地各文件夹的未读数才对）。
// 会话级操作用它解析出 message id 后复用既有的批量操作。
func (r *Repository) ThreadMembers(threadIDs []string) ([]Message, error) {
	if len(threadIDs) == 0 {
		return nil, nil
	}
	var list []Message
	err := r.db.Where("thread_id IN ?", threadIDs).Order("id ASC").Find(&list).Error
	return list, err
}
