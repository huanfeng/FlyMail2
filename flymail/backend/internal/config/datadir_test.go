package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopDataDirEnvOverride(t *testing.T) {
	got := desktopDataDir("D:/custom", t.TempDir())
	if got != "D:/custom" {
		t.Errorf("desktopDataDir env override = %q, want D:/custom", got)
	}
}

func TestDesktopDataDirPortableMarker(t *testing.T) {
	exeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(exeDir, PortableMarker), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got := desktopDataDir("", exeDir)
	if want := filepath.Join(exeDir, "data"); got != want {
		t.Errorf("desktopDataDir portable = %q, want %q", got, want)
	}
}

func TestDesktopDataDirFallbackToUserDir(t *testing.T) {
	// 无环境变量、无便携标记 → OS 用户数据目录
	got := desktopDataDir("", t.TempDir())
	if want := UserDataDir(); got != want {
		t.Errorf("desktopDataDir fallback = %q, want %q", got, want)
	}
}
