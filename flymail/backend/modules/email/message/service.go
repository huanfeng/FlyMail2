package message

import (
	"encoding/json"
	"flymail/internal/fts"
	"regexp"
	"strings"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"
)

const (
	defaultSyncDepth = 1000
	fetchBatchSize   = 200
)

// IMAPFetcher 是邮件元数据同步所需的最小 IMAP 能力（便于测试 mock）。*coreimap.Session 满足此接口。
type IMAPFetcher interface {
	SelectFolder(path string) (*coreimap.SelectedFolder, error)
	FolderStatus(path string, items ...coreimap.StatusItem) (*coreimap.FolderStatusResult, error)
	FetchByUIDRange(from, to imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error)
	FetchBySeqRange(from, to uint32, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error)
}

// FolderState 是单文件夹同步后回写文件夹表所需的状态。
type FolderState struct {
	UIDValidity uint32
	UIDNext     uint32
	Total       int
	Unread      int
}

type Service struct {
	repo      *Repository
	bodyRepo  *BodyRepository
	syncDepth int
}

func NewService(repo *Repository, bodyRepo *BodyRepository) *Service {
	return &Service{repo: repo, bodyRepo: bodyRepo, syncDepth: defaultSyncDepth}
}

// SetSyncDepth 覆盖同步深度（最近 N 封）；n<=0 时忽略。
func (s *Service) SetSyncDepth(n int) {
	if n > 0 {
		s.syncDepth = n
	}
}

// SyncFolderMessages 同步单个文件夹最近 ~defaultSyncDepth 封邮件的元数据。
// prevUIDValidity 为本地已存的该文件夹 UIDVALIDITY（0=从未同步）。
// 返回：同步后状态、是否因 UIDVALIDITY 变化而重建、错误。
func (s *Service) SyncFolderMessages(accountID, folderID uint, folderPath string, prevUIDValidity uint32, c IMAPFetcher) (*FolderState, bool, error) {
	sel, err := c.SelectFolder(folderPath)
	if err != nil {
		return nil, false, err
	}

	uidValidity := sel.UIDValidity
	if uidValidity == 0 {
		if st, serr := c.FolderStatus(folderPath, coreimap.StatusUIDValidity); serr == nil && st != nil && st.UIDValidity != nil {
			uidValidity = *st.UIDValidity
		}
	}

	rebuilt := false
	if prevUIDValidity != 0 && uidValidity != 0 && uidValidity != prevUIDValidity {
		if err := s.repo.DeleteByFolder(folderID); err != nil {
			return nil, false, err
		}
		rebuilt = true
	}

	// UIDNEXT 兜底：部分服务商 SELECT 不返回 UIDNEXT，尝试 STATUS。
	uidNext := sel.UIDNext
	if uidNext == 0 {
		if st, serr := c.FolderStatus(folderPath, coreimap.StatusUIDNext); serr == nil && st != nil && st.UIDNext != nil {
			uidNext = *st.UIDNext
		}
	}

	if sel.NumMessages > 0 {
		if uidNext > 0 {
			// 已知 UIDNEXT：按 UID 区间抓最近 ~depth 封。
			from := imapv2.UID(1)
			if uidNext > uint32(s.syncDepth) {
				from = imapv2.UID(uidNext - uint32(s.syncDepth))
			}
			end := imapv2.UID(uidNext - 1)
			if err := s.fetchRangeBatched(accountID, folderID, from, end, c); err != nil {
				return nil, rebuilt, err
			}
		} else {
			// 服务商不报 UIDNEXT（如网易 163）：按序号抓最近 ~depth 封。
			// 序号区间 [total-depth+1, total]，FETCH 响应仍带真实 UID。
			total := sel.NumMessages
			seqFrom := uint32(1)
			if total > uint32(s.syncDepth) {
				seqFrom = total - uint32(s.syncDepth) + 1
			}
			if err := s.fetchSeqRangeBatched(accountID, folderID, seqFrom, total, c); err != nil {
				return nil, rebuilt, err
			}
		}
	}

	total, _ := s.repo.CountByFolder(folderID, Filter{})
	unread, _ := s.repo.UnreadCountByFolder(folderID)
	// UIDNEXT 未知时，用本地已存的最大 UID + 1 作为锚点（供后续增量同步）。
	// 本地为空时也落 1：uid_next 非 0 是「这个文件夹同步过」的标记，基线判定靠它；
	// 留 0 的话空收件箱之后到的第一批邮件会被当成基线导入（不提醒、不跑规则）。
	if uidNext == 0 {
		maxUID, _ := s.repo.MaxUID(folderID)
		uidNext = maxUID + 1
	}
	return &FolderState{
		UIDValidity: uidValidity,
		UIDNext:     uidNext,
		Total:       int(total),
		Unread:      int(unread),
	}, rebuilt, nil
}

