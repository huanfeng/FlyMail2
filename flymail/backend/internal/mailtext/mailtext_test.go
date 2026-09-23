package mailtext

import (
	"strings"
	"testing"
)

// 这一条是这个包存在的理由：fts.StripHTML 把所有空白（含换行）压成一个空格，
// 推送里就是一坨没有段落的墙。段落必须留住。
func TestParagraphsSurvive(t *testing.T) {
	got := Extract("", `<p>第一段</p><p>第二段</p>`).Text
	if got != "第一段\n第二段" {
		t.Errorf("段落丢了：%q", got)
	}
}

func TestBlockBoundariesBecomeNewlines(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"br":     {`a<br>b`, "a\nb"},
		"div":    {`<div>a</div><div>b</div>`, "a\nb"},
		"li":     {`<ul><li>a</li><li>b</li></ul>`, "a\nb"},
		"表格行":    {`<table><tr><td>a</td></tr><tr><td>b</td></tr></table>`, "a\nb"},
		"标题":     {`<h1>标题</h1>正文`, "标题\n正文"},
		"自闭合 br": {`a<br/>b`, "a\nb"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Extract("", tc.in).Text; got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// 同行相邻的单元格不能粘在一起：<td>发票</td><td>报销</td> 直接删标签会变成
// 「发票报销」，读起来像一个词。
func TestInlineCellsDoNotGlueTogether(t *testing.T) {
	got := Extract("", `<table><tr><td>发票</td><td>报销</td></tr></table>`).Text
	if strings.Contains(got, "发票报销") {
		t.Errorf("单元格粘连了：%q", got)
	}
}

// script/style 必须连内容一起丢。只剥标签的话，整坨 CSS 和 JS 源码会留在正文里——
// 营销邮件的 <style> 动辄几 KB，足以把真正的内容挤出长度上限。
func TestScriptAndStyleContentDropped(t *testing.T) {
	in := `<style>.a{color:red}</style><script>var x=1;</script><p>正文</p>`
	got := Extract("", in).Text
	if got != "正文" {
		t.Errorf("script/style 内容泄漏：%q", got)
	}
}

func TestEntitiesUnescaped(t *testing.T) {
	got := Extract("", `<p>a &amp; b &lt;c&gt; &nbsp;d</p>`).Text
	if got != "a & b <c> d" {
		t.Errorf("实体未还原：%q", got)
	}
}

// HTML 转出来的文本里动辄十几个连续空行（嵌套 table/div 每层都贡献一个），
// 留着就是大段空白，在卡片里尤其难看。
func TestBlankLinesCollapsed(t *testing.T) {
	got := Extract("", `<div><div><div><p>a</p></div></div></div><br><br><br><p>b</p>`).Text
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("连续空行未收敛：%q", got)
	}
	if !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Errorf("内容丢了：%q", got)
	}
}

func TestPlainTextPreferredOverHTML(t *testing.T) {
	r := Extract("我是纯文本", "<p>我是 HTML</p>")
	if r.Text != "我是纯文本" {
		t.Errorf("应优先用纯文本：%q", r.Text)
	}
}

func TestEmptyInputs(t *testing.T) {
	for name, tc := range map[string][2]string{
		"全空":    {"", ""},
		"纯空白":   {"   \n  ", ""},
		"纯空白+空": {"  ", "   "},
	} {
		t.Run(name, func(t *testing.T) {
			r := Extract(tc[0], tc[1])
			if r.Text != "" || r.Links != nil {
				t.Errorf("应为零值，got %+v", r)
			}
		})
	}
}

// ── 链接提取 ────────────────────────────────────────────────────────────────

func TestHrefExtracted(t *testing.T) {
	// 转成文本后 <a href="...">点这里</a> 只剩「点这里」，地址没了——
	// 所以必须在剥标签之前从 href 里取。
	r := Extract("", `<a href="https://example.com/a">点这里</a>`)
	if len(r.Links) != 1 || r.Links[0] != "https://example.com/a" {
		t.Fatalf("href 未提取：%v", r.Links)
	}
	if r.Text != "点这里" {
		t.Errorf("正文应保留链接文字：%q", r.Text)
	}
}

