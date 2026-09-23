package translate

import (
	"errors"

	"gorm.io/gorm"
)

// Repository 存取译文缓存。
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Get 取一封邮件在某目标语言下的译文；没有时返回 (nil, nil)。
func (r *Repository) Get(messageID uint, target string) (*Translation, error) {
	var t Translation
	err := r.db.Where("message_id = ? AND target_lang = ?", messageID, target).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Save 落库；同一封信同一目标语言已有记录时整行覆盖。
//
// 覆盖而不是插新行：唯一索引已经保证了一份，重译时直接改那一份最省事，
// 也让"缓存里永远是最新一次翻译的结果"这件事不依赖调用方先删。
func (r *Repository) Save(t *Translation) error {
	existing, err := r.Get(t.MessageID, t.TargetLang)
	if err != nil {
		return err
	}
	if existing != nil {
		t.ID = existing.ID
		t.CreatedAt = existing.CreatedAt
	}
	return r.db.Save(t).Error
}

// DeleteByMessage 删掉一封邮件的全部译文（各目标语言）。
func (r *Repository) DeleteByMessage(messageID uint) error {
	return r.db.Where("message_id = ?", messageID).Delete(&Translation{}).Error
}
