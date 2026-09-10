package send

import (
	"strings"
	"testing"
)

// TestCleanMsgIDListDoesNotMaterializeAllTokens References 最多只保留 max 个 id，
// 分配量就该跟着 max 走。strings.FieldsFunc 会先把整串的所有分段物化成 []string
// （每段一个 16 字节 header），一个几 MB 的 References 在够到 max 之前就先分配几十 MB——
// 与 data: 图那条路上的空白密度放大是同一类脱钩。
func TestCleanMsgIDListDoesNotMaterializeAllTokens(t *testing.T) {
	raw := strings.Repeat("<a@b.com> ", 1<<19) // 5 MiB，约 52 万个 id，远超 max
	var (
		out string
		ok  bool
	)
	grew := allocOf(func() { out, ok = cleanMsgIDList(raw, 255) })

	if !ok || strings.Count(out, "@") != 255 {
		t.Fatalf("应保留 255 个 id，实际 ok=%v 个数=%d", ok, strings.Count(out, "@"))
	}
	// 保留 255 个 id 的输出不过几 KB，连输入的一份拷贝都不该要
	if limit := uint64(len(raw)); grew > limit {
		t.Errorf("分配了 %d 字节（输入才 %d），说明先把全部分段物化了一遍", grew, limit)
	}
}

// TestCleanMsgIDListSemanticsUnchanged 切分改手工实现后，分隔与校验语义必须不变。
func TestCleanMsgIDListSemanticsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
		ok              bool
	}{
		{"逗号与空白混排", " <a@x.com>,\t<b@x.com>  <c@x.com> ", "a@x.com> <b@x.com> <c@x.com", true},
		{"无尖括号", "a@x.com b@x.com", "a@x.com> <b@x.com", true},
		{"非法 id 静默丢弃", "<a@x.com> <bad id> <b@x.com>", "a@x.com> <b@x.com", true},
		{"含 CRLF 整条丢弃", "<a@x.com>\r\nBcc: victim@evil.com", "", false},
		{"全都非法", "<> <(x)>", "", false},
		{"空值", "   ", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cleanMsgIDList(tc.raw, 255)
			if ok != tc.ok || got != tc.want {
				t.Errorf("cleanMsgIDList(%q) = %q,%v，期望 %q,%v", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}