// NewMail 描述一轮增量同步新增的邮件情况，供「新邮件」提醒判断使用。
// Baseline（基线导入/UIDVALIDITY 重建）不应触发提醒——旧账户历史邮件不是新邮件。
type NewMail struct {
	Count    int  // 本轮入库的新增邮件数（含已读）
	Baseline bool // 是否基线导入：本地原本为空，或 UIDVALIDITY 变化后重建
	// AfterID 是本轮同步前该文件夹的最大主键：id > AfterID 的行就是这一轮新入库的。
	// 正文预取的 new 模式据此圈定「仅新邮件」，不会误伤历史邮件。
	AfterID     uint
	Unseen      []Message // 新增中的未读邮件（升序，最多 newMailUnseenCap 封，供单封精准通知）
	UnseenTotal int       // 新增未读总数
}

// newMailUnseenCap 限制 Unseen 明细条数（仅通知文案需要，不必全量拉取）。
const newMailUnseenCap = 3

// IncrementalSync 增量同步单文件夹：只抓取本地之后新增的邮件。
// prev* 为本地已存的该文件夹状态（来自 folders 表）。
// 返回：同步后状态、新增邮件情况（NewMail）、错误。
// UIDVALIDITY 变化时删除本地缓存并退化为完整重建。
func (s *Service) IncrementalSync(accountID, folderID uint, folderPath string, prevUIDValidity, prevUIDNext uint32, prevTotal int, c IMAPFetcher) (*FolderState, *NewMail, error) {
	sel, err := c.SelectFolder(folderPath)
	if err != nil {
		return nil, nil, err
	}

	uidValidity := sel.UIDValidity
	if uidValidity == 0 {
		if st, serr := c.FolderStatus(folderPath, coreimap.StatusUIDValidity); serr == nil && st != nil && st.UIDValidity != nil {
			uidValidity = *st.UIDValidity
		}
	}

	// UIDVALIDITY 变化：本地缓存失效，删除后完整重建。重建属基线导入，不触发新邮件提醒。
	if prevUIDValidity != 0 && uidValidity != 0 && uidValidity != prevUIDValidity {
		if err := s.repo.DeleteByFolder(folderID); err != nil {
			return nil, nil, err
		}
		state, _, err := s.SyncFolderMessages(accountID, folderID, folderPath, 0, c)
		if err != nil {
			return nil, nil, err
		}
		return state, &NewMail{Count: state.Total, Baseline: true}, nil
	}

	beforeCount, _ := s.repo.CountByFolder(folderID, Filter{})
	beforeMaxID, _ := s.repo.MaxIDByFolder(folderID)

	uidNext := sel.UIDNext
	if uidNext == 0 {
		if st, serr := c.FolderStatus(folderPath, coreimap.StatusUIDNext); serr == nil && st != nil && st.UIDNext != nil {
			uidNext = *st.UIDNext
		}
	}

	if uidNext > 0 {
		// 已知 UIDNEXT：抓 [anchor, uidNext-1]，anchor=prevUIDNext（无则本地 maxUID+1）。
		anchor := prevUIDNext
		if anchor == 0 {
			if maxUID, _ := s.repo.MaxUID(folderID); maxUID > 0 {
				anchor = maxUID + 1
			} else {
				anchor = 1
			}
		}
		if uidNext > anchor {
			if err := s.fetchRangeBatched(accountID, folderID, imapv2.UID(anchor), imapv2.UID(uidNext-1), c); err != nil {
				return nil, nil, err
			}
		}
	} else {
		// 无 UIDNEXT（163）：用本地已存最大 UID 作锚点，抓 UID 区间 [maxUID+1, *]——
		// 服务器只返回该区间内实际存在的 UID（即新到邮件），结果有界、不会每轮全量重抓。
		// 首次同步（本地为空）则按序号抓最近 syncDepth 封作基线。
		maxUID, _ := s.repo.MaxUID(folderID)
		if maxUID > 0 {
			emails, ferr := c.FetchByUIDRange(imapv2.UID(maxUID+1), 0, coreimap.FetchOptions{FetchBody: false, FallbackHeaders: true})
			if ferr != nil {
				return nil, nil, ferr
			}
			if err := s.upsertBatch(accountID, folderID, emails); err != nil {
				return nil, nil, err
			}
		} else {
			currentTotal := int(sel.NumMessages)
			if currentTotal > 0 {
				from := uint32(1)
				if currentTotal > s.syncDepth {
					from = uint32(currentTotal - s.syncDepth + 1)
				}
				if err := s.fetchSeqRangeBatched(accountID, folderID, from, uint32(currentTotal), c); err != nil {
					return nil, nil, err
				}
			}
		}
	}

	total, _ := s.repo.CountByFolder(folderID, Filter{})
	unread, _ := s.repo.UnreadCountByFolder(folderID)
	newCount := int(total) - int(beforeCount)
	if newCount < 0 {
		newCount = 0
	}
	// 基线判定：该文件夹从未同步过（本地为空且没有 UIDNEXT 锚点）的首次导入不算「新邮件」——
	// 旧账户的历史邮件不该提醒，也不该被规则引擎处置。只看「本地为空」不够：一个同步时还是空的
	// 收件箱，之后到的第一批邮件也会被当成基线，既不提醒也不跑规则。
	nm := &NewMail{Count: newCount, Baseline: beforeCount == 0 && prevUIDNext == 0, AfterID: beforeMaxID}
	if !nm.Baseline && newCount > 0 {
		if n, err := s.repo.CountUnseenAfterID(folderID, beforeMaxID); err == nil {
			nm.UnseenTotal = int(n)
		}
		if nm.UnseenTotal > 0 {
			nm.Unseen, _ = s.repo.UnseenAfterID(folderID, beforeMaxID, newMailUnseenCap)
		}
	}
	if uidNext == 0 {
		// 同 SyncFolderMessages：空文件夹也落 1，标记「同步过」
		maxUID, _ := s.repo.MaxUID(folderID)
		uidNext = maxUID + 1
	}
	return &FolderState{
		UIDValidity: uidValidity,
		UIDNext:     uidNext,
		Total:       int(total),
		Unread:      int(unread),
	}, nm, nil
}

