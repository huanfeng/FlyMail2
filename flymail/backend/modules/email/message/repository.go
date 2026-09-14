package message

import (
	"errors"
	"strings"
	"time"

	"flymail/internal/fts"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrMessageNotFound = errors.New("message not found")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Upsert 按 (folder_id, uid) 唯一键插入或更新元数据。
// 不更新 body_synced/snippet/has_attachment（正文相关，由 M4 流程维护）。
func (r *Repository) Upsert(m *Message) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "folder_id"}, {Name: "uid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"account_id", "message_id", "in_reply_to", "references_hdr", "subject",
			"from_name", "from_addr", "to_json", "cc_json", "date", "size",
			"seen", "flagged", "answered", "deleted", "updated_at",
		}),
	}).Create(m).Error
}

// GetByFolderUID 按 (folder_id, uid) 唯一键取一封。
func (r *Repository) GetByFolderUID(folderID uint, uid uint32) (*Message, error) {
	var m Message
	err := r.db.Where("folder_id = ? AND uid = ?", folderID, uid).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// uidChunk 是一次 IN 查询最多带的 UID 数，远低于 SQLite 绑定变量上限，留足其它参数的余量。
const uidChunk = 900

// UIDsNotSearchable 从给定 UID 中挑出「本地搜不到」的那些：本地没有这一行，或有行但正文尚未落库
// （body_synced=false 时全文索引里没有正文，服务器按正文命中的邮件本地依然搜不出来）。
// 服务端搜索兜底用它决定哪些命中需要补抓。
func (r *Repository) UIDsNotSearchable(folderID uint, uids []uint32) ([]uint32, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	searchable := make(map[uint32]bool, len(uids))
	// 分块查询：IN 列表的每个元素都是一个绑定变量，SQLite 上限 32766；
	// 服务器上一个常用词命中几万封并不稀奇。
	for start := 0; start < len(uids); start += uidChunk {
		end := start + uidChunk
		if end > len(uids) {
			end = len(uids)
		}
		var rows []struct {
			UID        uint32
			BodySynced bool
		}
		err := r.db.Model(&Message{}).Select("uid, body_synced").
			Where("folder_id = ? AND uid IN ?", folderID, uids[start:end]).Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.BodySynced {
				searchable[row.UID] = true
			}
		}
	}
	out := make([]uint32, 0, len(uids))
	for _, u := range uids {
		if !searchable[u] {
			out = append(out, u)
		}
	}
	return out, nil
}

func (r *Repository) DeleteByFolder(folderID uint) error {
	return r.db.Where("folder_id = ?", folderID).Delete(&Message{}).Error
}

// DeleteByID 删除单封邮件的本地元数据行（移动/删除成功后清理本地缓存）。
// 正文/附件随 message_id 外键留存，下次该 message 不再出现即可，可由后续清理流程回收。
func (r *Repository) DeleteByID(id uint) error {
	return r.db.Where("id = ?", id).Delete(&Message{}).Error
}

// DeleteByIDs 批量删除本地元数据行：批量删除/移动时一条 SQL 搞定，
// 避免几十封逐条开事务（SQLite 上每条都是一次 fsync，几十封能拖出可感知的卡顿）。
func (r *Repository) DeleteByIDs(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.Where("id IN ?", ids).Delete(&Message{}).Error
}

// SetSeenByIDs 批量置已读/未读（同上，合并成一条 UPDATE）。
func (r *Repository) SetSeenByIDs(ids []uint, seen bool) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.Model(&Message{}).Where("id IN ?", ids).Update("seen", seen).Error
}

// SetFlaggedByIDs 批量置星标（同上）。
func (r *Repository) SetFlaggedByIDs(ids []uint, flagged bool) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.Model(&Message{}).Where("id IN ?", ids).Update("flagged", flagged).Error
}

