package core

import (
	"testing"
)

// TestHashToken 验证 token 哈希函数
func TestHashToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "空字符串",
			token: "",
		},
		{
			name:  "简单 token",
			token: "test-token",
		},
		{
			name:  "复杂 token",
			token: "abc123!@#$%^&*()",
		},
		{
			name:  "中文 token",
			token: "测试令牌",
		},
		{
			name:  "长 token",
			token: "a-very-long-token-that-should-still-produce-a-fixed-length-hash-1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashToken(tt.token)

			// 验证哈希长度（SHA256 产生 64 个十六进制字符）
			if len(result) != 64 {
				t.Errorf("hashToken(%q) 长度 = %d, 期望 64", tt.token, len(result))
			}

			// 验证哈希是有效的十六进制字符串
			for _, c := range result {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("hashToken(%q) 包含无效字符: %c", tt.token, c)
					break
				}
			}
		})
	}
}

// TestHashToken_Consistency 验证哈希函数的一致性
func TestHashToken_Consistency(t *testing.T) {
	token := "my-secret-token-12345"

	// 多次哈希应该得到相同的结果
	hash1 := hashToken(token)
	hash2 := hashToken(token)
	hash3 := hashToken(token)

	if hash1 != hash2 || hash2 != hash3 {
		t.Errorf("哈希函数不一致: %q, %q, %q", hash1, hash2, hash3)
	}
}

// TestHashToken_DifferentInputs 验证不同输入得到不同哈希
func TestHashToken_DifferentInputs(t *testing.T) {
	tokens := []string{
		"token1",
		"token2",
		"token3",
		"different-token",
	}

	hashes := make(map[string]string)
	for _, token := range tokens {
		hash := hashToken(token)
		if prev, exists := hashes[hash]; exists {
			t.Errorf("不同的 token 得到相同的哈希: %q 和 %q 都得到 %q", prev, token, hash)
		}
		hashes[hash] = token
	}
}

// TestHashToken_Length 验证哈希长度
func TestHashToken_Length(t *testing.T) {
	tokens := []string{
		"",
		"short",
		"a-very-long-token-that-should-still-produce-a-fixed-length-hash",
	}

	for _, token := range tokens {
		hash := hashToken(token)
		if len(hash) != 64 { // SHA256 produces 64 hex characters
			t.Errorf("hashToken(%q) 长度 = %d, 期望 64", token, len(hash))
		}
	}
}

// TestGenerateRandomPassword 验证随机密码生成
func TestGenerateRandomPassword(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{
			name:   "默认长度（0）",
			length: 0,
		},
		{
			name:   "默认长度（负数）",
			length: -1,
		},
		{
			name:   "长度 8",
			length: 8,
		},
		{
			name:   "长度 16",
			length: 16,
		},
		{
			name:   "长度 32",
			length: 32,
		},
		{
			name:   "长度 64",
			length: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			password, err := generateRandomPassword(tt.length)
			if err != nil {
				t.Fatalf("generateRandomPassword(%d) 返回错误: %v", tt.length, err)
			}

			expectedLength := tt.length
			if expectedLength <= 0 {
				expectedLength = 12
			}

			// 验证密码长度
			if len(password) != expectedLength {
				t.Errorf("密码长度 = %d, 期望 %d", len(password), expectedLength)
			}

			// 验证密码不为空
			if password == "" {
				t.Error("密码不应该为空")
			}

			// 验证密码只包含有效字符（字母、数字、@#$%）
			validChars := "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789@#$%"
			for _, c := range password {
				found := false
				for _, valid := range validChars {
					if c == valid {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("密码包含无效字符: %c", c)
					break
				}
			}
		})
	}
}

// TestGenerateRandomPassword_Uniqueness 验证生成的密码是唯一的
func TestGenerateRandomPassword_Uniqueness(t *testing.T) {
	passwords := make(map[string]bool)
	numPasswords := 100

	for i := 0; i < numPasswords; i++ {
		password, err := generateRandomPassword(16)
		if err != nil {
			t.Fatalf("generateRandomPassword(16) 返回错误: %v", err)
		}

		if passwords[password] {
			t.Errorf("生成了重复的密码: %q", password)
		}
		passwords[password] = true
	}
}
