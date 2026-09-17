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

	// KeyBodySyncMode 正文预取模式，决定同步时顺带把哪些邮件的正文也下载到本地：
	//   new    仅新邮件——只预取本轮增量新收到的（默认，不回补历史）
	//   recent 最近 N 天内的历史邮件也补齐
	//   all    全部历史邮件都补齐
	// 预取范围限收件箱 + 自定义文件夹（与未读口径一致），避免 Gmail「所有邮件」
	// 这类全库镜像把整个邮箱的正文重复下一遍。
	KeyBodySyncMode     = "body_sync_mode"
	DefaultBodySyncMode = BodySyncNew

	// KeyAppBaseURL 是 FlyMail 对外可访问的根地址，例如 https://mail.example.com
	// 或 http://192.168.5.11:8086。
	//
	// ⚠ 服务端没有别的办法知道这个值：它看到的 Host 头可能是反向代理的内网名、
	// 容器名或 127.0.0.1，监听地址也可能是 0.0.0.0。所以必须由用户配置。
	//
	// 用途：通知里的「打开邮件」直达链接。留空则通知不带链接（其余功能不受影响）。
	KeyAppBaseURL = "app_base_url"

	// KeyBodySyncRecentDays 是 recent 模式的天数窗口。
	KeyBodySyncRecentDays     = "body_sync_recent_days"
	DefaultBodySyncRecentDays = "30"
)

// 正文预取模式取值。
const (
	BodySyncNew    = "new"
	BodySyncRecent = "recent"
	BodySyncAll    = "all"
)

// ValidBodySyncMode 报告字符串是否为合法的正文预取模式。
func ValidBodySyncMode(v string) bool {
	return v == BodySyncNew || v == BodySyncRecent || v == BodySyncAll
}
