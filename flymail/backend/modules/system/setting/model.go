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

	// KeySyncPollInterval 后台轮询间隔（秒）。
	KeySyncPollInterval = "sync_poll_interval"

	// KeySyncMaxConcurrent 全局同时执行全量同步的账户数上限。
	KeySyncMaxConcurrent     = "sync_max_concurrent"
	DefaultSyncMaxConcurrent = "8"

	// KeySyncMaxIdleConns 常驻 IDLE 连接数上限（超额账户降为轮询模式）。
	KeySyncMaxIdleConns     = "sync_max_idle_conns"
	DefaultSyncMaxIdleConns = "100"
)
