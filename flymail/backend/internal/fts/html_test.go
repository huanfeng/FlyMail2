package fts

import "testing"

func TestStripHTML(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"<p>三月<b>发票</b>报销</p>", "三月 发票 报销"},
		{"<td>发票</td><td>报销</td>", "发票 报销"},
		{"<style>p{color:red}</style><script>alert(1)</script>hello &amp; world&nbsp;!", "hello & world !"},
		{"<div>a\n\n  b</div>", "a b"},
		{"plain text", "plain text"},
	}
	for _, c := range cases {
		if got := StripHTML(c.in); got != c.want {
			t.Errorf("StripHTML(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
