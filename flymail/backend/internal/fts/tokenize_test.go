package fts

import "testing"

func TestTokenize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"hello world", "hello world"},
		{"发票", " 发票 "},
		{"票", " 票 "},
		{"张三给李四", " 张三 三给 给李 李四 "},
		{"Re: 发票 2024", "Re:  发票  2024"},
		{"合同signed发票", " 合同 signed 发票 "},
		{"こんにちは", " こん んに にち ちは "},
		{"한국어", " 한국 국어 "},
	}
	for _, c := range cases {
		if got := Tokenize(c.in); got != c.want {
			t.Errorf("Tokenize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSegments(t *testing.T) {
	got := Segments("发票2024合同")
	want := []Segment{{"发票", true}, {"2024", false}, {"合同", true}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("seg %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestMatchExprPhrase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello world", `"hello world"`},
		{"报销 发票", `"报销" "发票"`},
		{"invoice 发票", `"invoice" "发票"`},
	}
	for _, c := range cases {
		if got := MatchExpr(c.in, true); got != c.want {
			t.Errorf("MatchExpr(%q, phrase) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMatchExpr(t *testing.T) {
	cases := []struct{ in, want string }{
		{"发票", `"发票"`},
		{"李四发", `"李四 四发"`},
		{"票", `"票"`},
		{"inv", `"inv"*`},
		{"zhang@example.com", `"zhang"* "example"* "com"*`},
		{"发票2024", `"发票" "2024"*`},
		// FTS5 语法字符被清掉，不会注入语法
		{`a" OR "b`, `"a"* "OR"* "b"*`},
		{"(NOT)", `"NOT"*`},
		{"...", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := MatchExpr(c.in, false); got != c.want {
			t.Errorf("MatchExpr(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
