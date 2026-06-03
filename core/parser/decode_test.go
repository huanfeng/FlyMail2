package parser

import (
	"encoding/base64"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// bEncode 构造一个 RFC 2047 B-encoding（Base64）编码字。
func bEncode(charset string, raw []byte) string {
	return "=?" + charset + "?B?" + base64.StdEncoding.EncodeToString(raw) + "?="
}

// toGBK 把 UTF-8 字符串编码为 GBK 字节。
func toGBK(t *testing.T, s string) []byte {
	t.Helper()
	b, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("encode GBK %q: %v", s, err)
	}
	return b
}

func TestDecodeMIMEHeader(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "纯 ASCII 原样返回",
			in:   "Hello World",
			want: "Hello World",
		},
		{
			name: "UTF-8 Base64(B) 中文",
			in:   bEncode("UTF-8", []byte("测试主题")),
			want: "测试主题",
		},
		{
			name: "UTF-8 Quoted-Printable(Q) 中文",
			// "你好" 的 UTF-8 字节 e4 bd a0 e5 a5 bd 以 Q 编码
			in:   "=?UTF-8?Q?=E4=BD=A0=E5=A5=BD?=",
			want: "你好",
		},
		{
			name: "GBK Base64(B) 中文主题",
			in:   bEncode("GBK", toGBK(t, "中文主题")),
			want: "中文主题",
		},
		{
			name: "GB2312 Base64(B) 中文",
			in:   bEncode("GB2312", toGBK(t, "简体中文")),
			want: "简体中文",
		},
		{
			name: "混合：ASCII 前缀 + UTF-8 编码字",
			in:   "Re: " + bEncode("UTF-8", []byte("回复")),
			want: "Re: 回复",
		},
		{
			name: "非法编码字原样返回",
			in:   "=?UTF-8?B?not-valid-base64!!!?=",
			want: "=?UTF-8?B?not-valid-base64!!!?=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeMIMEHeader(tt.in)
			if got != tt.want {
				t.Errorf("DecodeMIMEHeader(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
