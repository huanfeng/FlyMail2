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

type Config struct {
	DataDir string       `mapstructure:"-"`
	Server  ServerConfig `mapstructure:"server"`
	Auth    AuthConfig   `mapstructure:"auth"`
	Crypto  CryptoConfig `mapstructure:"crypto"`
	Log     LogConfig    `mapstructure:"log"`
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
