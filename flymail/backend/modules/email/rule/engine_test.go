package rule

import (
	"strings"
	"testing"
)

func compileOK(t *testing.T, r Rule) *Compiled {
	t.Helper()
	c, err := Compile(r)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

func TestCompileValidation(t *testing.T) {
	base := Rule{Name: "r", Match: MatchAll, Conditions: `[{"field":"from","op":"contains","value":"x"}]`, Actions: `[{"type":"mark_read"}]`}
	compileOK(t, base)
	bad := []Rule{
		{Name: " ", Match: MatchAll, Conditions: base.Conditions, Actions: base.Actions},
		{Name: "r", Match: "some", Conditions: base.Conditions, Actions: base.Actions},
		{Name: "r", Match: MatchAll, Conditions: `[]`, Actions: base.Actions},
		{Name: "r", Match: MatchAll, Conditions: base.Conditions, Actions: `[]`},
		{Name: "r", Match: MatchAll, Conditions: `[{"field":"header","op":"contains","value":"x"}]`, Actions: base.Actions},
		{Name: "r", Match: MatchAll, Conditions: `[{"field":"from","op":"like","value":"x"}]`, Actions: base.Actions},
		{Name: "r", Match: MatchAll, Conditions: `[{"field":"from","op":"contains","value":"  "}]`, Actions: base.Actions},
		{Name: "r", Match: MatchAll, Conditions: `[{"field":"subject","op":"regex","value":"("}]`, Actions: base.Actions},
		{Name: "r", Match: MatchAll, Conditions: `[{"field":"has_attachment","op":"contains","value":"true"}]`, Actions: base.Actions},
		{Name: "r", Match: MatchAll, Conditions: base.Conditions, Actions: `[{"type":"move","value":""}]`},
		{Name: "r", Match: MatchAll, Conditions: base.Conditions, Actions: `[{"type":"forward","value":"a@b"}]`},
		{Name: "r", Match: MatchAll, Conditions: `not json`, Actions: base.Actions},
	}
	for i, r := range bad {
		if _, err := Compile(r); err == nil {
			t.Errorf("case %d should fail: %+v", i, r)
		}
	}
}

func TestEvaluateOpsAndMatchMode(t *testing.T) {
	v := &MessageView{
		From: "Alice Wang <alice@corp.cn>", FromAddr: "alice@corp.cn",
		To: "me@work.com, Bob <bob@x.io>", ToAddrs: "me@work.com, bob@x.io", Cc: "",
		Subject: "Re: 三月发票报销", Body: "请查收附件里的发票", BodyKnown: true,
		Attachments: []string{"invoice-2026-03.pdf"}, HasAttachment: true,
	}
	cases := []struct {
		conds string
		match string
		want  bool
	}{
		{`[{"field":"from","op":"contains","value":"ALICE"}]`, MatchAll, true},       // 大小写不敏感，显示名也算
		{`[{"field":"from","op":"ends_with","value":"@corp.cn"}]`, MatchAll, true},   // 纯地址也参与：用户写域名后缀能命中
		{`[{"field":"from","op":"equals","value":"alice@corp.cn"}]`, MatchAll, true}, // equals 对纯地址成立
		{`[{"field":"from","op":"equals","value":"alice"}]`, MatchAll, false},
		{`[{"field":"from","op":"not_contains","value":"corp.cn"}]`, MatchAll, false},
		{`[{"field":"to","op":"ends_with","value":"@x.io"}]`, MatchAll, true},
		{`[{"field":"to","op":"contains","value":"bob@x.io"}]`, MatchAll, true},
		{`[{"field":"cc","op":"contains","value":"x"}]`, MatchAll, false},
		{`[{"field":"subject","op":"starts_with","value":"re:"}]`, MatchAll, true},
		{`[{"field":"subject","op":"regex","value":"^Re: .*发票"}]`, MatchAll, true},
		{`[{"field":"subject","op":"regex","value":"^re:"}]`, MatchAll, false}, // regex 区分大小写
		{`[{"field":"subject","op":"not_contains","value":"周报"}]`, MatchAll, true},
		{`[{"field":"body","op":"contains","value":"发票"}]`, MatchAll, true},
		{`[{"field":"attachment_name","op":"ends_with","value":".pdf"}]`, MatchAll, true},
		{`[{"field":"attachment_name","op":"contains","value":".zip"}]`, MatchAll, false},
		{`[{"field":"has_attachment","op":"equals","value":"true"}]`, MatchAll, true},
		{`[{"field":"has_attachment","op":"equals","value":"false"}]`, MatchAll, false},
		// all / any
		{`[{"field":"from","op":"contains","value":"alice"},{"field":"subject","op":"contains","value":"周报"}]`, MatchAll, false},
		{`[{"field":"from","op":"contains","value":"alice"},{"field":"subject","op":"contains","value":"周报"}]`, MatchAny, true},
		{`[{"field":"from","op":"contains","value":"nobody"},{"field":"subject","op":"contains","value":"周报"}]`, MatchAny, false},
	}
	for _, tc := range cases {
		c := compileOK(t, Rule{Name: "r", Match: tc.match, Conditions: tc.conds, Actions: `[{"type":"star"}]`})
		if got, _ := c.Evaluate(v); got != tc.want {
			t.Errorf("%s %s: got %v want %v", tc.match, tc.conds, got, tc.want)
		}
	}
}

func TestEvaluateUnknownBody(t *testing.T) {
	v := &MessageView{From: "a@x", Subject: "hi", BodyKnown: false, HasAttachment: false}
	// 正文条件在正文未知时不命中，且上报 unknown；not_contains 同样不命中（未知 ≠ 不含）
	for _, conds := range []string{
		`[{"field":"body","op":"contains","value":"x"}]`,
		`[{"field":"body","op":"not_contains","value":"x"}]`,
		`[{"field":"attachment_name","op":"contains","value":"x"}]`,
	} {
		c := compileOK(t, Rule{Name: "r", Match: MatchAll, Conditions: conds, Actions: `[{"type":"star"}]`})
		hit, unknown := c.Evaluate(v)
		if hit || !unknown {
			t.Errorf("%s: hit=%v unknown=%v", conds, hit, unknown)
		}
	}
	// any 模式下另一条命中仍算命中，但 unknown 仍上报
	c := compileOK(t, Rule{Name: "r", Match: MatchAny, Conditions: `[{"field":"body","op":"contains","value":"x"},{"field":"subject","op":"equals","value":"hi"}]`, Actions: `[{"type":"star"}]`})
	if hit, unknown := c.Evaluate(v); !hit || !unknown {
		t.Errorf("any with unknown: hit=%v unknown=%v", hit, unknown)
	}
	// has_attachment 列由正文落库回填：正文未到时既不能判「有」也不能判「无」，一律未知
	for _, val := range []string{"true", "false"} {
		c = compileOK(t, Rule{Name: "r", Match: MatchAll, Conditions: `[{"field":"has_attachment","op":"equals","value":"` + val + `"}]`, Actions: `[{"type":"star"}]`})
		if hit, unknown := c.Evaluate(v); hit || !unknown {
			t.Errorf("has_attachment=%s without body: hit=%v unknown=%v", val, hit, unknown)
		}
	}
	known := &MessageView{BodyKnown: true, HasAttachment: false}
	c = compileOK(t, Rule{Name: "r", Match: MatchAll, Conditions: `[{"field":"has_attachment","op":"equals","value":"false"}]`, Actions: `[{"type":"star"}]`})
	if hit, unknown := c.Evaluate(known); !hit || unknown {
		t.Errorf("has_attachment=false with body: hit=%v unknown=%v", hit, unknown)
	}
}

// TestAttachmentNotContains：「附件名不含 X」= 没有任何附件名含 X；任一含即为假，无附件时为真。
func TestAttachmentNotContains(t *testing.T) {
	c := compileOK(t, Rule{Name: "r", Match: MatchAll, Conditions: `[{"field":"attachment_name","op":"not_contains","value":"发票"}]`, Actions: `[{"type":"delete"}]`})
	cases := []struct {
		atts []string
		want bool
	}{
		{[]string{"发票.pdf", "合同.docx"}, false}, // 有一个含 → 假（旧实现这里恒真，会把邮件删掉）
		{[]string{"合同.docx", "报价.xlsx"}, true},
		{[]string{"三月发票.PDF"}, false}, // 大小写不敏感
		{nil, true},
	}
	for _, tc := range cases {
		if got, _ := c.Evaluate(&MessageView{BodyKnown: true, Attachments: tc.atts, HasAttachment: len(tc.atts) > 0}); got != tc.want {
			t.Errorf("atts=%v: got %v want %v", tc.atts, got, tc.want)
		}
	}
	// 两个 move 的规则拒绝编译
	if _, err := Compile(Rule{Name: "r", Match: MatchAll, Conditions: `[{"field":"subject","op":"contains","value":"x"}]`, Actions: `[{"type":"move","value":"A"},{"type":"move","value":"B"}]`}); err == nil {
		t.Errorf("two moves in one rule must be rejected")
	}
}

func TestBlockedAndNormalize(t *testing.T) {
	norm := map[string]string{
		"  Spam@Example.COM ": "spam@example.com",
		"@spam.io":            "spam.io",
		"Spam.IO":             "spam.io",
		"":                    "",
		"no-at-no-dot":        "",
		"a@b@c":               "",
		"@":                   "",
		"user@":               "",
		"has space@x.com":     "",
	}
	for in, want := range norm {
		if got := NormalizePattern(in); got != want {
			t.Errorf("NormalizePattern(%q) = %q, want %q", in, got, want)
		}
	}
	patterns := []string{"spam@example.com", "spam.io"}
	cases := map[string]bool{
		"spam@example.com":     true,
		"SPAM@example.com":     true,
		"other@example.com":    false,
		"x@spam.io":            true,
		"x@mail.spam.io":       true, // 子域也算
		"x@notspam.io":         false,
		"spam.io@else.com":     false,
		"":                     false,
		strings.Repeat("a", 5): false,
	}
	for addr, want := range cases {
		if _, got := Blocked(addr, patterns); got != want {
			t.Errorf("Blocked(%q) = %v, want %v", addr, got, want)
		}
	}
}
