package imap

import (
	"testing"

	"flymail-core/types"
)

func TestFillThreadHeadersFromSection(t *testing.T) {
	// HEADER.FIELDS 区段：几行头 + 结尾空行（有的服务器不带结尾空行，两种都要能解析）
	for _, tail := range []string{"\r\n", ""} {
		section := []byte("References: <a@x>\r\n <b@y>\r\nIn-Reply-To: <b@y>\r\n" + tail)
		e := &types.ParsedEmail{}
		fillThreadHeadersFromSection(section, e)
		if e.InReplyTo != "b@y" || e.References != "a@x b@y" {
			t.Errorf("tail=%q: got %q / %q", tail, e.InReplyTo, e.References)
		}
	}
	// ENVELOPE 已给 In-Reply-To 时保留
	e := &types.ParsedEmail{InReplyTo: "env@id"}
	fillThreadHeadersFromSection([]byte("In-Reply-To: <other@id>\r\n\r\n"), e)
	if e.InReplyTo != "env@id" {
		t.Errorf("InReplyTo overwritten: %q", e.InReplyTo)
	}
	// 空区段不崩
	fillThreadHeadersFromSection(nil, &types.ParsedEmail{})
}
