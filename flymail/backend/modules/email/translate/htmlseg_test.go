package translate

import (
	"strings"
	"testing"
)

// 回填后结构必须一个标签都不差——这是"保留排版"这件事的全部内容。
func TestParseApplyKeepsStructure(t *testing.T) {
	const in = `<div class="x"><p>Hello <b>world</b></p><img src="cid:a@b"><a href="https://e.com">link</a></div>`
	doc, segs, err := parseDocument(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 3 {
		t.Fatalf("应当抽出 3 段文本（Hello / world / link），得到 %d", len(segs))
	}
	for i, s := range segs {
		s.out[0] = []string{"你好", "世界", "链接"}[i]
		s.apply()
	}
	out, err := renderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`class="x"`, `<b>`, `src="cid:a@b"`, `href="https://e.com"`, "你好", "世界", "链接"} {
		if !strings.Contains(out, want) {
			t.Errorf("译文丢了 %q：%s", want, out)
		}
	}
	if strings.Contains(out, "Hello") || strings.Contains(out, "world") {
		t.Errorf("原文没被替换掉：%s", out)
	}
}

// 文本节点首尾的空白是排版的一部分，模型会把它吃掉，所以我们自己留着。
func TestSegmentKeepsSurroundingSpace(t *testing.T) {
	const in = `<p>word <b>bold</b> tail</p>`
	doc, segs, err := parseDocument(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 3 {
		t.Fatalf("段数 = %d", len(segs))
	}
	for i, s := range segs {
		s.out[0] = []string{"词", "粗", "尾"}[i]
		s.apply()
	}
	out, _ := renderDocument(doc)
	if !strings.Contains(out, "词 <b>粗</b> 尾") {
		t.Errorf("首尾空白丢了，词会黏在一起：%s", out)
	}
}

// 模型返回的内容只会落进文本节点，渲染时一律转义——这是这套方案的安全底线。
func TestApplyEscapesModelOutput(t *testing.T) {
	doc, segs, err := parseDocument(`<p>hi</p>`)
	if err != nil {
		t.Fatal(err)
	}
	segs[0].out[0] = `<script>alert(1)</script>&<img onerror=x>`
	segs[0].apply()
	out, _ := renderDocument(doc)
	if strings.Contains(out, "<script") || strings.Contains(out, "<img") {
		t.Fatalf("模型输出被当成标记渲染了，等于让外部服务往正文里注入：%s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("应当转义成可见字符：%s", out)
	}
}

func TestSkipsNonTranslatableNodes(t *testing.T) {
	const in = `<div><style>.a{color:red}</style><script>var x=1</script><!-- 注释 -->
		<span>  </span><span>123</span><span>© 2026</span><span>Hello</span></div>`
	_, segs, err := parseDocument(in)
	if err != nil {
		t.Fatal(err)
	}
	// 只有 "Hello" 值得翻：样式/脚本是给机器读的，空白与纯数字节点在营销邮件里
	// 能占一半以上，送过去纯属烧 token。"© 2026" 没有字母，同样跳过。
	if len(segs) != 1 || segs[0].parts[0] != "Hello" {
		var got []string
		for _, s := range segs {
			got = append(got, s.parts...)
		}
		t.Fatalf("抽出的片段 = %q，只应有 Hello", got)
	}
}

// <pre> 必须参与翻译：很多客户端就是把纯文本邮件包成 <pre> 发出来的。
func TestPreIsTranslated(t *testing.T) {
	_, segs, err := parseDocument("<pre>Dear customer, your invoice is ready.</pre>")
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 {
		t.Fatalf("<pre> 里的正文应当参与翻译，段数 = %d", len(segs))
	}
}

func TestSplitLongText(t *testing.T) {
	// 超长段落要在句末断开，而不是整段压进一次请求撞上 max_tokens
	long := strings.Repeat("This is a sentence. ", 200) // 4000 字符
	parts := splitText(long, maxPartRunes)
	if len(parts) < 3 {
		t.Fatalf("应当拆成多段，得到 %d", len(parts))
	}
	for _, p := range parts {
		if n := len([]rune(p)); n > maxPartRunes {
			t.Errorf("片段超长：%d", n)
		}
	}
	if strings.Join(parts, "") != long {
		t.Error("拆开再拼回来必须与原文逐字相同")
	}
	// 断在句号之后，不是硬切在半个词上
	if !strings.HasSuffix(strings.TrimRight(parts[0], " "), ".") {
		t.Errorf("首段没断在句末：...%q", parts[0][len(parts[0])-20:])
	}
}

func TestSplitNoPunctuationFallsBackToHardCut(t *testing.T) {
	long := strings.Repeat("あ", 3000)
	parts := splitText(long, maxPartRunes)
	for _, p := range parts {
		if n := len([]rune(p)); n > maxPartRunes {
			t.Errorf("硬切也必须守住上限，得到 %d", n)
		}
	}
	if strings.Join(parts, "") != long {
		t.Error("拼回来必须与原文相同")
	}
}

// 漏翻的片段退回原文：半句原文半句译文仍然读得懂，空白会让人以为内容丢了。
func TestApplyFallsBackToOriginal(t *testing.T) {
	doc, segs, err := parseDocument("<p>" + strings.Repeat("Sentence one. ", 200) + "</p>")
	if err != nil {
		t.Fatal(err)
	}
	s := segs[0]
	if len(s.parts) < 2 {
		t.Fatal("这段应当被拆成多个片段")
	}
	s.out[0] = "第一句。"
	s.apply()
	out, _ := renderDocument(doc)
	if !strings.Contains(out, "第一句。") {
		t.Error("已翻的部分丢了")
	}
	if !strings.Contains(out, s.parts[1]) {
		t.Error("没翻出来的片段应当退回原文，而不是留空")
	}
}