// upsertBatch 把一批抓回来的邮件 upsert 入库，再整批做线程归属。
// 归属放在 upsert 之后而不是拼进 Upsert 语句：要先有主键与既有 thread_id 才能决定是沿用、合并还是新开。
func (s *Service) upsertBatch(accountID, folderID uint, emails []*types.ParsedEmail) error {
	if len(emails) == 0 {
		return nil
	}
	uids := make([]uint32, 0, len(emails))
	for _, e := range emails {
		if err := s.repo.Upsert(toMessage(accountID, folderID, e)); err != nil {
			return err
		}
		uids = append(uids, e.UID)
	}
	rows, err := s.repo.LoadByFolderUIDs(folderID, uids)
	if err != nil {
		return err
	}
	return s.repo.AssignThreads(rows)
}

// fetchSeqRangeBatched 把序号区间 [from,end] 切成 fetchBatchSize 的子区间逐批抓取并 upsert。
func (s *Service) fetchSeqRangeBatched(accountID, folderID uint, from, end uint32, c IMAPFetcher) error {
	for start := from; start <= end; {
		batchEnd := start + fetchBatchSize - 1
		if batchEnd > end || batchEnd < start {
			batchEnd = end
		}
		emails, err := c.FetchBySeqRange(start, batchEnd, coreimap.FetchOptions{FetchBody: false, FallbackHeaders: true})
		if err != nil {
			return err
		}
		if err := s.upsertBatch(accountID, folderID, emails); err != nil {
			return err
		}
		if batchEnd == end {
			break
		}
		start = batchEnd + 1
	}
	return nil
}

