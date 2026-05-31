package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// ResolveDataDir 返回默认数据目录。server/Docker 默认 ./data；桌面形态由 desktop 入口显式传入。
func ResolveDataDir() string {
	if d := os.Getenv("FLYMAIL_DATA_DIR"); d != "" {
		return d
	}
	return "data"
}

// UserDataDir 返回桌面形态的 OS 用户数据目录（供 cmd/desktop 使用）。
func UserDataDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		if runtime.GOOS == "windows" {
			base = os.Getenv("APPDATA")
		} else {
			home, _ := os.UserHomeDir()
			base = home
		}
	}
	return filepath.Join(base, "FlyMail")
}
