package parser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime/quotedprintable"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"

	"flymail-core/types"
)

// encodeTo 用给定 transform.Transformer 把 UTF-8 字符串编码为目标字节。
func encodeTo(t *testing.T, tr transform.Transformer, s string) []byte {
	t.Helper()
	b, _, err := transform.Bytes(tr, []byte(s))
	if err != nil {
		t.Fatalf("encode %q: %v", s, err)
	}
	return b
}

// b64 单行 base64。
func b64(b []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(b))
}

// qp quoted-printable 编码。
func qp(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := quotedprintable.NewWriter(&buf)
	if _, err := w.Write(b); err != nil {
		t.Fatalf("qp write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("qp close: %v", err)
	}
	return buf.Bytes()
}

// parseSingleBody 构造一封单部件邮件（指定 charset 与传输编码）并解析，返回结果。
func parseSingleBody(t *testing.T, contentType, cte string, body []byte) types.ParsedEmail {
	t.Helper()
	var raw bytes.Buffer
	fmt.Fprintf(&raw, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&raw, "Content-Type: %s\r\n", contentType)
	fmt.Fprintf(&raw, "Content-Transfer-Encoding: %s\r\n\r\n", cte)
	raw.Write(body)

	var email types.ParsedEmail
	if err := ParseBody(&raw, &email, true); err != nil {
		t.Fatalf("ParseBody: %v", err)
	}
	return email
}

func TestParseBody_ChineseCharsets(t *testing.T) {
	const wantText = "中文正文测试内容"

	tests := []struct {
		name        string
		contentType string
		cte         string
		body        []byte
		wantHTML    bool
	}{
		{
			name:        "GBK + base64 (text/plain)",
			contentType: "text/plain; charset=GBK",
			cte:         "base64",
			body:        b64(encodeTo(t, simplifiedchinese.GBK.NewEncoder(), wantText)),
		},
		{
			name:        "GBK + quoted-printable (text/plain)",
			contentType: "text/plain; charset=gbk",
			cte:         "quoted-printable",
			body:        qp(t, encodeTo(t, simplifiedchinese.GBK.NewEncoder(), wantText)),
		},
		{
			name:        "GB18030 + base64 (text/html)",
			contentType: "text/html; charset=GB18030",
			cte:         "base64",
			body:        b64(encodeTo(t, simplifiedchinese.GB18030.NewEncoder(), "<p>"+wantText+"</p>")),
			wantHTML:    true,
		},
		{
			name: "GB2312 + base64 (text/plain)",
			// 邮件里的 GB2312 实为 EUC 系（GBK 是其超集），用 GBK 编码器产字节、
			// charset 标 GB2312 以验证解码端的别名处理。
			contentType: "text/plain; charset=GB2312",
			cte:         "base64",
			body:        b64(encodeTo(t, simplifiedchinese.GBK.NewEncoder(), wantText)),
		},
		{
			name:        "euc-cn 别名 + base64 (charset.go 特例)",
			contentType: "text/plain; charset=euc-cn",
			cte:         "base64",
			body:        b64(encodeTo(t, simplifiedchinese.GB18030.NewEncoder(), wantText)),
		},
		{
			name:        "Big5 繁体 + base64 (text/plain)",
			contentType: "text/plain; charset=Big5",
			cte:         "base64",
			body:        b64(encodeTo(t, traditionalchinese.Big5.NewEncoder(), "繁體中文")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email := parseSingleBody(t, tt.contentType, tt.cte, tt.body)

			got := email.TextBody
			if tt.wantHTML {
				got = email.HTMLBody
			}

			want := wantText
			if tt.contentType == "text/html; charset=GB18030" {
				want = "<p>" + wantText + "</p>"
			}
			if tt.name == "Big5 繁体 + base64 (text/plain)" {
				want = "繁體中文"
			}

			if got != want {
				t.Errorf("body = %q, want %q", got, want)
			}
		})
	}
}

// TestParseBody_GBKSubjectHeader 验证 GBK 编码的主题头经 ParseBody 的 fallback 路径
// 正确解码（依赖 DecodeMIMEHeader 的 CharsetReader）。
func TestParseBody_GBKSubjectHeader(t *testing.T) {
	subjGBK := encodeTo(t, simplifiedchinese.GBK.NewEncoder(), "中文主题行")
	subjHeader := "=?GBK?B?" + base64.StdEncoding.EncodeToString(subjGBK) + "?="

	var raw bytes.Buffer
	fmt.Fprintf(&raw, "Subject: %s\r\n", subjHeader)
	fmt.Fprintf(&raw, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
	raw.WriteString("body")

	var email types.ParsedEmail
	if err := ParseBody(&raw, &email, true); err != nil {
		t.Fatalf("ParseBody: %v", err)
	}
	if email.Subject != "中文主题行" {
		t.Errorf("Subject = %q, want 中文主题行", email.Subject)
	}
}