// fetchRangeBatched 把 [from,end] 切成 fetchBatchSize 的子区间逐批抓取并 upsert。
func (s *Service) fetchRangeBatched(accountID, folderID uint, from, end imapv2.UID, c IMAPFetcher) error {
	for start := from; start <= end; {
		batchEnd := start + fetchBatchSize - 1
		if batchEnd > end || batchEnd < start { // 上限裁剪 + uint32 溢出保护
			batchEnd = end
		}
		emails, err := c.FetchByUIDRange(start, batchEnd, coreimap.FetchOptions{FetchBody: false, FallbackHeaders: true})
		if err != nil {
			return err
		}
		if err := s.upsertBatch(accountID, folderID, emails); err != nil {
			return err
		}
		if batchEnd == end {
			break
		}
		start = batchEnd + 1
	}
	return nil
}

// List 返回文件夹内的邮件列表项（UID 游标分页），f 为零值时不筛选。
func (s *Service) List(folderID uint, beforeUID uint32, limit int, f Filter) ([]MessageListItem, error) {
	rows, err := s.repo.ListByFolder(folderID, beforeUID, limit, f)
	if err != nil {
		return nil, err
	}
	out := make([]MessageListItem, 0, len(rows))
	for i := range rows {
		out = append(out, toListItem(&rows[i]))
	}
	return out, nil
}

// AggCursor 是聚合列表的不透明翻页游标，由最后一行的全精度日期 + 主键 ID 组成。
// 用全精度日期（RFC3339Nano）规避列表 DTO 日期被截断到秒导致的 keyset 边界错位。
type AggCursor struct {
	BeforeDate string `json:"before_date"`
	BeforeID   uint   `json:"before_id"`
}

// ListAggregate 返回跨账户聚合列表项 + 下一页游标（无更多时为 nil）。
// view: inbox / unread / starred。beforeDate==nil 取首页。
func (s *Service) ListAggregate(view string, beforeDate *time.Time, beforeID uint, limit int, f Filter) ([]MessageListItem, *AggCursor, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.repo.ListAggregate(view, beforeDate, beforeID, limit, f)
	if err != nil {
		return nil, nil, err
	}
	out := make([]MessageListItem, 0, len(rows))
	for i := range rows {
		out = append(out, toListItem(&rows[i]))
	}
	var cur *AggCursor
	if len(rows) == limit {
		last := rows[len(rows)-1]
		cur = &AggCursor{BeforeDate: last.Date.Format(time.RFC3339Nano), BeforeID: last.ID}
	}
	return out, cur, nil
}

