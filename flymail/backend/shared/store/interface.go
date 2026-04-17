package store

import (
	"context"
	"flymail/shared/store/model"
	"time"
)

// UserRepository defines the interface for user data access
type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id uint) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	Update(ctx context.Context, user *model.User) error
	UpdatePassword(ctx context.Context, id uint, hashedPassword string) error
	Delete(ctx context.Context, id uint) error
}

// AccountRepository defines the interface for email account data access
type AccountRepository interface {
	Create(ctx context.Context, account *model.EmailAccount) error
	GetByID(ctx context.Context, id uint, userID uint) (*model.EmailAccount, error)
	GetByUserID(ctx context.Context, userID uint) ([]*model.EmailAccount, error)
	GetActiveAccounts(ctx context.Context) ([]*model.EmailAccount, error)
	Update(ctx context.Context, account *model.EmailAccount) error
	UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error
	Delete(ctx context.Context, id uint) error
	GetAll(ctx context.Context) ([]*model.EmailAccount, error)
	Count(ctx context.Context) (int64, error)
}

// EmailRepository defines the interface for email data access
type EmailRepository interface {
	Create(ctx context.Context, email *model.Email) error
	GetByID(ctx context.Context, id uint) (*model.Email, error)
	GetByAccountID(ctx context.Context, accountID uint, limit, offset int) ([]*model.Email, error)
	GetByMessageID(ctx context.Context, messageID string, accountID uint) (*model.Email, error)
	GetByUID(ctx context.Context, accountID uint, folderName string, uid uint32) (*model.Email, error)
	Update(ctx context.Context, email *model.Email) error
	UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error
	Delete(ctx context.Context, id uint) error
	BulkCreate(ctx context.Context, emails []*model.Email) error
	GetLatestByAccountID(ctx context.Context, accountID uint) (*model.Email, error)
	CountByAccount(ctx context.Context, accountID uint) (int64, error)
	CountUnreadByAccount(ctx context.Context, accountID uint) (int64, error)
	GetTotalSizeByAccount(ctx context.Context, accountID uint) (int64, error)
	List(ctx context.Context, userID uint, filter interface{}) ([]model.Email, int64, error)
}

// FolderRepository defines the interface for folder data access
type FolderRepository interface {
	Create(ctx context.Context, folder *model.Folder) error
	GetByID(ctx context.Context, id uint) (*model.Folder, error)
	GetByAccountID(ctx context.Context, accountID uint) ([]*model.Folder, error)
	GetByName(ctx context.Context, name string, accountID uint) (*model.Folder, error)
	GetByAccountAndRawName(ctx context.Context, accountID uint, rawName string) (*model.Folder, error)
	Update(ctx context.Context, folder *model.Folder) error
	UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error
	Delete(ctx context.Context, id uint) error
	SyncFolders(ctx context.Context, accountID uint, folders []*model.Folder) error
	CountEmailsByFolder(ctx context.Context, accountID uint, folderName string) (int64, error)
	CountUnreadEmailsByFolder(ctx context.Context, accountID uint, folderName string) (int64, error)
}

// SettingRepository defines the interface for setting data access
type SettingRepository interface {
	Get(ctx context.Context, key string) (*model.Setting, error)
	Set(ctx context.Context, key, value string) error
	GetAll(ctx context.Context) ([]*model.Setting, error)
	GetMultiple(ctx context.Context, keys []string) ([]*model.Setting, error)
	SetMultiple(ctx context.Context, settings map[string]string) error
	Delete(ctx context.Context, key string) error
}

// NotifyRepository defines the interface for notification data operations
type NotifyRepository interface {
	// Channel operations
	CreateChannel(ctx context.Context, channel *model.NotifyChannel) error
	GetChannelByID(ctx context.Context, id string) (*model.NotifyChannel, error)
	GetChannels(ctx context.Context, enabled *bool) ([]*model.NotifyChannel, error)
	UpdateChannel(ctx context.Context, channel *model.NotifyChannel) error
	DeleteChannel(ctx context.Context, id string) error

	// Log operations
	CreateLog(ctx context.Context, log *model.NotifyLog) error
	GetLogByID(ctx context.Context, id uint) (*model.NotifyLog, error)
	GetLogs(ctx context.Context, filter map[string]interface{}, offset, limit int) ([]*model.NotifyLog, int64, error)
	UpdateLog(ctx context.Context, log *model.NotifyLog) error
	GetPendingLogs(ctx context.Context, maxRetries int) ([]*model.NotifyLog, error)
	CleanOldLogs(ctx context.Context, before time.Time) error
}
