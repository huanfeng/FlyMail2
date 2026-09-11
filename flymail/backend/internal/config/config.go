package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	// TrustedProxies 是允许改写客户端 IP 的反向代理地址（CIDR 或 IP）。默认空：不信任任何代理，
	// ClientIP 只取 TCP 对端——否则任何直连客户端发一个 X-Forwarded-For 就能伪造 IP 绕过登录限流。
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

type AuthConfig struct {
	JWTSecret       string `mapstructure:"jwt_secret"`
	AccessTokenTTL  int    `mapstructure:"access_token_ttl"`  // 分钟
	RefreshTokenTTL int    `mapstructure:"refresh_token_ttl"` // 小时
}

type CryptoConfig struct {
	EncryptionKey string `mapstructure:"encryption_key"` // 账户凭证 AES 加密密钥；生产环境务必覆盖并保持稳定
}

// LogConfig 日志输出与轮转策略。Dir 为空时默认 <dataDir>/logs。
type LogConfig struct {
	Dir        string `mapstructure:"dir"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`  // 单文件最大 MB
	MaxBackups int    `mapstructure:"max_backups"`  // 保留备份数
	MaxAgeDays int    `mapstructure:"max_age_days"` // 备份保留天数
	Compress   bool   `mapstructure:"compress"`     // 是否压缩旧备份
	Console    bool   `mapstructure:"console"`      // 是否同时输出到控制台
	Level      string `mapstructure:"level"`        // debug/info/warn/error，默认 info
	Format     string `mapstructure:"format"`       // json/console，默认 json
}

// OAuthProviderConfig 是单个 OAuth 提供方的客户端凭据。
//
// client_secret 允许留空：loopback + PKCE 属公共客户端，Microsoft 公共客户端不需要
// secret，Google「桌面应用」类型虽仍签发 secret 但规范上不视其为机密。
type OAuthProviderConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	// Tenant 仅 Microsoft 使用：common 同时接受个人与工作账户，organizations 仅工作账户，
	// 也可填具体租户 ID 把登录限制在单一组织内。留空按 common 处理。
	Tenant string `mapstructure:"tenant"`
}

// OAuthConfig 汇总各提供方的 OAuth 客户端凭据。留空即不启用该提供方的入口。
type OAuthConfig struct {
	Google    OAuthProviderConfig `mapstructure:"google"`
	Microsoft OAuthProviderConfig `mapstructure:"microsoft"`
	// RedirectBaseURL 是 FlyMail 对外可访问的根地址（如 https://mail.example.com）。
	//
	// 留空时授权回调走 loopback（127.0.0.1 的临时端口），要求后端与浏览器同机——
	// 桌面端和本机自用的默认路径，无需在服务商后台登记任何地址。
	// 远程部署（Docker、独立服务器）下浏览器打不到服务端的回环地址，必须在此填入公开地址，
	// 并把 <该地址>/api/v1/accounts/oauth/callback 登记为服务商的重定向 URI。
	RedirectBaseURL string `mapstructure:"redirect_base_url"`
}

type Config struct {
	DataDir string       `mapstructure:"-"`
	Server  ServerConfig `mapstructure:"server"`
	Auth    AuthConfig   `mapstructure:"auth"`
	Crypto  CryptoConfig `mapstructure:"crypto"`
	Log     LogConfig    `mapstructure:"log"`
	OAuth   OAuthConfig  `mapstructure:"oauth"`
}

func (c *Config) DBPath() string         { return filepath.Join(c.DataDir, "flymail.db") }
func (c *Config) AttachmentsDir() string { return filepath.Join(c.DataDir, "attachments") }

// LogDir 返回日志目录：配置为空时默认 <dataDir>/logs。
func (c *Config) LogDir() string {
	if c.Log.Dir != "" {
		return c.Log.Dir
	}
	return filepath.Join(c.DataDir, "logs")
}

type LoadOptions struct {
	DataDir    string
	ConfigFile string
}

func Load(opts LoadOptions) (*Config, error) {
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = ResolveDataDir()
	}

	v := viper.New()
	// 默认只监听本机回环，避免每次启动弹防火墙；对外暴露请显式设
	// server.host=0.0.0.0（或环境变量 FLYMAIL_SERVER_HOST=0.0.0.0，如 Docker 部署）。
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 8080)
	// trusted_proxies 同样必须注册：不注册则 FLYMAIL_SERVER_TRUSTED_PROXIES 读不到，
	// 反向代理部署下所有请求的客户端 IP 都是代理自身——登录限流会按代理 IP 计数，
	// 一个人触发限流就把整站锁住，日志里的来源 IP 也全是同一个。
	// 环境变量按逗号分隔（viper 的默认 decode hook 含 StringToSliceHookFunc(",")）。
	v.SetDefault("server.trusted_proxies", []string{})
	// jwt_secret 注册空串默认值：与 log.dir 同理，viper 的 AutomaticEnv 仅对「已知的 key」
	// 在 Unmarshal 时生效，不注册则 FLYMAIL_AUTH_JWT_SECRET 不会被读取（Docker 部署必需）。
	v.SetDefault("auth.jwt_secret", "")
	v.SetDefault("auth.access_token_ttl", 15)
	v.SetDefault("auth.refresh_token_ttl", 168)
	v.SetDefault("crypto.encryption_key", "flymail-default-insecure-key-change-me")
	// log.dir 注册默认空串：viper 的 AutomaticEnv 仅对「已知的 key」在 Unmarshal 时生效，
	// 否则环境变量 FLYMAIL_LOG_DIR 不会被读取（dir 为空时 LogDir() 回退到 <dataDir>/logs）。
	v.SetDefault("log.dir", "")
	v.SetDefault("log.max_size_mb", 10)
	v.SetDefault("log.max_backups", 5)
	v.SetDefault("log.max_age_days", 30)
	v.SetDefault("log.compress", false)
	v.SetDefault("log.console", true)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	// OAuth 客户端凭据全部注册空串默认值：viper 的 AutomaticEnv 只对「已知的 key」
	// 在 Unmarshal 时生效，不注册则 FLYMAIL_OAUTH_GOOGLE_CLIENT_ID 这类环境变量
	// 不会被读取（Docker 部署下凭据只能走环境变量）。
	v.SetDefault("oauth.google.client_id", "")
	v.SetDefault("oauth.google.client_secret", "")
	v.SetDefault("oauth.microsoft.client_id", "")
	v.SetDefault("oauth.microsoft.client_secret", "")
	v.SetDefault("oauth.microsoft.tenant", "common")
	v.SetDefault("oauth.redirect_base_url", "")

	v.SetEnvPrefix("FLYMAIL")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if opts.ConfigFile != "" {
		v.SetConfigFile(opts.ConfigFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(dataDir)
	}
	if err := v.ReadInConfig(); err != nil {
		_, isNotFound := err.(viper.ConfigFileNotFoundError)
		isExplicitMissing := opts.ConfigFile != "" && os.IsNotExist(err)
		if !isNotFound && !isExplicitMissing {
			return nil, err
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}
	cfg.DataDir = dataDir
	return cfg, nil
}
