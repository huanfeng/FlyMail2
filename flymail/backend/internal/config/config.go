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

type Config struct {
	DataDir string       `mapstructure:"-"`
	Server  ServerConfig `mapstructure:"server"`
	Auth    AuthConfig   `mapstructure:"auth"`
}

func (c *Config) DBPath() string         { return filepath.Join(c.DataDir, "flymail.db") }
func (c *Config) AttachmentsDir() string { return filepath.Join(c.DataDir, "attachments") }

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
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("auth.access_token_ttl", 15)
	v.SetDefault("auth.refresh_token_ttl", 168)

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
