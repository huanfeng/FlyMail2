package message

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BodyRepository struct{ db *gorm.DB }

func NewBodyRepository(db *gorm.DB) *BodyRepository { return &BodyRepository{db: db} }

// Upsert 按 message_id 唯一键插入或更新正文。
func (r *BodyRepository) Upsert(b *MessageBody) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"text_body", "html_body", "updated_at"}),
	}).Create(b).Error
}

// GetByMessageID 取正文，未找到返回 (nil, nil)。
func (r *BodyRepository) GetByMessageID(messageID uint) (*MessageBody, error) {
	var b MessageBody
	err := r.db.Where("message_id = ?", messageID).First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ReplaceAttachments 用新列表替换该邮件的全部附件元数据（事务内先删后插）。
func (r *BodyRepository) ReplaceAttachments(messageID uint, atts []Attachment) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("message_id = ?", messageID).Delete(&Attachment{}).Error; err != nil {
			return err
		}
		if len(atts) == 0 {
			return nil
		}
		return tx.Create(&atts).Error
	})
}

func (r *BodyRepository) ListAttachments(messageID uint) ([]Attachment, error) {
	var list []Attachment
	err := r.db.Where("message_id = ?", messageID).Find(&list).Error
	return list, err
}
