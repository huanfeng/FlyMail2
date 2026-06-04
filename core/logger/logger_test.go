package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetLogger 释放全局 logger 持有的文件句柄，避免 Windows 下 TempDir 清理失败。
func resetLogger() {
	_ = Close()
}

// 新增字段：WarnErrorPath 指定分离文件路径
func TestInit_WarnErrorPathHonored(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(resetLogger) // LIFO：在 TempDir 清理之前释放文件句柄
	warnPath := filepath.Join(dir, "we.log")
	mainPath := filepath.Join(dir, "main.log")
	cfg := &Config{
		Level:         "info",
		Development:   false,
		OutputPaths:   []string{mainPath},
		EncoderFormat: "json",
		WarnErrorPath: warnPath,
		Rotation:      &RotationConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Error("boom")
	_ = Sync()
	if _, err := os.Stat(warnPath); err != nil {
		t.Fatalf("warn_error 文件未按 WarnErrorPath 生成: %v", err)
	}
}

// 新增字段：DisableSeparateWarnError 关闭分离文件
func TestInit_DisableSeparateWarnError(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(resetLogger) // LIFO：在 TempDir 清理之前释放文件句柄
	cfg := &Config{
		Level:                    "info",
		Development:              false,
		OutputPaths:              []string{filepath.Join(dir, "main.log")},
		EncoderFormat:            "json",
		DisableSeparateWarnError: true,
		WarnErrorPath:            filepath.Join(dir, "we.log"),
		Rotation:                 &RotationConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Error("boom")
	_ = Sync()
	if _, err := os.Stat(filepath.Join(dir, "we.log")); !os.IsNotExist(err) {
		t.Fatalf("DisableSeparateWarnError=true 时不应生成分离文件, err=%v", err)
	}
}

// 新增字段：EncoderFormat=json 即使非 dev 也输出可解析 JSON
func TestInit_EncoderFormatJSON(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(resetLogger) // LIFO：在 TempDir 清理之前释放文件句柄
	mainPath := filepath.Join(dir, "main.log")
	cfg := &Config{
		Level:                    "info",
		Development:              false,
		OutputPaths:              []string{mainPath},
		EncoderFormat:            "json",
		DisableSeparateWarnError: true,
		Rotation:                 &RotationConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Info("hello", String("k", "v"))
	_ = Sync()
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main.log: %v", err)
	}
	line := strings.TrimSpace(string(data))
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("日志行不是合法 JSON: %q err=%v", line, err)
	}
	if m["msg"] != "hello" || m["k"] != "v" {
		t.Fatalf("JSON 字段不符: %v", m)
	}
}

// 兼容性：WarnErrorPath 为空且非 dev 时，沿用旧默认 ./logs/warn_error.log
func TestInit_DefaultWarnErrorPathUnchanged(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(resetLogger) // LIFO：在 TempDir 清理之前释放文件句柄
	t.Chdir(dir)           // Go 1.24+：隔离工作目录，避免污染仓库
	cfg := &Config{
		Level:         "info",
		Development:   false,
		OutputPaths:   []string{filepath.Join(dir, "main.log")},
		EncoderFormat: "json",
		Rotation:      &RotationConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Error("boom")
	_ = Sync()
	if _, err := os.Stat(filepath.Join(dir, "logs", "warn_error.log")); err != nil {
		t.Fatalf("默认 ./logs/warn_error.log 行为被破坏: %v", err)
	}
}
