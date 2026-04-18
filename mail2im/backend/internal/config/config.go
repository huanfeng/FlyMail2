package config

import (
	"os"

	coreconfig "flymail-core/config"
)

// Config 应用配置
type Config struct {
	Port        string     `mapstructure:"port"`
	DataRoot    string     `mapstructure:"data_root"`
	Env         string     `mapstructure:"env"`
	JWTSecret   string     `mapstructure:"jwt_secret"`
	AppSecret   string     `mapstructure:"app_secret"`
	CORSOrigins string     `mapstructure:"cors_origins"`
	CORS        CORSConfig `mapstructure:"cors"`
}

// CORSConfig CORS 配置
type CORSConfig struct {
	AllowOrigins     []string `mapstructure:"allow_origins"`
	AllowMethods     []string `mapstructure:"allow_methods"`
	AllowHeaders     []string `mapstructure:"allow_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
}

// AppConfig 全局配置实例
var AppConfig *Config

// Load 加载应用配置
func Load() (*Config, error) {
	configPath := os.Getenv("MAIL2IM_CONFIG")
	if configPath == "" {
		configPath = "./mail2im_data/config.yaml"
	}

	var cfg Config
	if err := coreconfig.LoadConfig(coreconfig.LoadOptions{
		EnvPrefix:  "MAIL2IM",
		ConfigPath: configPath,
		Defaults:   defaultValues(),
	}, &cfg); err != nil {
		return nil, err
	}

	// 兼容旧的环境变量名
	if cfg.JWTSecret == "change-me-in-prod" {
		if secret := os.Getenv("JWT_SECRET"); secret != "" {
			cfg.JWTSecret = secret
		}
	}
	if cfg.DataRoot == "" {
		if dr := os.Getenv("DATA_ROOT"); dr != "" {
			cfg.DataRoot = dr
		}
	}

	AppConfig = &cfg
	return &cfg, nil
}

func defaultValues() map[string]any {
	return map[string]any{
		"port":               "8080",
		"data_root":          "./mail2im_data",
		"env":                "development",
		"jwt_secret":         "change-me-in-prod",
		"app_secret":         "mail2im-default-secret-key-32bytes",
		"cors_origins":       "",
		"cors.allow_origins": []string{},
		"cors.allow_methods": []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		"cors.allow_headers": []string{"Origin", "Content-Type", "Authorization"},
		"cors.allow_credentials": true,
	}
}
