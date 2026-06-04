package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flymail-core/logger"

	"go.uber.org/zap"
)

func TestSetup_WritesParsableJSON(t *testing.T) {
	dir := t.TempDir()
	closeFn, err := Setup(Options{
		Dir:     dir,
		Console: false,
		Level:   "info",
		Format:  "json",
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	logger.Info("接入测试", zap.Uint("account_id", 7))
	_ = closeFn()

	data, err := os.ReadFile(filepath.Join(dir, "flymail.log"))
	if err != nil {
		t.Fatalf("read flymail.log: %v", err)
	}
	last := ""
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(l) != "" {
			last = l
		}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(last), &m); err != nil {
		t.Fatalf("日志行非合法 JSON: %q err=%v", last, err)
	}
	if m["msg"] != "接入测试" {
		t.Fatalf("msg 字段不符: %v", m)
	}
	if _, ok := m["account_id"]; !ok {
		t.Fatalf("缺 account_id 字段: %v", m)
	}
}

func TestSetup_LevelFiltersDebug(t *testing.T) {
	dir := t.TempDir()
	closeFn, err := Setup(Options{Dir: dir, Console: false, Level: "info", Format: "json"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	logger.Debug("不应出现")
	logger.Info("应出现")
	_ = closeFn()

	data, _ := os.ReadFile(filepath.Join(dir, "flymail.log"))
	if strings.Contains(string(data), "不应出现") {
		t.Fatalf("info 级别下 debug 日志不应写入")
	}
	if !strings.Contains(string(data), "应出现") {
		t.Fatalf("info 日志缺失")
	}
}
