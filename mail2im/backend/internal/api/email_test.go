package api

import (
	"testing"
)

// TestBuildEmailOrder 验证邮件排序字段构建
func TestBuildEmailOrder(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		order    string
		expected string
	}{
		{
			name:     "subject 升序",
			field:    "subject",
			order:    "asc",
			expected: "subject ASC",
		},
		{
			name:     "subject 降序",
			field:    "subject",
			order:    "desc",
			expected: "subject DESC",
		},
		{
			name:     "from 升序",
			field:    "from",
			order:    "ASC",
			expected: "`from` ASC",
		},
		{
			name:     "to 降序",
			field:    "to",
			order:    "DESC",
			expected: "`to` DESC",
		},
		{
			name:     "received_at 升序",
			field:    "received_at",
			order:    "asc",
			expected: "received_at ASC",
		},
		{
			name:     "created_at 降序",
			field:    "created_at",
			order:    "desc",
			expected: "created_at DESC",
		},
		{
			name:     "mail_type 升序",
			field:    "mail_type",
			order:    "asc",
			expected: "mail_type ASC",
		},
		{
			name:     "mailbox 降序",
			field:    "mailbox",
			order:    "desc",
			expected: "mailbox DESC",
		},
		{
			name:     "未知字段默认 received_at",
			field:    "unknown_field",
			order:    "asc",
			expected: "received_at ASC",
		},
		{
			name:     "空字段默认 received_at",
			field:    "",
			order:    "asc",
			expected: "received_at ASC",
		},
		{
			name:     "order 为 1 表示升序",
			field:    "subject",
			order:    "1",
			expected: "subject ASC",
		},
		{
			name:     "order 为空默认降序",
			field:    "subject",
			order:    "",
			expected: "subject DESC",
		},
		{
			name:     "order 为其他值默认降序",
			field:    "subject",
			order:    "random",
			expected: "subject DESC",
		},
		{
			name:     "大小写不敏感 ASC",
			field:    "subject",
			order:    "Asc",
			expected: "subject ASC",
		},
		{
			name:     "大小写不敏感 DESC",
			field:    "subject",
			order:    "Desc",
			expected: "subject DESC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildEmailOrder(tt.field, tt.order)
			if result != tt.expected {
				t.Errorf("buildEmailOrder(%q, %q) = %q, 期望 %q",
					tt.field, tt.order, result, tt.expected)
			}
		})
	}
}