func TestHrefQuoteStyles(t *testing.T) {
	in := `<a href="https://a.com">1</a><a href='https://b.com'>2</a><a href=https://c.com>3</a>`
	got := Extract("", in).Links
	want := []string{"https://a.com", "https://b.com", "https://c.com"}
	if len(got) != 3 {
		t.Fatalf("三种引号写法应都识别，got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBareURLsInPlainText(t *testing.T) {
	r := Extract("详见 https://example.com/a 与 http://b.cn/x", "")
	if len(r.Links) != 2 {
		t.Fatalf("裸地址应被识别，got %v", r.Links)
	}
}

// 句末标点属于句子不属于地址。不剪掉的话点开就是 404。
func TestTrailingPunctuationTrimmed(t *testing.T) {
	for _, in := range []string{
		"详见 https://example.com/a。",
		"详见 https://example.com/a.",
		"详见 https://example.com/a,",
		"（详见 https://example.com/a）",
	} {
		r := Extract(in, "")
		if len(r.Links) != 1 || r.Links[0] != "https://example.com/a" {
			t.Errorf("%q → %v", in, r.Links)
		}
	}
}

// 发信人写什么 href 我们就读到什么。javascript: / data: 点了要么没反应要么有害，
// 绝不该出现在推送卡片的可点区域里。
func TestNonHTTPSchemesRejected(t *testing.T) {
	for _, bad := range []string{
		`<a href="javascript:alert(1)">x</a>`,
		`<a href="data:text/html;base64,PHNjcmlwdD4=">x</a>`,
		`<a href="file:///etc/passwd">x</a>`,
		`<a href="mailto:a@b.com">x</a>`,
		`<a href="/relative/path">x</a>`,
	} {
		if links := Extract("", bad).Links; links != nil {
			t.Errorf("%q 不该被放行：%v", bad, links)
		}
	}
}

// 带换行的 href 能在卡片里制造额外的行，把一条链接伪装成两行内容。
func TestURLsWithControlCharsRejected(t *testing.T) {
	if links := Extract("", "<a href=\"https://a.com/\nfake\">x</a>").Links; links != nil {
		t.Errorf("含换行的地址不该放行：%v", links)
	}
}

func TestOverlongURLRejected(t *testing.T) {
	long := "https://a.com/" + strings.Repeat("x", 400)
	if links := Extract(long, "").Links; links != nil {
		t.Errorf("超长追踪链接不该列出：%v", links)
	}
}

func TestLinksDedupedInOrder(t *testing.T) {
	in := `<a href="https://b.com">1</a><a href="https://a.com">2</a><a href="https://b.com">3</a>`
	got := Extract("", in).Links
	if len(got) != 2 || got[0] != "https://b.com" || got[1] != "https://a.com" {
		t.Errorf("应按出现顺序去重：%v", got)
	}
}

// 营销邮件每张图、每个图标都是一个链接，几十上百条全列出来就是刷屏。
func TestLinkCountCapped(t *testing.T) {
	var b strings.Builder
	for i := range 50 {
		b.WriteString(`<a href="https://example.com/`)
		b.WriteByte(byte('a' + i%26))
		b.WriteString(`x`)
		b.WriteString(strings.Repeat("y", i))
		b.WriteString(`">x</a>`)
	}
	if got := Extract("", b.String()).Links; len(got) != maxLinks {
		t.Errorf("应截到 %d 条，got %d", maxLinks, len(got))
	}
}

func TestHrefEntitiesUnescaped(t *testing.T) {
	// 邮件里 query 串的 & 常写成 &amp;
	r := Extract("", `<a href="https://a.com/?x=1&amp;y=2">x</a>`)
	if len(r.Links) != 1 || r.Links[0] != "https://a.com/?x=1&y=2" {
		t.Errorf("href 实体未还原：%v", r.Links)
	}
}