// searchScope 构造搜索的匹配条件，供列表查询与计数共用。
// 抽出来是因为两处各写一遍这串条件迟早会漂移，届时「共 N 封」会与实际能翻到的条目数对不上。
//
// 文本条件走 FTS5：messages_fts MATCH（索引命中的 rowid 即 messages.id），
// 结构化条件（未读/星标/附件/日期/文件夹/账户）落到主表 WHERE，两者 AND。
// 用 IN 子查询而不是 JOIN 虚表：与既有的 dedupeSameMessage / Filter 拼装方式正交，
// 并且 SQLite 会把子查询物化一次再探测主表，不会为每行重跑一遍 MATCH。
func (r *Repository) searchScope(q fts.Query) *gorm.DB {
	dbq := r.db.Model(&Message{})
	if m := q.Match(); m != "" {
		dbq = dbq.Where("messages.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)", m)
	}
	if q.Seen != nil {
		dbq = dbq.Where("messages.seen = ?", *q.Seen)
	}
	if q.Flagged != nil {
		dbq = dbq.Where("messages.flagged = ?", *q.Flagged)
	}
	if q.HasAttachment != nil {
		dbq = dbq.Where("messages.has_attachment = ?", *q.HasAttachment)
	}
	// before:/after: 是「按邮件自带时区的日期」比较，不是绝对时刻：date 列以带偏移的文本存储
	// （2026-03-01 21:00:00+08:00），比较走字节序，串尾的偏移量不参与主序。
	// 这与 IMAP SENTBEFORE/SENTSINCE 按 Date 头日期比较的口径一致，本地与服务端结果不会打架；
	// 代价是跨时区邮件在日期边界上可能差一天。既有的 ORDER BY date 也是同一口径。
	if q.Before != nil {
		dbq = dbq.Where("messages.date < ?", *q.Before)
	}
	if q.After != nil {
		dbq = dbq.Where("messages.date >= ?", *q.After)
	}
	if q.Folder != "" {
		// in:inbox 这类按类型精确匹配；in:发票 这类按显示名/路径模糊匹配
		like := "%" + escapeLike(q.Folder) + "%"
		dbq = dbq.Where(
			"messages.folder_id IN (SELECT id FROM folders WHERE type = ? OR display_name LIKE ? ESCAPE '\\' OR path LIKE ? ESCAPE '\\')",
			strings.ToLower(q.Folder), like, like,
		)
	}
	if q.Account != "" {
		like := "%" + escapeLike(q.Account) + "%"
		dbq = dbq.Where(
			"messages.account_id IN (SELECT id FROM accounts WHERE email LIKE ? ESCAPE '\\' OR name LIKE ? ESCAPE '\\')",
			like, like,
		)
	}
	return dbq
}

// CountSearchMessages 返回搜索命中的总条数，与列表同口径去重 + 同筛选条件。
func (r *Repository) CountSearchMessages(q fts.Query, f Filter) (int64, error) {
	var n int64
	err := f.apply(dedupeSameMessage(r.searchScope(q))).Count(&n).Error
	return n, err
}

// SearchMessages 跨账户检索，按 (date, id) 降序 keyset 分页，与聚合一致。
// 语法解析在上层完成（fts.Parse）；空查询由调用方拦下，这里不重复判断。
func (r *Repository) SearchMessages(q fts.Query, beforeDate *time.Time, beforeID uint, limit int, f Filter) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	dbq := f.apply(r.searchScope(q)).Select("messages.*")
	if beforeDate != nil {
		dbq = dbq.Where("messages.date < ? OR (messages.date = ? AND messages.id < ?)", *beforeDate, *beforeDate, beforeID)
	}
	var list []Message
	err := dedupeSameMessage(dbq).
		Order("messages.date DESC").Order("messages.id DESC").Limit(limit).Find(&list).Error
	return list, err
}

