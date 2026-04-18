package config

import (
	"fmt"
	"os"
	"path/filepath"

	coreconfig "flymail-core/config"
)

type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Logger   LoggerConfig   `mapstructure:"logger"`
	DataDir  string         `mapstructure:"data_dir"`
	CORS     CORSConfig     `mapstructure:"cors"`
	Worker   WorkerConfig   `mapstructure:"worker"`
}

type AppConfig struct {
	Name    string `mapstructure:"name"`
	Version string `mapstructure:"version"`
	Env     string `mapstructure:"env"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

type DatabaseConfig struct {
	Path    string `mapstructure:"path"`
	LogPath string `mapstructure:"log_path"`
}

type AuthConfig struct {
	JWTSecret                 string `mapstructure:"jwt_secret"`
	JWTExpiration             int    `mapstructure:"jwt_expiration"`
	JWTRefreshExpirationHours int    `mapstructure:"jwt_refresh_expiration_hours"`
	AdminDefaultPassword      string `mapstructure:"admin_default_password"`
}

type LoggerConfig struct {
	Level       string          `mapstructure:"level"`        // debug, info, warn, error
	Development bool            `mapstructure:"development"`  // development mode
	OutputPaths []string        `mapstructure:"output_paths"` // output destinations
	Rotation    *RotationConfig `mapstructure:"rotation"`     // log rotation config
}

type RotationConfig struct {
	MaxSize    int  `mapstructure:"max_size"`    // megabytes
	MaxBackups int  `mapstructure:"max_backups"` // number of backups
	MaxAge     int  `mapstructure:"max_age"`     // days
	Compress   bool `mapstructure:"compress"`    // compress rotated files
}

// 新增 CORS 配置结构
type CORSConfig struct {
	AllowOrigins     []string `mapstructure:"allow_origins"`
	AllowMethods     []string `mapstructure:"allow_methods"`
	AllowHeaders     []string `mapstructure:"allow_headers"`
	ExposeHeaders    []string `mapstructure:"expose_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
	MaxAge           int      `mapstructure:"max_age"`
}

// WorkerConfig 定义工作线程配置
type WorkerConfig struct {
	NotifyWorkers int `mapstructure:"notify_workers"` // 通知系统工作线程数
	TaskWorkers   int `mapstructure:"task_workers"`   // 任务系统工作线程数
}

func Load() (*Config, error) {
	// Determine config file path from env or default
	configPath := os.Getenv("FLYMAIL_CONFIG")
	if configPath == "" {
		configPath = "./data/config.yaml"
	}

	var cfg Config
	if err := coreconfig.LoadConfig(coreconfig.LoadOptions{
		EnvPrefix:  "FLYMAIL",
		ConfigPath: configPath,
		Defaults:   defaultValues(),
	}, &cfg); err != nil {
		return nil, err
	}

	// Validate JWT secret
	if cfg.Auth.JWTSecret == "" {
		return nil, fmt.Errorf("JWT secret is empty. Please set a secure JWT secret in the config file or via FLYMAIL_AUTH_JWT_SECRET environment variable")
	}
	if len(cfg.Auth.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT secret is too short (minimum 32 characters). Please set a secure JWT secret in the config file or via FLYMAIL_AUTH_JWT_SECRET environment variable")
	}

	// Ensure data directory exists
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Set absolute database path
	if !filepath.IsAbs(cfg.Database.Path) {
		cfg.Database.Path = filepath.Join(dataDir, cfg.Database.Path)
	}

	return &cfg, nil
}

var cfg *Config

// GetConfig returns the global config instance
func GetConfig() *Config {
	if cfg == nil {
		var err error
		cfg, err = Load()
		if err != nil {
			panic(fmt.Sprintf("Failed to load config: %v", err))
		}
	}
	return cfg
}

func defaultValues() map[string]any {
	return map[string]any{
		"app.name":                         "FlyMail",
		"app.version":                      "1.0.0",
		"app.env":                          "production",
		"server.port":                      8080,
		"server.host":                      "127.0.0.1",
		"database.path":                    "flymail.db",
		"database.log_path":                "flymail_log.db",
		"auth.jwt_secret":                  coreconfig.GenerateRandomSecret(),
		"auth.jwt_expiration":              3600,
		"auth.jwt_refresh_expiration_hours": 7 * 24,
		"auth.admin_default_password":      "admin123",
		"data_dir":                         "./data",
		"logger.level":                     "info",
		"logger.development":               false,
		"logger.output_paths":              []string{"stdout"},
		"cors.allow_origins":               []string{},
		"cors.allow_methods":               []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		"cors.allow_headers":               []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		"cors.expose_headers":              []string{},
		"cors.allow_credentials":           true,
		"cors.max_age":                     3600,
		"worker.notify_workers":            5,
		"worker.task_workers":              5,
	}
}
