package setting

import "time"

// Setting 以键值对存储系统配置。
type Setting struct {
	Key       string    `gorm:"primaryKey" json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"-"`
}

func (Setting) TableName() string { return "settings" }

// 已知设置键 + 默认值。
const (
	KeySyncDepth     = "sync_depth"
	DefaultSyncDepth = "1000"
)
