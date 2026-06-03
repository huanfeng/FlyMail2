package folder

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrFolderNotFound = errors.New("folder not found")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) UpsertByPath(f *Folder) error {
	var existing Folder
	err := r.db.Where("account_id = ? AND path = ?", f.AccountID, f.Path).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Select("*").Create(f).Error
	}
	if err != nil {
		return err
	}
	f.ID = existing.ID
	f.CreatedAt = existing.CreatedAt
	if f.UIDValidity == 0 {
		f.UIDValidity = existing.UIDValidity
	}
	if f.UIDNext == 0 {
		f.UIDNext = existing.UIDNext
	}
	if f.LastSyncedUID == 0 {
		f.LastSyncedUID = existing.LastSyncedUID
	}
	if f.LastSyncedAt == nil {
		f.LastSyncedAt = existing.LastSyncedAt
	}
	if f.TotalCount == 0 {
		f.TotalCount = existing.TotalCount
	}
	if f.UnreadCount == 0 {
		f.UnreadCount = existing.UnreadCount
	}
	return r.db.Save(f).Error
}

func (r *Repository) ListByAccount(accountID uint) ([]Folder, error) {
	var list []Folder
	err := r.db.Where("account_id = ?", accountID).
		Order("sort_order, display_name").Find(&list).Error
	return list, err
}

func (r *Repository) CountByAccount(accountID uint) (int64, error) {
	var n int64
	err := r.db.Model(&Folder{}).Where("account_id = ?", accountID).Count(&n).Error
	return n, err
}

func (r *Repository) GetByID(id uint) (*Folder, error) {
	var f Folder
	err := r.db.First(&f, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrFolderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *Repository) GetByPath(accountID uint, path string) (*Folder, error) {
	var f Folder
	err := r.db.Where("account_id = ? AND path = ?", accountID, path).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrFolderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *Repository) FindByType(accountID uint, folderType string) (*Folder, error) {
	var f Folder
	err := r.db.Where("account_id = ? AND type = ?", accountID, folderType).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *Repository) FindInbox(accountID uint) (*Folder, error) {
	return r.FindByType(accountID, "inbox")
}

// UpdateUnreadCount 只更新文件夹未读数（标记已读/未读后即时刷新角标用）。
func (r *Repository) UpdateUnreadCount(id uint, unread int) error {
	return r.db.Model(&Folder{}).Where("id = ?", id).Update("unread_count", unread).Error
}

// UpdateCounts 同时更新文件夹总数与未读数（删除/移动邮件后即时刷新角标用）。
func (r *Repository) UpdateCounts(id uint, total, unread int) error {
	return r.db.Model(&Folder{}).Where("id = ?", id).Updates(map[string]any{
		"total_count":  total,
		"unread_count": unread,
	}).Error
}

func (r *Repository) UpdateSyncState(id uint, uidValidity, uidNext uint32, total, unread int, syncedAt time.Time) error {
	return r.db.Model(&Folder{}).Where("id = ?", id).Updates(map[string]any{
		"uid_validity":   uidValidity,
		"uid_next":       uidNext,
		"total_count":    total,
		"unread_count":   unread,
		"last_synced_at": syncedAt,
	}).Error
}
