package setting

import (
	"time"

	"flymail/internal/lang"
)

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

	// KeyOAuthGoogleClientID / KeyOAuthGoogleClientSecret 是 Google OAuth 应用的客户端凭据，
	// 由管理员在设置页填写（也可用 FLYMAIL_OAUTH_GOOGLE_* 环境变量，见 internal/config）。
	//
	// 放数据库而不是只认环境变量，是因为配 OAuth 应用要反复试：回调地址填错、
	// 测试用户没加、secret 复制漏一位，每试一次重启一次容器不可接受。
	// 两处都有值时以数据库为准——那是管理员在界面上刚做的事，应当压过部署时的默认。
	// KeyNotifyBodyRunes 外发通知里正文的字符上限；0 或留空用内置默认。
	//
	// 它只是「想放多少」的排版偏好，不是安全上限：无论配多大，最终都还要过一道
	// 按序列化字节数算的裁剪（notify.fitFeishuCard），否则配个 999999
	// 就会因为超出飞书 30KB 的请求体上限而整条发不出去。
	KeyNotifyBodyRunes     = "notify_body_runes"
	DefaultNotifyBodyRunes = "8000"

	// KeyAIBaseURL / KeyAIAPIKey / KeyAIModel 是 AI 翻译所用的 OpenAI 兼容接口配置。
	//
	// 只存这三样，是因为 OpenAI 兼容接口本来就只要这三样就能调通——地址决定
	// 连谁（云端服务、自建网关、本机 Ollama 都是同一种形状），模型决定用哪个，
	// 密钥是可选的（本地模型通常不要）。多存一个参数，就多一处"换个服务商
	// 就要重新试"的地方。
	//
	// 地址在 handler 里归一化后落库：用户填 https://api.openai.com 还是
	// .../v1 还是完整的 .../v1/chat/completions 都认（见 ai.Endpoint）。
	KeyAIBaseURL = "ai_base_url"
	// ⚠ 值是密文（见 SetEncryptor），永远不出网：GET /settings 只回报「配没配」。
	KeyAIAPIKey = "ai_api_key"
	KeyAIModel  = "ai_model"

	// KeyTranslateTargetLang 是翻译的默认目标语言，取值见 lang.Supported。
	//
	// 默认简体中文而不是"跟随界面语言"：界面语言是用户看得懂的语言之一，
	// 但未必是他想把邮件翻成的那门——把界面切成英文练听力的中文用户，
	// 并不想让账单邮件也翻成英文。
	KeyTranslateTargetLang     = "translate_target_lang"
	DefaultTranslateTargetLang = lang.DefaultTarget

	KeyOAuthGoogleClientID = "oauth_google_client_id"
	// ⚠ 值是密文（见 SetEncryptor），永远不出网：GET /settings 只回报「配没配」。
	KeyOAuthGoogleClientSecret = "oauth_google_client_secret"
)

// secretKeys 是值以密文存储、且**任何情况下都不得回显**的设置键。
//
// 读取一侧（All）把它们整个摘掉，换成 <key>_set 的布尔标记；写入一侧（SetMany）
// 收到明文时先加密。名单集中在这里，是为了让「新增一个密文设置」只需要动一行——
// 而不是指望下一个人记得在读和写两处各补一段。
var secretKeys = []string{KeyOAuthGoogleClientSecret, KeyAIAPIKey}

// SecretSetSuffix 是密文设置在对外响应里的标记后缀：<key>_set = "true"/"false"。
const SecretSetSuffix = "_set"

// isSecretKey 报告某个键是否属于密文设置。
func isSecretKey(key string) bool {
	for _, k := range secretKeys {
		if k == key {
			return true
		}
	}
	return false
}

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