// SearchContacts 从历史邮件的发件人中检索去重联系人，按往来频率(出现次数)降序。
// q 为空时返回最常往来的前 N 个；否则按 地址/姓名 LIKE 过滤。
func (r *Repository) SearchContacts(q string, limit int) ([]Contact, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	dbq := r.db.Model(&Message{}).
		Select("from_addr as email, MAX(from_name) as name").
		Where("from_addr <> ''")
	if q != "" {
		like := "%" + escapeLike(q) + "%"
		dbq = dbq.Where("from_addr LIKE ? ESCAPE '\\' OR from_name LIKE ? ESCAPE '\\'", like, like)
	}
	var list []Contact
	err := dbq.Group("from_addr").Order("COUNT(*) DESC").Limit(limit).Scan(&list).Error
	return list, err
}

// escapeLike 转义 LIKE 通配符，避免用户输入的 % _ \ 被当作模式。
func escapeLike(s string) string {
	r := make([]rune, 0, len(s))
	for _, c := range s {
		if c == '%' || c == '_' || c == '\\' {
			r = append(r, '\\')
		}
		r = append(r, c)
	}
	return string(r)
}

// folderScope 构造单文件夹查询的公共条件（文件夹 + 筛选），供列表与计数共用。
// 抽出来的理由同 searchScope：两处各写一遍，「共 N 封」迟早与实际能翻到的条目数对不上。
func (r *Repository) folderScope(folderID uint, f Filter) *gorm.DB {
	return f.apply(r.db.Model(&Message{}).Where("messages.folder_id = ?", folderID))
}

func (r *Repository) ListByFolder(folderID uint, beforeUID uint32, limit int, f Filter) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := r.folderScope(folderID, f)
	if beforeUID > 0 {
		q = q.Where("messages.uid < ?", beforeUID)
	}
	var list []Message
	err := q.Order("messages.uid DESC").Limit(limit).Find(&list).Error
	return list, err
}

// dedupeSameMessage 给聚合/搜索查询附加「同一封邮件只保留一份」的条件。
//
// Gmail 把标签映射成 IMAP 文件夹：同一封邮件会同时出现在 INBOX、[Gmail]/所有邮件
// 以及每个命中标签的文件夹里——本地是多行（folder_id/uid 各不相同）。跨文件夹的
// 聚合/搜索若不去重，一封邮件会显示成三封。
//
// ⚠ 副本判定不能只看 Message-ID：GitHub 等发信端会给同一会话的多封通知复用同一个
// Message-ID（好让客户端归拢成一个线程），实测一个 id 下挂着 3 封日期/内容都不同的邮件。
// 只按 Message-ID 合并会把不同邮件误吞掉。副本的真正特征是 message_id + date + size
// 三者全同——同一封邮件的各份标签副本字节完全一致，而同线程的不同邮件日期与大小必然不同。
//
// 代表行按「收件箱 > 自定义 > 其它」优先级取，同级取最小 id（最早入库那份），
// 这样点开的总是收件箱里的那份。message_id 为空的邮件（少数不合规发信端）
// 不参与去重，各自独立展示，避免被错误地合并成一封。
func dedupeSameMessage(q *gorm.DB) *gorm.DB {
	return q.Where(`messages.message_id = '' OR messages.id = (
		SELECT m2.id FROM messages m2 JOIN folders f2 ON f2.id = m2.folder_id
		WHERE m2.account_id = messages.account_id AND m2.message_id = messages.message_id
		  AND m2.date = messages.date AND m2.size = messages.size
		ORDER BY CASE f2.type WHEN 'inbox' THEN 0 WHEN 'custom' THEN 1 ELSE 2 END, m2.id
		LIMIT 1)`)
}

