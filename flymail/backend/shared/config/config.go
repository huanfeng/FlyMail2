package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
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
	// Set default values
	setDefaults()

	// Read environment variables first
	viper.SetEnvPrefix("FLYMAIL")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Set config file path
	var configPath string
	if configFile := viper.GetString("config"); configFile != "" {
		configPath = configFile
		viper.SetConfigFile(configFile)
	} else {
		configPath = "./data/config.yaml"
		viper.SetConfigFile(configPath)
	}

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		var pathError *fs.PathError
		if errors.As(err, &pathError) && errors.Is(pathError.Err, fs.ErrNotExist) {
			if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
				return nil, fmt.Errorf("failed to create data directory: %w", err)
			}

			if err := viper.SafeWriteConfigAs(configPath); err != nil {
				return nil, fmt.Errorf("failed to create default config: %w", err)
			}

			fmt.Printf("Created default config file at %s\n", configPath)
			fmt.Println("WARNING: Please change the default admin password and review the JWT secret for security!")
		} else {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
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

func setDefaults() {
	viper.SetDefault("app.name", "FlyMail")
	viper.SetDefault("app.version", "1.0.0")
	viper.SetDefault("app.env", "production")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.host", "127.0.0.1")
	viper.SetDefault("database.path", "flymail.db")
	viper.SetDefault("database.log_path", "flymail_log.db")
	viper.SetDefault("auth.jwt_secret", generateRandomSecret())
	viper.SetDefault("auth.jwt_expiration", 3600)
	viper.SetDefault("auth.jwt_refresh_expiration_hours", 7*24)
	viper.SetDefault("auth.admin_default_password", "admin123")
	viper.SetDefault("data_dir", "./data")
	viper.SetDefault("logger.level", "info")
	viper.SetDefault("logger.development", false)
	viper.SetDefault("logger.output_paths", []string{"stdout"})
	// CROS 默认设置
	viper.SetDefault("cors.allow_origins", []string{})
	viper.SetDefault("cors.allow_methods", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	viper.SetDefault("cors.allow_headers", []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"})
	viper.SetDefault("cors.expose_headers", []string{})
	viper.SetDefault("cors.allow_credentials", true)
	viper.SetDefault("cors.max_age", 3600)
	// Worker 默认设置
	viper.SetDefault("worker.notify_workers", 5)
	viper.SetDefault("worker.task_workers", 5)
}

// generateRandomSecret generates a random 32-character hex string for JWT secret
func generateRandomSecret() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a default if random generation fails
		return "default-insecure-jwt-secret-replace-me"
	}
	return hex.EncodeToString(bytes)
}
