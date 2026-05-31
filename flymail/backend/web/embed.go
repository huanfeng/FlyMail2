package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS 返回前端构建产物（dist 根）。
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
