package message

import (
	"errors"

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

func (r *Repository) CountByFolder(folderID uint) (int64, error) {
	var n int64
	err := r.db.Model(&Message{}).Where("folder_id = ?", folderID).Count(&n).Error
	return n, err
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
