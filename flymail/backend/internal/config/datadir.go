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

// PortableMarker 是便携模式标记文件名：exe 同目录存在该文件时，
// 桌面形态数据落在 <exe 目录>/data（跟着 exe 走，便于 U 盘/绿色部署）。
const PortableMarker = "portable.txt"

// DesktopDataDir 返回桌面形态的数据目录，优先级：
//  1. FLYMAIL_DATA_DIR 环境变量；
//  2. exe 同目录存在 portable.txt 便携标记 → <exe 目录>/data；
//  3. OS 用户数据目录（Windows: %APPDATA%\FlyMail）。
func DesktopDataDir() string {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	return desktopDataDir(os.Getenv("FLYMAIL_DATA_DIR"), exeDir)
}

// desktopDataDir 是 DesktopDataDir 的可测试内核。
func desktopDataDir(envDir, exeDir string) string {
	if envDir != "" {
		return envDir
	}
	if exeDir != "" {
		if _, err := os.Stat(filepath.Join(exeDir, PortableMarker)); err == nil {
			return filepath.Join(exeDir, "data")
		}
	}
	return UserDataDir()
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