// ListSearch 跨账户全文检索，返回列表项 + 下一页游标（与聚合同款 keyset）。
// q 是用户原始输入（含 from:/is:/before: 等限定符，见 fts.Parse）；解析后没有任何有效条件
// 时直接返回空结果——「搜一堆标点」不该退化成列出全部邮件。
func (s *Service) ListSearch(q string, beforeDate *time.Time, beforeID uint, limit int, f Filter) ([]MessageListItem, *AggCursor, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	parsed := fts.Parse(q)
	if parsed.Empty() {
		return []MessageListItem{}, nil, nil
	}
	rows, err := s.repo.SearchMessages(parsed, beforeDate, beforeID, limit, f)
	if err != nil {
		return nil, nil, err
	}
	out := make([]MessageListItem, 0, len(rows))
	for i := range rows {
		out = append(out, toListItem(&rows[i]))
	}
	var cur *AggCursor
	if len(rows) == limit {
		last := rows[len(rows)-1]
		cur = &AggCursor{BeforeDate: last.Date.Format(time.RFC3339Nano), BeforeID: last.ID}
	}
	return out, cur, nil
}

// DeleteByID 删除单封邮件的本地行（移动/删除成功后调用）。
func (s *Service) DeleteByID(id uint) error { return s.repo.DeleteByID(id) }

// ListAfterID 返回文件夹内本轮新入库的邮件（id > afterID，升序，最多 limit 封）。
func (s *Service) ListAfterID(folderID uint, afterID uint, limit int) ([]Message, error) {
	return s.repo.ListAfterID(folderID, afterID, limit)
}

// ListRecentInbox 返回收件箱最近的邮件（accountID = 0 表示全部账户），规则试运行用。
func (s *Service) ListRecentInbox(accountID uint, limit int) ([]Message, error) {
	return s.repo.ListRecentInbox(accountID, limit)
}

// RefreshNewMail 在规则引擎改动过本轮新邮件（移走 / 删除 / 标已读）之后重算未读部分，
// 让后面的通知闸门看到的是执行后的状态。
func (s *Service) RefreshNewMail(folderID uint, nm *NewMail) {
	nm.Unseen, nm.UnseenTotal = nil, 0
	if n, err := s.repo.CountUnseenAfterID(folderID, nm.AfterID); err == nil {
		nm.UnseenTotal = int(n)
	}
	if nm.UnseenTotal > 0 {
		nm.Unseen, _ = s.repo.UnseenAfterID(folderID, nm.AfterID, newMailUnseenCap)
	}
}

// BodyText 返回一封邮件用于规则匹配的正文文本：优先纯文本，否则剥掉 HTML 标签。
// 正文尚未落库时 known 为 false，调用方据此把正文条件当作「未知」。
func (s *Service) BodyText(messageID uint) (text string, known bool) {
	b, err := s.bodyRepo.GetByMessageID(messageID)
	if err != nil || b == nil {
		return "", false
	}
	if b.TextBody != "" {
		return b.TextBody, true
	}
	return fts.StripHTML(b.HTMLBody), true
}

// AttachmentNames 返回一封邮件的附件文件名（含内联部件）。
func (s *Service) AttachmentNames(messageID uint) []string {
	atts, err := s.bodyRepo.ListAttachments(messageID)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(atts))
	for _, a := range atts {
		if a.Filename != "" {
			names = append(names, a.Filename)
		}
	}
	return names
}

// SearchContacts 收件人自动补全：返回历史往来联系人（按频率降序）。
func (s *Service) SearchContacts(q string, limit int) ([]Contact, error) {
	return s.repo.SearchContacts(q, limit)
}

// CountByFolder 返回文件夹内邮件总数（应用筛选后）。f 为零值时即全量。
func (s *Service) CountByFolder(folderID uint, f Filter) (int64, error) {
	return s.repo.CountByFolder(folderID, f)
}

// CountSearchMessages 返回搜索命中的总条数（应用筛选后）。空查询计 0，与 ListSearch 口径一致。
func (s *Service) CountSearchMessages(q string, f Filter) (int64, error) {
	parsed := fts.Parse(q)
	if parsed.Empty() {
		return 0, nil
	}
	return s.repo.CountSearchMessages(parsed, f)
}

// RebuildSearchIndex 整体重建全文索引（运维入口：索引与主表漂移时使用）。
func (s *Service) RebuildSearchIndex() error {
	return RebuildFTS(s.repo.db)
}

