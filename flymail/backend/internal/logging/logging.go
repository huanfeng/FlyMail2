// Package logging 提供统一的文件日志（按大小轮转）+ 控制台输出，
// 并把标准库 log 与 gin 的输出都接到同一组 writer。
package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// Options 控制日志输出与轮转策略。
type Options struct {
	Dir        string // 日志目录（必填）
	Filename   string // 文件名，默认 flymail.log
	MaxSizeMB  int    // 单文件最大 MB，默认 10
	MaxBackups int    // 保留备份数，默认 5
	MaxAgeDays int    // 备份保留天数，默认 30
	Compress   bool   // 是否 gzip 压缩旧备份，默认 false
	Console    bool   // 是否同时输出到 stdout，默认 true
}

// Setup 初始化日志：创建目录、配置轮转文件写入器，并把标准库 log 输出
// 重定向到「文件(+控制台)」。返回组合 writer（供 gin 复用）与关闭函数。
func Setup(opts Options) (io.Writer, func() error, error) {
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
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, nil, err
	}

	lj := &lumberjack.Logger{
		Filename:   filepath.Join(opts.Dir, opts.Filename),
		MaxSize:    opts.MaxSizeMB,
		MaxBackups: opts.MaxBackups,
		MaxAge:     opts.MaxAgeDays,
		Compress:   opts.Compress,
	}

	var w io.Writer = lj
	if opts.Console {
		w = io.MultiWriter(os.Stdout, lj)
	}

	log.SetOutput(w)
	// 纯文本行：2026/06/02 10:30:01 <msg>。沿用标准库 log 风格，便于 tail/肉眼阅读。
	log.SetFlags(log.LstdFlags)

	return w, lj.Close, nil
}
