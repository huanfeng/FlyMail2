package message

import (
	"errors"
	"time"

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

func (r *Repository) DeleteByFolder(folderID uint) error {
	return r.db.Where("folder_id = ?", folderID).Delete(&Message{}).Error
}

// DeleteByID 删除单封邮件的本地元数据行（移动/删除成功后清理本地缓存）。
// 正文/附件随 message_id 外键留存，下次该 message 不再出现即可，可由后续清理流程回收。
func (r *Repository) DeleteByID(id uint) error {
	return r.db.Where("id = ?", id).Delete(&Message{}).Error
}

// SearchMessages 跨账户全文检索：在 主题/发件人名/发件人地址/摘要/正文 上做 LIKE。
// 按 (date, id) 降序 keyset 分页，与聚合一致。q 已由调用方做转义。
func (r *Repository) SearchMessages(q string, beforeDate *time.Time, beforeID uint, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	like := "%" + escapeLike(q) + "%"
	dbq := r.db.Model(&Message{}).
		Joins("LEFT JOIN message_bodies ON message_bodies.message_id = messages.id").
		Where(
			"messages.subject LIKE ? ESCAPE '\\' OR messages.from_name LIKE ? ESCAPE '\\' OR "+
				"messages.from_addr LIKE ? ESCAPE '\\' OR messages.snippet LIKE ? ESCAPE '\\' OR "+
				"message_bodies.text_body LIKE ? ESCAPE '\\'",
			like, like, like, like, like,
		).
		Select("messages.*")
	if beforeDate != nil {
		dbq = dbq.Where("messages.date < ? OR (messages.date = ? AND messages.id < ?)", *beforeDate, *beforeDate, beforeID)
	}
	var list []Message
	err := dbq.Order("messages.date DESC").Order("messages.id DESC").Limit(limit).Find(&list).Error
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

func (r *Repository) ListByFolder(folderID uint, beforeUID uint32, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := r.db.Where("folder_id = ?", folderID)
	if beforeUID > 0 {
		q = q.Where("uid < ?", beforeUID)
	}
	var list []Message
	err := q.Order("uid DESC").Limit(limit).Find(&list).Error
	return list, err
}

// aggregateScope 给聚合查询附加 JOIN folders + 对应过滤条件。
// view 取值：inbox（各账户收件箱）/ unread（全部未读，排除回收站/垃圾箱）/ starred（星标，排除回收站）。
// 跨账户聚合，单管理员假设下不按 account 过滤。
func aggregateScope(db *gorm.DB, view string) *gorm.DB {
	q := db.Joins("JOIN folders ON folders.id = messages.folder_id")
	switch view {
	case "inbox":
		q = q.Where("folders.type = ?", "inbox")
	case "unread":
		q = q.Where("messages.seen = ?", false).
			Where("folders.type NOT IN ?", []string{"trash", "junk"})
	case "starred":
		q = q.Where("messages.flagged = ?", true).
			Where("folders.type <> ?", "trash")
	}
	return q
}

// ListAggregate 跨文件夹/账户聚合邮件列表，按 (date, id) 降序 keyset 分页。
// beforeDate==nil 取首页；翻页时传入上一页最后一封的 date+id 作游标。
func (r *Repository) ListAggregate(view string, beforeDate *time.Time, beforeID uint, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := aggregateScope(r.db.Model(&Message{}), view).Select("messages.*")
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
	err := q.Count(&n).Error
	return n, err
}

func (r *Repository) CountByFolder(folderID uint) (int64, error) {
	var n int64
	err := r.db.Model(&Message{}).Where("folder_id = ?", folderID).Count(&n).Error
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

func (r *Repository) UnreadCountByFolder(folderID uint) (int64, error) {
	var n int64
	err := r.db.Model(&Message{}).Where("folder_id = ? AND seen = ?", folderID, false).Count(&n).Error
	return n, err
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

// MarkBodySynced 置 body_synced=true 并回填 snippet/has_attachment。
func (r *Repository) MarkBodySynced(id uint, snippet string, hasAttachment bool) error {
	return r.db.Model(&Message{}).Where("id = ?", id).Updates(map[string]any{
		"body_synced":    true,
		"snippet":        snippet,
		"has_attachment": hasAttachment,
	}).Error
}
