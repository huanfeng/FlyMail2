package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	// Logger is the global logger instance
	Logger *zap.Logger
	// Sugar is the sugared logger for convenience
	Sugar *zap.SugaredLogger
	// closers holds io.Closer writers (e.g. lumberjack) so Close() can release file handles.
	closers []func() error
)

// Config represents logger configuration
type Config struct {
	Level       string          `mapstructure:"level"`        // debug, info, warn, error
	Development bool            `mapstructure:"development"`  // development mode
	OutputPaths []string        `mapstructure:"output_paths"` // output destinations
	Rotation    *RotationConfig `mapstructure:"rotation"`     // log rotation config

	// 以下为可选增量字段；零值时沿用历史行为，保证既有调用方不受影响。
	EncoderFormat            string `mapstructure:"encoder_format"`              // "json"/"console"；空→按 Development 推断
	WarnErrorPath            string `mapstructure:"warn_error_path"`             // 分离 warn/error 文件路径；空→沿用 ./logs/warn_error.log
	DisableSeparateWarnError bool   `mapstructure:"disable_separate_warn_error"` // true→不生成分离文件
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
	// Reset closers from any previous Init call
	closers = nil

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

	var allWriters []zapcore.WriteSyncer
	var warnErrorWriters []zapcore.WriteSyncer

	if len(cfg.OutputPaths) > 0 {
		for _, path := range cfg.OutputPaths {
			switch path {
			case "stdout":
				allWriters = append(allWriters, zapcore.AddSync(os.Stdout))
			case "stderr":
				allWriters = append(allWriters, zapcore.AddSync(os.Stderr))
			default:
				if cfg.Rotation != nil {
					lj := &lumberjack.Logger{
						Filename:   path,
						MaxSize:    cfg.Rotation.MaxSize,
						MaxBackups: cfg.Rotation.MaxBackups,
						MaxAge:     cfg.Rotation.MaxAge,
						Compress:   cfg.Rotation.Compress,
					}
					closers = append(closers, lj.Close)
					allWriters = append(allWriters, zapcore.AddSync(lj))
				} else {
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

	// 分离的 warn/error 文件输出。
	// 兼容：WarnErrorPath 为空且非 development 时，沿用历史默认 ./logs/warn_error.log。
	if !cfg.DisableSeparateWarnError {
		warnErrorPath := cfg.WarnErrorPath
		if warnErrorPath == "" && !cfg.Development {
			warnErrorPath = "./logs/warn_error.log"
		}
		if warnErrorPath != "" {
			if dir := filepath.Dir(warnErrorPath); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
			}
			if cfg.Rotation != nil {
				lj := &lumberjack.Logger{
					Filename:   warnErrorPath,
					MaxSize:    cfg.Rotation.MaxSize,
					MaxBackups: cfg.Rotation.MaxBackups,
					MaxAge:     cfg.Rotation.MaxAge,
					Compress:   cfg.Rotation.Compress,
				}
				closers = append(closers, lj.Close)
				warnErrorWriters = append(warnErrorWriters, zapcore.AddSync(lj))
			} else {
				warnErrorFile, err := os.OpenFile(warnErrorPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
				if err != nil {
					return err
				}
				warnErrorWriters = append(warnErrorWriters, zapcore.AddSync(warnErrorFile))
			}
		}
	}

	var encoder zapcore.Encoder
	useConsole := cfg.EncoderFormat == "console" || (cfg.EncoderFormat == "" && cfg.Development)
	if useConsole {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	var cores []zapcore.Core

	mainCore := zapcore.NewCore(encoder, zapcore.NewMultiWriteSyncer(allWriters...), level)
	cores = append(cores, mainCore)

	if len(warnErrorWriters) > 0 {
		warnErrorCore := zapcore.NewCore(
			encoder,
			zapcore.NewMultiWriteSyncer(warnErrorWriters...),
			zap.NewAtomicLevelAt(zapcore.WarnLevel),
		)
		cores = append(cores, warnErrorCore)
	}

	core := zapcore.NewTee(cores...)

	var options []zap.Option
	if cfg.Development {
		options = append(options, zap.Development())
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}
	options = append(options, zap.AddCaller())
	options = append(options, zap.AddCallerSkip(1))

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

// Close flushes and closes all underlying writers (e.g. lumberjack file handles).
// Call this during shutdown or in tests to release file handles before cleanup.
// After Close returns, Logger and Sugar are nil; subsequent log calls become no-ops.
func Close() error {
	_ = Sync()
	var lastErr error
	for _, fn := range closers {
		if err := fn(); err != nil {
			lastErr = err
		}
	}
	closers = nil
	Logger = nil
	Sugar = nil
	return lastErr
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

// String is a convenience re-export of zap.String for use within this package and tests.
func String(key, val string) zap.Field {
	return zap.String(key, val)
}
