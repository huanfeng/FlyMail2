package dispatcher

import (
	"testing"
)

// TestNormalizeQuietMode 验证静默模式标准化
func TestNormalizeQuietMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "override 小写",
			input:    "override",
			expected: "override",
		},
		{
			name:     "override 大写",
			input:    "OVERRIDE",
			expected: "override",
		},
		{
			name:     "override 混合大小写",
			input:    "Override",
			expected: "override",
		},
		{
			name:     "off 小写",
			input:    "off",
			expected: "off",
		},
		{
			name:     "off 大写",
			input:    "OFF",
			expected: "off",
		},
		{
			name:     "off 混合大小写",
			input:    "Off",
			expected: "off",
		},
		{
			name:     "global 小写",
			input:    "global",
			expected: "global",
		},
		{
			name:     "global 大写",
			input:    "GLOBAL",
			expected: "global",
		},
		{
			name:     "空字符串返回 global",
			input:    "",
			expected: "global",
		},
		{
			name:     "未知值返回 global",
			input:    "unknown",
			expected: "global",
		},
		{
			name:     "带空格的值返回 global",
			input:    "  override  ",
			expected: "global", // 没有 TrimSpace，所以不会匹配
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeQuietMode(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeQuietMode(%q) = %q, 期望 %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestExtractUint 验证从 map 中提取 uint 值
func TestExtractUint(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]any
		key      string
		expected uint
		ok       bool
	}{
		{
			name:     "uint 值",
			m:        map[string]any{"id": uint(123)},
			key:      "id",
			expected: 123,
			ok:       true,
		},
		{
			name:     "float64 值",
			m:        map[string]any{"id": float64(456)},
			key:      "id",
			expected: 456,
			ok:       true,
		},
		{
			name:     "int 值",
			m:        map[string]any{"id": int(789)},
			key:      "id",
			expected: 789,
			ok:       true,
		},
		{
			name:     "int64 值（不支持）",
			m:        map[string]any{"id": int64(101112)},
			key:      "id",
			expected: 0,
			ok:       false,
		},
		{
			name:     "uint64 值（不支持）",
			m:        map[string]any{"id": uint64(131415)},
			key:      "id",
			expected: 0,
			ok:       false,
		},
		{
			name:     "键不存在",
			m:        map[string]any{"other": uint(123)},
			key:      "id",
			expected: 0,
			ok:       false,
		},
		{
			name:     "空 map",
			m:        map[string]any{},
			key:      "id",
			expected: 0,
			ok:       false,
		},
		{
			name:     "nil map",
			m:        nil,
			key:      "id",
			expected: 0,
			ok:       false,
		},
		{
			name:     "字符串值",
			m:        map[string]any{"id": "123"},
			key:      "id",
			expected: 0,
			ok:       false,
		},
		{
			name:     "布尔值",
			m:        map[string]any{"id": true},
			key:      "id",
			expected: 0,
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := extractUint(tt.m, tt.key)
			if ok != tt.ok {
				t.Errorf("extractUint(%v, %q) ok = %v, 期望 %v", tt.m, tt.key, ok, tt.ok)
			}
			if result != tt.expected {
				t.Errorf("extractUint(%v, %q) = %d, 期望 %d", tt.m, tt.key, result, tt.expected)
			}
		})
	}
}