// aggregateScope 给聚合查询附加 JOIN folders + 对应过滤条件。
// view 取值：inbox（各账户收件箱）/ unread（全部未读）/ starred（星标，排除回收站）。
// unread 只统计收件箱+自定义文件夹（对齐主流客户端与侧栏账户级未读口径）：
// 排除 trash/junk（干扰项）、archive（Gmail「所有邮件」全库镜像会与收件箱重复计）、
// sent/drafts（草稿常带未读标记，无意义）。
// 跨账户聚合，单管理员假设下不按 account 过滤。
func aggregateScope(db *gorm.DB, view string) *gorm.DB {
	q := db.Joins("JOIN folders ON folders.id = messages.folder_id")
	switch view {
	case "inbox":
		q = q.Where("folders.type = ?", "inbox")
	case "unread":
		q = q.Where("messages.seen = ?", false).
			Where("folders.type IN ?", []string{"inbox", "custom"})
	case "starred":
		q = q.Where("messages.flagged = ?", true).
			Where("folders.type <> ?", "trash")
	}
	return q
}

// ListAggregate 跨文件夹/账户聚合邮件列表，按 (date, id) 降序 keyset 分页。
// beforeDate==nil 取首页；翻页时传入上一页最后一封的 date+id 作游标。
func (r *Repository) ListAggregate(view string, beforeDate *time.Time, beforeID uint, limit int, f Filter) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := f.apply(dedupeSameMessage(aggregateScope(r.db.Model(&Message{}), view))).Select("messages.*")
	if beforeDate != nil {
		q = q.Where("messages.date < ? OR (messages.date = ? AND messages.id < ?)", *beforeDate, *beforeDate, beforeID)
	}
	var list []Message
	err := q.Order("messages.date DESC").Order("messages.id DESC").Limit(limit).Find(&list).Error
	return list, err
}

// CountAggregate 返回聚合入口的徽标计数：
// inbox -> 各账户收件箱未读数；unread -> 全部未读数；starred -> 星标数。
func (r *Repository) CountAggregate(view string) (int64, error) {
	q := aggregateScope(r.db.Model(&Message{}), view)
	if view == "inbox" {
		// 收件箱聚合按 MailMaster 语义展示未读数。
		q = q.Where("messages.seen = ?", false)
	}
	var n int64
	// 与列表同口径去重，否则 Gmail 场景下角标数与实际条目数对不上。
	err := dedupeSameMessage(q).Count(&n).Error
	return n, err
}

// CountAggregateTotal 返回聚合视图的条目总数——不附加未读过滤。
//
// ⚠ 与 CountAggregate 的区别只在 inbox 视图：那个方法对 inbox 返回的是「未读数」
// （入口徽标的语义），而列表标题要的是「共几封」。两者混用会让标题显示成未读数。
func (r *Repository) CountAggregateTotal(view string, f Filter) (int64, error) {
	var n int64
	err := f.apply(dedupeSameMessage(aggregateScope(r.db.Model(&Message{}), view))).Count(&n).Error
	return n, err
}