// CountAggregateView 返回某聚合视图应用筛选后的条目总数，供列表标题「共 N 封」使用。
// 与 AggregateCounts 的入口徽标不同：徽标是固定语义（如 inbox 显示未读数）且不受筛选影响，
// 这里要的是「当前列表实际有多少条」。
func (s *Service) CountAggregateView(view string, f Filter) (int64, error) {
	return s.repo.CountAggregateTotal(view, f)
}

// AggregateCounts 返回三个聚合入口的徽标计数，外加收件箱聚合的条目总数。
//
// inbox 键是未读数（入口徽标的语义），列表标题要显示的「共几封」是另一回事，
// 因此单独给出 inbox_total；unread / starred 两个视图里每一条都符合该条件，
// 徽标数本身就是总数，无需另算。
func (s *Service) AggregateCounts() (map[string]int64, error) {
	out := make(map[string]int64, 4)
	for _, v := range []string{"inbox", "unread", "starred"} {
		n, err := s.repo.CountAggregate(v)
		if err != nil {
			return nil, err
		}
		out[v] = n
	}
	total, err := s.repo.CountAggregateTotal("inbox", Filter{})
	if err != nil {
		return nil, err
	}
	out["inbox_total"] = total
	return out, nil
}

// PendingBodies 返回该账户缺正文、落在预取范围内的邮件（recent/all 模式用）。
func (s *Service) PendingBodies(accountID uint, sinceDays, limit int) ([]Message, error) {
	return s.repo.PendingBodies(accountID, sinceDays, limit)
}

// PendingBodiesAfterID 返回某文件夹本轮新增中缺正文的邮件（new 模式用）。
func (s *Service) PendingBodiesAfterID(folderID uint, afterID uint, limit int) ([]Message, error) {
	return s.repo.PendingBodiesAfterID(folderID, afterID, limit)
}

// CountPendingBodies 返回该账户预取范围内还缺多少封正文。
func (s *Service) CountPendingBodies(accountID uint, sinceDays int) (int64, error) {
	return s.repo.CountPendingBodies(accountID, sinceDays)
}

// DeleteByIDs 批量删除本地元数据行（批量删除/移动用）。
func (s *Service) DeleteByIDs(ids []uint) error { return s.repo.DeleteByIDs(ids) }

// SetSeenByIDs 批量置已读/未读。
func (s *Service) SetSeenByIDs(ids []uint, seen bool) error {
	return s.repo.SetSeenByIDs(ids, seen)
}

// SetFlaggedByIDs 批量置星标。
func (s *Service) SetFlaggedByIDs(ids []uint, flagged bool) error {
	return s.repo.SetFlaggedByIDs(ids, flagged)
}

// AccountUnreadCounts 返回各账户的未读数（account_id → 未读），供侧栏账户角标使用。
func (s *Service) AccountUnreadCounts() (map[uint]int64, error) {
	return s.repo.AccountUnreadCounts()
}

// UIDsNotSearchable 透传仓储：给定 UID 中本地搜不到（无行或无正文）的那些。
func (s *Service) UIDsNotSearchable(folderID uint, uids []uint32) ([]uint32, error) {
	return s.repo.UIDsNotSearchable(folderID, uids)
}

