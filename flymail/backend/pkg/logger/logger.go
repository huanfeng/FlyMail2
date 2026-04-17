package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	// Logger is the global logger instance
	Logger *zap.Logger
	// Sugar is the sugared logger for convenience
	Sugar *zap.SugaredLogger
)

// Config represents logger configuration
type Config struct {
	Level       string          `mapstructure:"level"`        // debug, info, warn, error
	Development bool            `mapstructure:"development"`  // development mode
	OutputPaths []string        `mapstructure:"output_paths"` // output destinations
	Rotation    *RotationConfig `mapstructure:"rotation"`     // log rotation config
}

// RotationConfig represents log rotation configuration
type RotationConfig struct {
	MaxSize    int  `mapstructure:"max_size"`    // megabytes, default 100
	MaxBackups int  `mapstructure:"max_backups"` // number of backups, default 3
	MaxAge     int  `mapstructure:"max_age"`     // days, default 15
	Compress   bool `mapstructure:"compress"`    // compress rotated files, default true
}

// DefaultConfig returns default logger configuration
func DefaultConfig() *Config {
	return &Config{
		Level:       "info",
		Development: false,
		OutputPaths: []string{"stdout"},
		Rotation: &RotationConfig{
			MaxSize:    100, // 100MB
			MaxBackups: 3,
			MaxAge:     15, // 15 days
			Compress:   true,
		},
	}
}

// Init initializes the global logger
func Init(cfg *Config) error {
	// Parse log level
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return err
	}

	// Create encoder config
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 创建不同级别的输出
	var allWriters []zapcore.WriteSyncer
	var warnErrorWriters []zapcore.WriteSyncer

	// 处理输出配置
	if len(cfg.OutputPaths) > 0 {
		for _, path := range cfg.OutputPaths {
			switch path {
			case "stdout":
				allWriters = append(allWriters, zapcore.AddSync(os.Stdout))
			case "stderr":
				allWriters = append(allWriters, zapcore.AddSync(os.Stderr))
			default:
				// 文件输出，使用 lumberjack 进行日志轮转
				if cfg.Rotation != nil {
					lj := &lumberjack.Logger{
						Filename:   path,
						MaxSize:    cfg.Rotation.MaxSize,
						MaxBackups: cfg.Rotation.MaxBackups,
						MaxAge:     cfg.Rotation.MaxAge,
						Compress:   cfg.Rotation.Compress,
					}
					allWriters = append(allWriters, zapcore.AddSync(lj))
				} else {
					// 无轮转配置时使用普通文件
					file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
					if err != nil {
						return err
					}
					allWriters = append(allWriters, zapcore.AddSync(file))
				}
			}
		}
	} else {
		allWriters = append(allWriters, zapcore.AddSync(os.Stdout))
	}

	// 创建warn和error级别的独立文件输出
	if !cfg.Development {
		// 确保logs目录存在
		if err := os.MkdirAll("./logs", 0755); err != nil {
			return err
		}

		// Warn和Error日志文件，使用日志轮转
		if cfg.Rotation != nil {
			lj := &lumberjack.Logger{
				Filename:   "./logs/warn_error.log",
				MaxSize:    cfg.Rotation.MaxSize,
				MaxBackups: cfg.Rotation.MaxBackups,
				MaxAge:     cfg.Rotation.MaxAge,
				Compress:   cfg.Rotation.Compress,
			}
			warnErrorWriters = append(warnErrorWriters, zapcore.AddSync(lj))
		} else {
			warnErrorFile, err := os.OpenFile("./logs/warn_error.log", os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
			if err != nil {
				return err
			}
			warnErrorWriters = append(warnErrorWriters, zapcore.AddSync(warnErrorFile))
		}
	}

	// 创建编码器
	var encoder zapcore.Encoder
	if cfg.Development {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// 创建多个核心
	var cores []zapcore.Core

	// 主核心 - 输出所有级别到主输出
	mainCore := zapcore.NewCore(encoder, zapcore.NewMultiWriteSyncer(allWriters...), level)
	cores = append(cores, mainCore)

	// Warn/Error核心 - 只输出warn和error级别到独立文件
	if len(warnErrorWriters) > 0 {
		warnErrorCore := zapcore.NewCore(
			encoder,
			zapcore.NewMultiWriteSyncer(warnErrorWriters...),
			zap.NewAtomicLevelAt(zapcore.WarnLevel),
		)
		cores = append(cores, warnErrorCore)
	}

	// 合并所有核心
	core := zapcore.NewTee(cores...)

	// Create logger options
	var options []zap.Option
	if cfg.Development {
		options = append(options, zap.Development())
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}
	options = append(options, zap.AddCaller())
	options = append(options, zap.AddCallerSkip(1))

	// Create logger
	Logger = zap.New(core, options...)
	Sugar = Logger.Sugar()

	return nil
}

// Sync flushes any buffered log entries
func Sync() error {
	if Logger != nil {
		return Logger.Sync()
	}
	return nil
}

// Debug logs a message at debug level
func Debug(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Debug(msg, fields...)
	}
}

// Info logs a message at info level
func Info(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Info(msg, fields...)
	}
}

// Warn logs a message at warn level
func Warn(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Warn(msg, fields...)
	}
}

// Error logs a message at error level
func Error(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Error(msg, fields...)
	}
}

// Fatal logs a message at fatal level and exits
func Fatal(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Fatal(msg, fields...)
	}
}

// With creates a child logger with fields
func With(fields ...zap.Field) *zap.Logger {
	if Logger != nil {
		return Logger.With(fields...)
	}
	return zap.NewNop()
}

// Debugf logs a formatted message at debug level
func Debugf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Debugf(template, args...)
	}
}

// Infof logs a formatted message at info level
func Infof(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Infof(template, args...)
	}
}

// Warnf logs a formatted message at warn level
func Warnf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Warnf(template, args...)
	}
}

// Errorf logs a formatted message at error level
func Errorf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Errorf(template, args...)
	}
}

// Fatalf logs a formatted message at fatal level and exits
func Fatalf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Fatalf(template, args...)
	}
}

// WithTag creates a logger with a specific tag for categorization
func WithTag(tag string) *zap.Logger {
	if Logger != nil {
		return Logger.With(zap.String("tag", tag))
	}
	return zap.NewNop()
}

// DebugWithTag logs a debug message with a tag
func DebugWithTag(tag string, msg string, fields ...zap.Field) {
	if Logger != nil {
		fields = append(fields, zap.String("tag", tag))
		Logger.Debug(msg, fields...)
	}
}

// InfoWithTag logs an info message with a tag
func InfoWithTag(tag string, msg string, fields ...zap.Field) {
	if Logger != nil {
		fields = append(fields, zap.String("tag", tag))
		Logger.Info(msg, fields...)
	}
}

// WarnWithTag logs a warn message with a tag
func WarnWithTag(tag string, msg string, fields ...zap.Field) {
	if Logger != nil {
		fields = append(fields, zap.String("tag", tag))
		Logger.Warn(msg, fields...)
	}
}

// ErrorWithTag logs an error message with a tag
func ErrorWithTag(tag string, msg string, fields ...zap.Field) {
	if Logger != nil {
		fields = append(fields, zap.String("tag", tag))
		Logger.Error(msg, fields...)
	}
}
