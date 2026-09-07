package fts

import (
	"testing"
	"time"
)

func TestParseFreeText(t *testing.T) {
	q := Parse(`发票 invoice "hello world"`)
	if len(q.Terms) != 3 || q.Terms[2].Text != "hello world" || !q.Terms[2].Phrase || q.Terms[0].Phrase {
		t.Fatalf("terms = %+v", q.Terms)
	}
	if q.HasStructured() {
		t.Fatal("should have no structured condition")
	}
	want := `("发票") AND ("invoice"*) AND ("hello world")`
	if got := q.Match(); got != want {
		t.Errorf("Match = %q, want %q", got, want)
	}
}

func TestParseQualifiers(t *testing.T) {
	q := Parse(`from:zhang subject:"报销 发票" has:attachment is:unread is:starred before:2024-06-01 after:2024/1/15 in:inbox account:me@x.com to:李四`)
	if len(q.From) != 1 || q.From[0].Text != "zhang" {
		t.Errorf("from = %+v", q.From)
	}
	if len(q.Subject) != 1 || q.Subject[0].Text != "报销 发票" {
		t.Errorf("subject = %+v", q.Subject)
	}
	if len(q.To) != 1 || q.To[0].Text != "李四" {
		t.Errorf("to = %+v", q.To)
	}
	if q.HasAttachment == nil || !*q.HasAttachment {
		t.Error("has:attachment")
	}
	if q.Seen == nil || *q.Seen {
		t.Error("is:unread")
	}
	if q.Flagged == nil || !*q.Flagged {
		t.Error("is:starred")
	}
	if q.Before == nil || !q.Before.Equal(time.Date(2024, 6, 1, 0, 0, 0, 0, time.Local)) {
		t.Errorf("before = %v", q.Before)
	}
	if q.After == nil || !q.After.Equal(time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local)) {
		t.Errorf("after = %v", q.After)
	}
	if q.Folder != "inbox" || q.Account != "me@x.com" {
		t.Errorf("folder=%q account=%q", q.Folder, q.Account)
	}
	if len(q.Terms) != 0 {
		t.Errorf("unexpected free terms %+v", q.Terms)
	}
	if !q.Subject[0].Phrase || q.From[0].Phrase {
		t.Errorf("带引号的限定符取值应保留短语语义: subject=%+v from=%+v", q.Subject[0], q.From[0])
	}
	want := `{from_name from_addr}: ("zhang"*) AND recipients: ("李四") AND subject: ("报销" "发票")`
	if got := q.Match(); got != want {
		t.Errorf("Match = %q, want %q", got, want)
	}
}

func TestParseInvalidFallsBackToText(t *testing.T) {
	// 非法取值退化为自由词；空取值丢弃；未知的 key:value 原样当文本
	q := Parse(`has:banana is: before:notadate Re:test http://x`)
	if q.HasAttachment != nil || q.Seen != nil || q.Before != nil {
		t.Fatalf("structured should be empty: %+v", q)
	}
	got := []string{}
	for _, tm := range q.Terms {
		got = append(got, tm.Text)
	}
	want := []string{"has:banana", "before:notadate", "Re:test", "http://x"}
	if len(got) != len(want) {
		t.Fatalf("terms = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("term %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseQualifierPhrase(t *testing.T) {
	q := Parse(`subject:"hello world" from:"Alice Wang"`)
	want := `{from_name from_addr}: ("Alice Wang") AND subject: ("hello world")`
	if got := q.Match(); got != want {
		t.Errorf("Match = %q, want %q", got, want)
	}
}

func TestParseEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "...", `""`} {
		if q := Parse(in); !q.Empty() {
			t.Errorf("Parse(%q) should be empty, got %+v", in, q)
		}
	}
	// 只有结构化条件也算非空
	if Parse("is:unread").Empty() {
		t.Error("is:unread should not be empty")
	}
}

func TestParseCaseAndFullwidth(t *testing.T) {
	q := Parse("FROM:Zhang　IS:Unread “报销 发票”")
	if len(q.From) != 1 || q.Seen == nil || *q.Seen {
		t.Fatalf("%+v", q)
	}
	if len(q.Terms) != 1 || q.Terms[0].Text != "报销 发票" || !q.Terms[0].Phrase {
		t.Fatalf("terms = %+v", q.Terms)
	}
}
