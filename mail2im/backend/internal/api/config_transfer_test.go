package api

import (
	"testing"
)

// TestParseSectionSelection 验证配置导入部分选择解析
func TestParseSectionSelection(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected map[string]bool
	}{
		{
			name:  "nil 输入返回全部启用",
			input: nil,
			expected: map[string]bool{
				"accounts": true,
				"proxies":  true,
				"settings": true,
				"channels": true,
			},
		},
		{
			name:  "空字符串返回全部启用",
			input: "",
			expected: map[string]bool{
				"accounts": true,
				"proxies":  true,
				"settings": true,
				"channels": true,
			},
		},
		{
			name:  "只选择 accounts",
			input: "accounts",
			expected: map[string]bool{
				"accounts": true,
				"proxies":  false,
				"settings": false,
				"channels": false,
			},
		},
		{
			name:  "选择多个部分",
			input: "accounts,proxies",
			expected: map[string]bool{
				"accounts": true,
				"proxies":  true,
				"settings": false,
				"channels": false,
			},
		},
		{
			name:  "选择全部",
			input: "accounts,proxies,settings,channels",
			expected: map[string]bool{
				"accounts": true,
				"proxies":  true,
				"settings": true,
				"channels": true,
			},
		},
		{
			name:  "大小写不敏感",
			input: "Accounts,PROXIES",
			expected: map[string]bool{
				"accounts": true,
				"proxies":  true,
				"settings": false,
				"channels": false,
			},
		},
		{
			name:  "带空格",
			input: "  accounts , proxies  ",
			expected: map[string]bool{
				"accounts": true,
				"proxies":  true,
				"settings": false,
				"channels": false,
			},
		},
		{
			name:  "包含无效部分",
			input: "accounts,invalid,proxies",
			expected: map[string]bool{
				"accounts": true,
				"proxies":  true,
				"settings": false,
				"channels": false,
			},
		},
		{
			name:  "字符串切片输入",
			input: []string{"accounts", "settings"},
			expected: map[string]bool{
				"accounts": true,
				"proxies":  false,
				"settings": true,
				"channels": false,
			},
		},
		{
			name:  "空字符串切片返回全部启用",
			input: []string{},
			expected: map[string]bool{
				"accounts": true,
				"proxies":  true,
				"settings": true,
				"channels": true,
			},
		},
		{
			name:  "字符串切片大小写不敏感",
			input: []string{"ACCOUNTS", "Proxies"},
			expected: map[string]bool{
				"accounts": true,
				"proxies":  true,
				"settings": false,
				"channels": false,
			},
		},
		{
			name:  "其他类型返回全部启用",
			input: 123,
			expected: map[string]bool{
				"accounts": true,
				"proxies":  true,
				"settings": true,
				"channels": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSectionSelection(tt.input)

			// 验证结果长度
			if len(result) != len(tt.expected) {
				t.Errorf("parseSectionSelection(%v) 长度 = %d, 期望 %d", tt.input, len(result), len(tt.expected))
				return
			}

			// 验证所有期望的部分都正确
			for key, expected := range tt.expected {
				if result[key] != expected {
					t.Errorf("parseSectionSelection(%v)[%q] = %v, 期望 %v", tt.input, key, result[key], expected)
				}
			}
		})
	}
}
