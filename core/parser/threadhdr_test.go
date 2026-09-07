package parser

import (
	"reflect"
	"strings"
	"testing"

	"flymail-core/types"
)

func TestMessageIDs(t *testing.T) {
	cases := map[string][]string{
		"":                                   nil,
		"<a@x>":                              {"a@x"},
		"<a@x> <b@y>\r\n <c@z>":              {"a@x", "b@y", "c@z"},
		"<a@x>,<b@y>":                        {"a@x", "b@y"},
		"a@x b@y":                            {"a@x", "b@y"},
		"a@x, b@y":                           {"a@x", "b@y"},
		"Your message of Monday <a@x>":       {"a@x"}, // 旧 Outlook 会在 In-Reply-To 里夹说明文字
		"<>":                                 nil,
		"<a@x> trailing text without angles": {"a@x"},
		"no id here":                         nil,
	}
	for in, want := range cases {
		if got := MessageIDs(in); !reflect.DeepEqual(got, want) {
			t.Errorf("MessageIDs(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseBodyFillsThreadHeaders(t *testing.T) {
	raw := strings.Join([]string{
		"From: a@x",
		"To: b@y",
		"Subject: Re: hi",
		"Message-ID: <c@z>",
		"In-Reply-To: <b@y>",
		"References: <a@x>",
		" <b@y>",
		"Content-Type: text/plain",
		"",
		"body",
	}, "\r\n")
	email := &types.ParsedEmail{}
	if err := ParseBody(strings.NewReader(raw), email, false); err != nil {
		t.Fatal(err)
	}
	if email.InReplyTo != "b@y" || email.References != "a@x b@y" {
		t.Errorf("thread headers = %q / %q", email.InReplyTo, email.References)
	}
	// 已由 ENVELOPE 填过的 In-Reply-To 不覆盖
	email = &types.ParsedEmail{InReplyTo: "env@id"}
	_ = ParseBody(strings.NewReader(raw), email, true)
	if email.InReplyTo != "env@id" {
		t.Errorf("InReplyTo overwritten: %q", email.InReplyTo)
	}
}