// StoreFetched 把一封抓回来的邮件整体入库：元数据 upsert，若带正文则一并落库并标记 body_synced。
// 供服务端搜索兜底使用——命中的邮件本地可能根本没有行，也可能有行但没正文，
// 两种情况统一走这里，回来即可被全文索引命中。返回本地行（含 ID）。
func (s *Service) StoreFetched(accountID, folderID uint, e *types.ParsedEmail, withBody bool) (*Message, error) {
	if err := s.repo.Upsert(toMessage(accountID, folderID, e)); err != nil {
		return nil, err
	}
	m, err := s.repo.GetByFolderUID(folderID, e.UID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.AssignThreads([]Message{*m}); err != nil {
		return nil, err
	}
	if withBody {
		if err := s.StoreParsedBody(m.ID, e); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// ── 会话线程 ─────────────────────────────────────────────────────────────────

// ListFolderThreads 单文件夹会话列表。
func (s *Service) ListFolderThreads(folderID uint, f Filter, beforeDate *time.Time, beforeThread string, limit int) (*ThreadPage, error) {
	return s.repo.FolderThreads(folderID, f, beforeDate, beforeThread, limit)
}

// ListAggregateThreads 聚合视图会话列表（view: inbox / unread / starred）。
func (s *Service) ListAggregateThreads(view string, f Filter, beforeDate *time.Time, beforeThread string, limit int) (*ThreadPage, error) {
	return s.repo.AggregateThreads(view, f, beforeDate, beforeThread, limit)
}

// ListSearchThreads 搜索结果按会话折叠。空查询返回空页，与 ListSearch 口径一致。
func (s *Service) ListSearchThreads(q string, f Filter, beforeDate *time.Time, beforeThread string, limit int) (*ThreadPage, error) {
	parsed := fts.Parse(q)
	if parsed.Empty() {
		return &ThreadPage{Threads: []ThreadListItem{}}, nil
	}
	return s.repo.SearchThreads(parsed, f, beforeDate, beforeThread, limit)
}

// ThreadMessages 返回一条会话的成员列表项（日期升序，跨文件夹去重，最多 limit 封）。
func (s *Service) ThreadMessages(threadID string, limit int) ([]MessageListItem, error) {
	rows, err := s.repo.ThreadMessages(threadID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]MessageListItem, 0, len(rows))
	for i := range rows {
		out = append(out, toListItem(&rows[i]))
	}
	return out, nil
}

// ThreadMembers 返回若干会话的全部成员行（含副本），供会话级操作解析 id。
func (s *Service) ThreadMembers(threadIDs []string) ([]Message, error) {
	return s.repo.ThreadMembers(threadIDs)
}

// RebuildThreads 整库重建线程归属（运维入口）。
func (s *Service) RebuildThreads() (int, error) {
	return RebuildThreads(s.repo.db)
}

// StoreParsedBody 落正文+附件，回填 snippet/has_attachment/body_synced。
//
// 顺带补线程头：元数据同步靠 HEADER.FIELDS 拿 In-Reply-To/References，但有的服务器（GreenMail 实测）
// 对这个区段一律回空，ENVELOPE 也不带 In-Reply-To。整封正文里头总是全的，若行上还没有线程头而
// 这里解析出来了，就回填并重新归属——这类服务器上的会话会在正文预取/打开邮件后归并。
func (s *Service) StoreParsedBody(messageID uint, e *types.ParsedEmail) error {
	if e.InReplyTo != "" || e.References != "" {
		m, err := s.repo.GetByID(messageID)
		if err != nil {
			return err
		}
		// 两列各自独立判断：ENVELOPE 给了 In-Reply-To 但 HEADER.FIELDS 回空的服务器，行上只有 in_reply_to；
		// 要求两列都空才补的话 References 永远补不上，「父邮件不在本地、祖父在」的回复就归不进去。
		irt, refs := m.InReplyTo, m.References
		if irt == "" {
			irt = e.InReplyTo
		}
		if refs == "" {
			refs = e.References
		}
		if irt != m.InReplyTo || refs != m.References {
			if err := s.repo.SetThreadHeaders(m.ID, irt, refs); err != nil {
				return err
			}
			m.InReplyTo, m.References = irt, refs
			if err := s.repo.AssignThreads([]Message{*m}); err != nil {
				return err
			}
		}
	}
	if err := s.bodyRepo.Upsert(&MessageBody{MessageID: messageID, TextBody: e.TextBody, HTMLBody: e.HTMLBody}); err != nil {
		return err
	}
	atts := make([]Attachment, 0, len(e.Attachments))
	for _, a := range e.Attachments {
		atts = append(atts, Attachment{
			MessageID:   messageID,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        a.Size,
			ContentID:   a.ContentID,
			IsInline:    a.IsInline,
		})
	}
	if err := s.bodyRepo.ReplaceAttachments(messageID, atts); err != nil {
		return err
	}
	return s.repo.MarkBodySynced(messageID, makeSnippet(e.TextBody, e.HTMLBody), len(atts) > 0)
}

// Detail 从本地组装邮件详情。
func (s *Service) Detail(messageID uint) (*MessageDetail, error) {
	m, err := s.repo.GetByID(messageID)
	if err != nil {
		return nil, err
	}
	item := toListItem(m)
	d := &MessageDetail{
		MessageListItem: item,
		BodySynced:      m.BodySynced,
		Attachments:     []Attachment{},
		MessageID:       m.MessageID,
		InReplyTo:       m.InReplyTo,
		References:      m.References,
		ThreadID:        m.ThreadID,
	}
	if b, _ := s.bodyRepo.GetByMessageID(messageID); b != nil {
		d.TextBody = b.TextBody
		d.HTMLBody = b.HTMLBody
	}
	if atts, _ := s.bodyRepo.ListAttachments(messageID); len(atts) > 0 {
		d.Attachments = atts
	}
	if m.CcJSON != "" {
		_ = json.Unmarshal([]byte(m.CcJSON), &d.Cc)
	}
	return d, nil
}

// CountByAccount 返回账户下全部邮件数量。
func (s *Service) CountByAccount(accountID uint) (int64, error) {
	return s.repo.CountByAccount(accountID)
}

// GetByID 透传单封邮件原始记录。
func (s *Service) GetByID(id uint) (*Message, error) { return s.repo.GetByID(id) }

// GetByIDs 批量取行，不存在的缺席。
func (s *Service) GetByIDs(ids []uint) ([]Message, error) { return s.repo.GetByIDs(ids) }

// SetSeenLocal 本地标记已读/未读。
func (s *Service) SetSeenLocal(id uint, seen bool) error { return s.repo.SetSeen(id, seen) }

// UnreadCountByFolder 返回文件夹当前未读邮件数。
func (s *Service) UnreadCountByFolder(folderID uint) (int64, error) {
	return s.repo.UnreadCountByFolder(folderID)
}

// SetFlaggedLocal 本地标记星标/取消星标。
func (s *Service) SetFlaggedLocal(id uint, flagged bool) error {
	return s.repo.SetFlagged(id, flagged)
}

var reHTML = regexp.MustCompile(`<[^>]*>`)

// stripHTML 简单去除 HTML 标签。
func stripHTML(html string) string {
	return reHTML.ReplaceAllString(html, " ")
}

// makeSnippet 生成不超过 150 字的摘要。
func makeSnippet(text, html string) string {
	s := text
	if s == "" {
		s = stripHTML(html)
	}
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 150 {
		return string(r[:150]) + "…"
	}
	return s
}

func toMessage(accountID, folderID uint, e *types.ParsedEmail) *Message {
	m := &Message{
		AccountID:  accountID,
		FolderID:   folderID,
		UID:        e.UID,
		MessageID:  e.MessageID,
		InReplyTo:  e.InReplyTo,
		References: e.References,
		Subject:    e.Subject,
		Date:       e.Date,
		Size:       e.Size,
		Seen:       e.IsRead,
		Flagged:    e.IsStarred,
	}
	if len(e.From) > 0 {
		m.FromName = e.From[0].Name
		m.FromAddr = e.From[0].Email
	}
	if b, err := json.Marshal(e.To); err == nil {
		m.ToJSON = string(b)
	}
	if b, err := json.Marshal(e.CC); err == nil {
		m.CcJSON = string(b)
	}
	for _, f := range e.Flags {
		switch f {
		case "\\Answered":
			m.Answered = true
		case "\\Deleted":
			m.Deleted = true
		}
	}
	return m
}