// AccountUnreadCounts 返回各账户的未读数（account_id → 未读），供侧栏账户角标使用。
// 口径与 unread 聚合完全一致：只算收件箱 + 自定义文件夹，且同一封邮件跨文件夹只计一次——
// 否则 Gmail 的一封未读会被 INBOX 与各标签文件夹重复累加，账户角标与聚合入口互相矛盾。
func (r *Repository) AccountUnreadCounts() (map[uint]int64, error) {
	var rows []struct {
		AccountID uint
		N         int64
	}
	err := dedupeSameMessage(aggregateScope(r.db.Model(&Message{}), "unread")).
		Select("messages.account_id as account_id, COUNT(*) as n").
		Group("messages.account_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint]int64, len(rows))
	for _, row := range rows {
		out[row.AccountID] = row.N
	}
	return out, nil
}

// CountByFolder 返回文件夹内邮件总数；f 为零值时即全量计数。
func (r *Repository) CountByFolder(folderID uint, f Filter) (int64, error) {
	var n int64
	err := r.folderScope(folderID, f).Count(&n).Error
	return n, err
}

func (r *Repository) CountByAccount(accountID uint) (int64, error) {
	var n int64
	err := r.db.Model(&Message{}).Where("account_id = ?", accountID).Count(&n).Error
	return n, err
}

// MaxUID 返回文件夹内最大的 UID（无邮件返回 0）。用于服务商不报 UIDNEXT 时推导锚点。
func (r *Repository) MaxUID(folderID uint) (uint32, error) {
	var maxUID *uint32
	err := r.db.Model(&Message{}).Where("folder_id = ?", folderID).
		Select("MAX(uid)").Scan(&maxUID).Error
	if err != nil || maxUID == nil {
		return 0, err
	}
	return *maxUID, nil
}

// MaxIDByFolder 返回文件夹内最大的消息主键（无邮件返回 0）。
// 同步前记录，同步后用 UnseenAfterID 找出本轮新增的未读邮件。
func (r *Repository) MaxIDByFolder(folderID uint) (uint, error) {
	var maxID *uint
	err := r.db.Model(&Message{}).Where("folder_id = ?", folderID).
		Select("MAX(id)").Scan(&maxID).Error
	if err != nil || maxID == nil {
		return 0, err
	}
	return *maxID, nil
}

// UnseenAfterID 返回文件夹内主键大于 afterID 的未读邮件（升序，最多 limit 封）。
func (r *Repository) UnseenAfterID(folderID uint, afterID uint, limit int) ([]Message, error) {
	var rows []Message
	err := r.db.Where("folder_id = ? AND id > ? AND seen = ?", folderID, afterID, false).
		Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

// ListAfterID 返回文件夹内主键大于 afterID 的全部邮件（升序，最多 limit 封）——规则引擎的输入：
// UnseenAfterID 只给未读且截断到 3 封，PendingBodiesAfterID 只给缺正文的，都不能当规则输入。
func (r *Repository) ListAfterID(folderID uint, afterID uint, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 500
	}
	var rows []Message
	err := r.db.Where("folder_id = ? AND id > ?", folderID, afterID).
		Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

// ListRecentInbox 返回各账户（accountID = 0）或指定账户收件箱里最近的 limit 封，规则试运行用。
func (r *Repository) ListRecentInbox(accountID uint, limit int) ([]Message, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := r.db.Model(&Message{}).
		Joins("JOIN folders ON folders.id = messages.folder_id").
		Where("folders.type = ?", "inbox")
	if accountID > 0 {
		q = q.Where("messages.account_id = ?", accountID)
	}
	var rows []Message
	err := q.Select("messages.*").Order("messages.date DESC").Order("messages.id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

// CountUnseenAfterID 返回文件夹内主键大于 afterID 的未读邮件总数。
func (r *Repository) CountUnseenAfterID(folderID uint, afterID uint) (int64, error) {
	var n int64
	err := r.db.Model(&Message{}).
		Where("folder_id = ? AND id > ? AND seen = ?", folderID, afterID, false).Count(&n).Error
	return n, err
}

func (r *Repository) UnreadCountByFolder(folderID uint) (int64, error) {
	var n int64
	err := r.db.Model(&Message{}).Where("folder_id = ? AND seen = ?", folderID, false).Count(&n).Error
	return n, err
}

// UnreadIDsByFolder 返回该文件夹里全部未读邮件的主键，升序。
//
// 只取 id 而不是整行：调用方（文件夹级"全部标为已读"）只需要主键，
// 而这个集合可能有几万条——整行捞出来就是几十 MB 的正文片段与头部。
//
// 也**只查未读**，不是查全部再过滤：已读的那些既不需要改本地、也不需要回写，
// 带上它们只会让 IMAP STORE 的 UID 集合白白膨胀几倍。
func (r *Repository) UnreadIDsByFolder(folderID uint) ([]uint, error) {
	var ids []uint
	err := r.db.Model(&Message{}).
		Where("folder_id = ? AND seen = ?", folderID, false).
		Order("id ASC").
		Pluck("id", &ids).Error
	return ids, err
}

// GetByIDs 按主键批量取行（分块防绑定变量上限），不存在的 id 直接缺席；结果按 id 升序。
func (r *Repository) GetByIDs(ids []uint) ([]Message, error) {
	var out []Message
	for start := 0; start < len(ids); start += uidChunk {
		end := start + uidChunk
		if end > len(ids) {
			end = len(ids)
		}
		var rows []Message
		if err := r.db.Where("id IN ?", ids[start:end]).Order("id ASC").Find(&rows).Error; err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

func (r *Repository) GetByID(id uint) (*Message, error) {
	var m Message
	err := r.db.First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repository) SetSeen(id uint, seen bool) error {
	return r.db.Model(&Message{}).Where("id = ?", id).Update("seen", seen).Error
}

func (r *Repository) SetFlagged(id uint, flagged bool) error {
	return r.db.Model(&Message{}).Where("id = ?", id).Update("flagged", flagged).Error
}

// bodyPrefetchTypes 是正文预取覆盖的文件夹类型：收件箱 + 自定义（标签）。
// 与未读口径一致——archive 是 Gmail「所有邮件」全库镜像，正文与收件箱那份重复，
// junk/trash/sent/drafts 也不值得占磁盘。
var bodyPrefetchTypes = []string{"inbox", "custom"}

// PendingBodies 返回该账户里缺正文、且落在预取范围内的邮件，按日期新→旧。
// sinceDays > 0 时只取该天数窗口内的邮件（recent 模式）；<= 0 表示不限日期（all 模式）。
func (r *Repository) PendingBodies(accountID uint, sinceDays, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 100
	}
	q := r.db.Model(&Message{}).
		Joins("JOIN folders ON folders.id = messages.folder_id").
		Where("messages.account_id = ?", accountID).
		Where("messages.body_synced = ?", false).
		Where("folders.type IN ?", bodyPrefetchTypes)
	if sinceDays > 0 {
		q = q.Where("messages.date >= ?", time.Now().AddDate(0, 0, -sinceDays))
	}
	var list []Message
	err := q.Select("messages.*").
		Order("messages.date DESC").Order("messages.id DESC").
		Limit(limit).Find(&list).Error
	return list, err
}

// PendingBodiesAfterID 返回某文件夹内主键大于 afterID、且缺正文的邮件（升序）。
// new 模式用：afterID 取本轮同步前该文件夹的最大主键，圈出的就是这一轮新收到的邮件。
func (r *Repository) PendingBodiesAfterID(folderID uint, afterID uint, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 100
	}
	var list []Message
	err := r.db.Where("folder_id = ? AND id > ? AND body_synced = ?", folderID, afterID, false).
		Order("id ASC").Limit(limit).Find(&list).Error
	return list, err
}

// CountPendingBodies 返回该账户预取范围内还缺多少封正文（进度展示/诊断用）。
func (r *Repository) CountPendingBodies(accountID uint, sinceDays int) (int64, error) {
	q := r.db.Model(&Message{}).
		Joins("JOIN folders ON folders.id = messages.folder_id").
		Where("messages.account_id = ?", accountID).
		Where("messages.body_synced = ?", false).
		Where("folders.type IN ?", bodyPrefetchTypes)
	if sinceDays > 0 {
		q = q.Where("messages.date >= ?", time.Now().AddDate(0, 0, -sinceDays))
	}
	var n int64
	err := q.Count(&n).Error
	return n, err
}

// MarkBodySynced 置 body_synced=true 并回填 snippet/has_attachment。
func (r *Repository) MarkBodySynced(id uint, snippet string, hasAttachment bool) error {
	return r.db.Model(&Message{}).Where("id = ?", id).Updates(map[string]any{
		"body_synced":    true,
		"snippet":        snippet,
		"has_attachment": hasAttachment,
	}).Error
}
