package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// windowState 是持久化的窗口几何状态（<数据目录>/window_state.json）。
type windowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	HasPos    bool `json:"has_pos"` // 首次运行无历史位置时交给 OS 决定
	Maximised bool `json:"maximised"`
}

const (
	defaultWidth  = 1280
	defaultHeight = 820
)

func windowStatePath(dataDir string) string {
	return filepath.Join(dataDir, "window_state.json")
}

// loadWindowState 读取上次的窗口状态；缺失/损坏/数值异常时回到默认值。
func loadWindowState(dataDir string) *windowState {
	st := &windowState{Width: defaultWidth, Height: defaultHeight}
	raw, err := os.ReadFile(windowStatePath(dataDir))
	if err != nil {
		return st
	}
	if err := json.Unmarshal(raw, st); err != nil {
		return &windowState{Width: defaultWidth, Height: defaultHeight}
	}
	// 尺寸下限与 MinWidth/MinHeight 对齐；坐标做粗粒度合法性检查
	//（显示器变更导致的离屏坐标无法完全识别，异常时删 window_state.json 即可复位）。
	if st.Width < 960 || st.Height < 600 {
		st.Width, st.Height = defaultWidth, defaultHeight
	}
	if st.X < -32000 || st.X > 32000 || st.Y < -32000 || st.Y > 32000 {
		st.HasPos = false
	}
	return st
}

// restoreWindowPosition 在 OnStartup 时恢复上次的窗口位置（尺寸/最大化由 wails.Run 选项承担）。
func (d *desktop) restoreWindowPosition() {
	if d.state.HasPos && !d.state.Maximised {
		wailsrt.WindowSetPosition(d.ctx, d.state.X, d.state.Y)
	}
}

// saveWindowState 把当前窗口几何写盘。最大化/最小化时保留上次的普通尺寸，只更新状态位。
func (d *desktop) saveWindowState() {
	if d.ctx == nil {
		return
	}
	st := *d.state
	st.Maximised = wailsrt.WindowIsMaximised(d.ctx)
	if !st.Maximised && !wailsrt.WindowIsMinimised(d.ctx) {
		st.Width, st.Height = wailsrt.WindowGetSize(d.ctx)
		st.X, st.Y = wailsrt.WindowGetPosition(d.ctx)
		st.HasPos = true
	}
	*d.state = st

	raw, err := json.MarshalIndent(&st, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(windowStatePath(d.dataDir), raw, 0o644); err != nil {
		log.Printf("保存窗口状态失败: %v", err)
	}
}
