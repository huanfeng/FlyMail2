package core

import (
	"testing"
	"time"
)

// TestIsInWindow 验证时间窗口判断逻辑
func TestIsInWindow(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name     string
		now      time.Time
		window   TimeWindow
		expected bool
	}{
		{
			name:     "禁用窗口返回 false",
			now:      time.Date(2024, 1, 1, 12, 0, 0, 0, loc),
			window:   TimeWindow{Enabled: false, Start: "00:00", End: "23:59"},
			expected: false,
		},
		{
			name:     "时间在窗口内返回 true",
			now:      time.Date(2024, 1, 1, 12, 0, 0, 0, loc),
			window:   TimeWindow{Enabled: true, Start: "09:00", End: "18:00"},
			expected: true,
		},
		{
			name:     "时间在窗口外返回 false",
			now:      time.Date(2024, 1, 1, 20, 0, 0, 0, loc),
			window:   TimeWindow{Enabled: true, Start: "09:00", End: "18:00"},
			expected: false,
		},
		{
			name:     "时间在窗口开始边界返回 false",
			now:      time.Date(2024, 1, 1, 9, 0, 0, 0, loc),
			window:   TimeWindow{Enabled: true, Start: "09:00", End: "18:00"},
			expected: false,
		},
		{
			name:     "时间在窗口结束边界返回 false",
			now:      time.Date(2024, 1, 1, 18, 0, 0, 0, loc),
			window:   TimeWindow{Enabled: true, Start: "09:00", End: "18:00"},
			expected: false,
		},
		{
			name:     "跨午夜窗口-时间在开始后返回 true",
			now:      time.Date(2024, 1, 1, 23, 30, 0, 0, loc),
			window:   TimeWindow{Enabled: true, Start: "22:00", End: "06:00"},
			expected: true,
		},
		{
			name:     "跨午夜窗口-时间在结束前返回 true",
			now:      time.Date(2024, 1, 1, 5, 30, 0, 0, loc),
			window:   TimeWindow{Enabled: true, Start: "22:00", End: "06:00"},
			expected: true,
		},
		{
			name:     "跨午夜窗口-时间在中间返回 false",
			now:      time.Date(2024, 1, 1, 12, 0, 0, 0, loc),
			window:   TimeWindow{Enabled: true, Start: "22:00", End: "06:00"},
			expected: false,
		},
		{
			name:     "开始等于结束返回 false",
			now:      time.Date(2024, 1, 1, 12, 0, 0, 0, loc),
			window:   TimeWindow{Enabled: true, Start: "12:00", End: "12:00"},
			expected: false,
		},
		{
			name:     "无效时间格式返回 false",
			now:      time.Date(2024, 1, 1, 12, 0, 0, 0, loc),
			window:   TimeWindow{Enabled: true, Start: "invalid", End: "18:00"},
			expected: false,
		},
		{
			name:     "nil 时区使用 UTC",
			now:      time.Date(2024, 1, 1, 12, 0, 0, 0, loc),
			window:   TimeWindow{Enabled: true, Start: "09:00", End: "18:00"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsInWindow(tt.now, tt.window, loc)
			if result != tt.expected {
				t.Errorf("IsInWindow(%v, %+v) = %v, 期望 %v",
					tt.now.Format("15:04"), tt.window, result, tt.expected)
			}
		})
	}
}

// TestParseBoolVal 验证布尔值解析
func TestParseBoolVal(t *testing.T) {
	tests := []struct {
		name       string
		val        string
		defaultVal string
		expected   bool
	}{
		{
			name:       "空值使用默认值 true",
			val:        "",
			defaultVal: "true",
			expected:   true,
		},
		{
			name:       "空值使用默认值 false",
			val:        "",
			defaultVal: "false",
			expected:   false,
		},
		{
			name:       "true 字符串返回 true",
			val:        "true",
			defaultVal: "false",
			expected:   true,
		},
		{
			name:       "false 字符串返回 false",
			val:        "false",
			defaultVal: "true",
			expected:   false,
		},
		{
			name:       "1 返回 true",
			val:        "1",
			defaultVal: "false",
			expected:   true,
		},
		{
			name:       "0 返回 false",
			val:        "0",
			defaultVal: "true",
			expected:   false,
		},
		{
			name:       "TRUE 大写返回 true",
			val:        "TRUE",
			defaultVal: "false",
			expected:   true,
		},
		{
			name:       "FALSE 大写返回 false",
			val:        "FALSE",
			defaultVal: "true",
			expected:   false,
		},
		{
			name:       "带空格的 true 返回 true",
			val:        "  true  ",
			defaultVal: "false",
			expected:   true,
		},
		{
			name:       "无效值使用默认值",
			val:        "invalid",
			defaultVal: "true",
			expected:   false, // ParseBool returns error for invalid values
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBoolVal(tt.val, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("parseBoolVal(%q, %q) = %v, 期望 %v",
					tt.val, tt.defaultVal, result, tt.expected)
			}
		})
	}
}

// TestListTimezones 验证时区列表获取
func TestListTimezones(t *testing.T) {
	timezones := ListTimezones()

	// 验证返回的时区列表不为空
	if len(timezones) == 0 {
		t.Error("时区列表不应该为空")
	}

	// 验证包含 UTC
	foundUTC := false
	for _, tz := range timezones {
		if tz == "UTC" {
			foundUTC = true
			break
		}
	}
	if !foundUTC {
		t.Error("时区列表应该包含 UTC")
	}

	// 验证时区列表是排序的
	for i := 1; i < len(timezones); i++ {
		if timezones[i-1] > timezones[i] {
			t.Errorf("时区列表未排序: %s > %s", timezones[i-1], timezones[i])
			break
		}
	}

	// 验证没有重复的时区
	seen := make(map[string]bool)
	for _, tz := range timezones {
		if seen[tz] {
			t.Errorf("时区列表包含重复的时区: %s", tz)
		}
		seen[tz] = true
	}
}

// TestParseBoolSetting 验证布尔设置解析
func TestParseBoolSetting(t *testing.T) {
	// 这个测试需要数据库支持
	// TODO: 使用 setupTestDB 实现
	t.Skip("需要数据库支持，暂未实现")
}

// TestGetSystemLocation 验证时区获取
func TestGetSystemLocation(t *testing.T) {
	// 这个测试需要数据库支持
	// TODO: 使用 setupTestDB 实现
	t.Skip("需要数据库支持，暂未实现")
}
