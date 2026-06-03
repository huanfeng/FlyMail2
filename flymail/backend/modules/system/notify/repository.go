package notify

import (
	"errors"

	"gorm.io/gorm"
)

var ErrChannelNotFound = errors.New("channel not found")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// ── 站内通知 ────────────────────────────────────────────────

func (r *Repository) InsertNotification(n *Notification) error {
	return r.db.Create(n).Error
}

// ListNotifications 按 id 降序分页（before_id=0 取首页）。
func (r *Repository) ListNotifications(beforeID uint, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := r.db.Model(&Notification{})
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var list []Notification
	err := q.Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}

func (r *Repository) CountUnread() (int64, error) {
	var n int64
	err := r.db.Model(&Notification{}).Where("read = ?", false).Count(&n).Error
	return n, err
}

func (r *Repository) MarkRead(id uint) error {
	return r.db.Model(&Notification{}).Where("id = ?", id).Update("read", true).Error
}

func (r *Repository) MarkAllRead() error {
	return r.db.Model(&Notification{}).Where("read = ?", false).Update("read", true).Error
}

func (r *Repository) DeleteAllNotifications() error {
	return r.db.Where("1 = 1").Delete(&Notification{}).Error
}

// ── 渠道 ────────────────────────────────────────────────────

func (r *Repository) CreateChannel(c *Channel) error { return r.db.Create(c).Error }

func (r *Repository) ListChannels() ([]Channel, error) {
	var list []Channel
	err := r.db.Order("id").Find(&list).Error
	return list, err
}

func (r *Repository) GetChannel(id uint) (*Channel, error) {
	var c Channel
	err := r.db.First(&c, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// EnabledChannelsFor 返回订阅了某事件且启用的渠道。
func (r *Repository) EnabledChannelsFor(t EventType) ([]Channel, error) {
	var list []Channel
	if err := r.db.Where("enabled = ?", true).Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]Channel, 0, len(list))
	for i := range list {
		if list[i].subscribes(t) {
			out = append(out, list[i])
		}
	}
	return out, nil
}

func (r *Repository) UpdateChannel(c *Channel) error { return r.db.Save(c).Error }

func (r *Repository) DeleteChannel(id uint) error {
	return r.db.Delete(&Channel{}, id).Error
}

// ── 投递日志 ────────────────────────────────────────────────

func (r *Repository) InsertLog(l *Log) error { return r.db.Create(l).Error }

func (r *Repository) ListLogs(limit int) ([]Log, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var list []Log
	err := r.db.Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}
