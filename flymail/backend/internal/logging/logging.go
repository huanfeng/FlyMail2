// Package logging 基于 flymail-core/logger(zap) 提供结构化 JSON 日志。
// 主输出 <Dir>/flymail.log（lumberjack 轮转）+ 可选 stdout；
// warn/error 另写 <Dir>/warn_error.log；并把标准库 log 收编进 zap。
package logging

import (
	"path/filepath"

	"flymail-core/logger"

	"go.uber.org/zap"
)

// Options 控制日志输出与轮转策略。
type Options struct {
	Dir        string // 日志目录（必填）
	Filename   string // 文件名，默认 flymail.log
	MaxSizeMB  int    // 单文件最大 MB，默认 10
	MaxBackups int    // 保留备份数，默认 5
	MaxAgeDays int    // 备份保留天数，默认 30
	Compress   bool   // 是否 gzip 压缩旧备份，默认 false
	Console    bool   // 是否同时输出到 stdout
	Level      string // debug/info/warn/error，默认 info
	Format     string // json/console，默认 json
}

// Setup 初始化全局 zap logger（flymail-core/logger 单例），返回 close 函数（内部 Sync）。
func Setup(opts Options) (func() error, error) {
	if opts.Filename == "" {
		opts.Filename = "flymail.log"
	}
	if opts.MaxSizeMB <= 0 {
		opts.MaxSizeMB = 10
	}
	if opts.MaxBackups <= 0 {
		opts.MaxBackups = 5
	}
	if opts.MaxAgeDays <= 0 {
		opts.MaxAgeDays = 30
	}
	if opts.Level == "" {
		opts.Level = "info"
	}
	if opts.Format == "" {
		opts.Format = "json"
	}

	outputs := []string{filepath.Join(opts.Dir, opts.Filename)}
	if opts.Console {
		outputs = append(outputs, "stdout")
	}

	cfg := &logger.Config{
		Level:         opts.Level,
		Development:   false,
		OutputPaths:   outputs,
		EncoderFormat: opts.Format,
		WarnErrorPath: filepath.Join(opts.Dir, "warn_error.log"),
		Rotation: &logger.RotationConfig{
			MaxSize:    opts.MaxSizeMB,
			MaxBackups: opts.MaxBackups,
			MaxAge:     opts.MaxAgeDays,
			Compress:   opts.Compress,
		},
	}
	if err := logger.Init(cfg); err != nil {
		return nil, err
	}

	// 把标准库 log（残留 log.Printf 与第三方）收编进 zap（迁移期安全网）。
	_ = zap.RedirectStdLog(logger.Logger)

	return func() error { return logger.Close() }, nil
}
