package parser

import (
	"fmt"
	"io"
	"mime"

	gomessage "github.com/emersion/go-message"
)

// wordDecoder is reused across calls (stateless and safe for concurrent use).
//
// CharsetReader 复用正文用的字符集解码器（GBK/GB2312/GB18030/Big5/euc-cn 等），
// 使 =?GBK?B?...?= 这类非 UTF-8 的中文编码字头也能正确解码——mime.WordDecoder
// 默认只认 utf-8/us-ascii/iso-8859-1，缺了它中文主题会原样返回。
// 用闭包惰性委派可避免包内 init 顺序问题（RegisterCharsets 幂等）。
var wordDecoder = &mime.WordDecoder{
	CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
		RegisterCharsets()
		if gomessage.CharsetReader != nil {
			return gomessage.CharsetReader(charset, input)
		}
		return nil, fmt.Errorf("unhandled charset %q", charset)
	},
}

// DecodeMIMEHeader decodes an RFC 2047 encoded header value.
// Returns the original string unchanged if decoding fails.
func DecodeMIMEHeader(s string) string {
	decoded, err := wordDecoder.DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}
