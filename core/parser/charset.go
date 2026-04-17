package parser

import (
	"fmt"
	"io"
	"strings"
	"sync"

	gomessage "github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // registers standard charsets (GBK, GB2312, Big5, etc.)
	"golang.org/x/text/encoding/simplifiedchinese"
)

var charsetOnce sync.Once

// RegisterCharsets registers additional charset decoders beyond the standard set.
// Safe to call multiple times; actual registration happens only once.
func RegisterCharsets() {
	charsetOnce.Do(func() {
		original := gomessage.CharsetReader
		gomessage.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
			switch strings.ToLower(charset) {
			case "euc-cn":
				// euc-cn is a legacy alias; decode via GB18030 which is backward-compatible
				return simplifiedchinese.GB18030.NewDecoder().Reader(input), nil
			}

			if original != nil {
				return original(charset, input)
			}
			return nil, fmt.Errorf("unhandled charset %q", charset)
		}
	})
}
