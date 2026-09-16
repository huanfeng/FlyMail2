package imap

import (
	"bytes"
	"testing"

	"flymail-core/parser"
	"flymail-core/types"
)

// 元数据抓取拿到的头区段要能解析出线程头。
//
// 这条原先钉的是 fillThreadHeadersFromSection——那个函数只解析 In-Reply-To /
// References，其余字段靠服务端的 ENVELOPE。现在信封字段也一起从头里取了
// （见 envelopeHeaderSection 的说明），解析入口换成 parser.ParseHeaders，
// 但「区段可能不以空行结尾」这个坑依然在，所以这条用例保留。
func TestParseHeadersFromSection(t *testing.T) {
	// 有的服务器返回的区段不带结尾空行，两种都要能解析
	for _, tail := range []string{"\r\n", ""} {
		section := []byte("References: <a@x>\r\n <b@y>\r\nIn-Reply-To: <b@y>\r\n" + tail)
		e := &types.ParsedEmail{}
		if err := parser.ParseHeaders(bytes.NewReader(section), e); err != nil {
			t.Fatalf("tail=%q: ParseHeaders 出错 %v", tail, err)
		}
		if e.InReplyTo != "b@y" || e.References != "a@x b@y" {
			t.Errorf("tail=%q: got %q / %q", tail, e.InReplyTo, e.References)
		}
	}

	// 空区段不崩
	if err := parser.ParseHeaders(bytes.NewReader(nil), &types.ParsedEmail{}); err != nil {
		t.Errorf("空区段报错了：%v", err)
	}
}
