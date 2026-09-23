package translate

import (
	"strings"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	units := []unit{
		{id: 1, text: "Hello"},
		{id: 2, text: "World"},
	}
	req := encodeChunk(units)
	if !strings.Contains(req, "⟦1⟧Hello") || !strings.Contains(req, "⟦2⟧World") {
		t.Fatalf("编码结果不对：%s", req)
	}

	got := decodeChunk("⟦1⟧你好\n⟦2⟧世界")
	if got[1] != "你好" || got[2] != "世界" {
		t.Errorf("解码 = %v", got)
	}
}

// 模型爱在前面加一句寒暄。那段话在第一个标记之前，必须被丢掉而不是串进第一段。
func TestDecodeIgnoresPreamble(t *testing.T) {
	got := decodeChunk("好的，以下是译文：\n\n⟦1⟧你好\n⟦2⟧世界\n\n希望对你有帮助！")
	if got[1] != "你好" {
		t.Errorf("第 1 段 = %q，寒暄没被丢掉", got[1])
	}
	// 结尾的客套话会黏在最后一段上——这是这套宽松解析的已知代价，
	// 但它只影响最后一段的观感，比整批解析失败（那一段正文全是原文）划算得多。
	if !strings.HasPrefix(got[2], "世界") {
		t.Errorf("第 2 段 = %q", got[2])
	}
}

// 多行译文要完整收回来：按标记切分而不是按行，正是为了这个。
func TestDecodeKeepsMultilineText(t *testing.T) {
	got := decodeChunk("⟦1⟧第一行\n第二行\n第三行\n⟦2⟧下一段")
	if got[1] != "第一行\n第二行\n第三行" {
		t.Errorf("多行译文被截断了：%q", got[1])
	}
}

// 漏掉的编号不能补空串——回填那侧靠"没有这个编号"来决定退回原文。
func TestDecodeSkipsEmptyAndMissing(t *testing.T) {
	got := decodeChunk("⟦1⟧你好\n⟦2⟧\n⟦4⟧第四段")
	if _, ok := got[2]; ok {
		t.Error("空译文不该入表，否则会把一段正文换成空白")
	}
	if _, ok := got[3]; ok {
		t.Error("没回的编号不该凭空出现")
	}
	if got[4] != "第四段" {
		t.Errorf("第 4 段 = %q", got[4])
	}
}

// 模型偶尔把编号标记抄进译文里，抄进去的那份要擦掉，否则会显示在正文上。
func TestDecodeStripsNestedMarks(t *testing.T) {
	got := decodeChunk("⟦1⟧你好⟦1⟧")
	if strings.Contains(got[1], "⟦") {
		t.Errorf("译文里残留了编号标记：%q", got[1])
	}
}

func TestDecodeNoMarksReturnsNil(t *testing.T) {
	// 模型完全没按格式回（比如回了一句"我无法翻译"）：整批算没翻出来，
	// 回填退回原文，而不是把这段回复当成译文贴到正文里。
	if got := decodeChunk("抱歉，我无法完成这个请求。"); got != nil {
		t.Errorf("不含标记时应返回 nil，得到 %v", got)
	}
}

func TestBuildUnitsGlobalIDs(t *testing.T) {
	segs := []*segment{
		{parts: []string{"a", "b"}, out: make([]string, 2)},
		{parts: []string{"c"}, out: make([]string, 1)},
	}
	units := buildUnits(segs)
	if len(units) != 3 {
		t.Fatalf("单元数 = %d", len(units))
	}
	for i, u := range units {
		if u.id != i+1 {
			t.Errorf("第 %d 个单元编号 = %d，应当是全局连续编号", i, u.id)
		}
	}
	if units[1].seg != segs[0] || units[1].part != 1 {
		t.Error("单元没指回正确的片段位置")
	}
}

func TestChunkUnitsRespectsBudget(t *testing.T) {
	units := []unit{
		{id: 1, text: strings.Repeat("あ", 100)},
		{id: 2, text: strings.Repeat("い", 100)},
		{id: 3, text: strings.Repeat("う", 100)},
	}
	chunks := chunkUnits(units, 250)
	if len(chunks) != 2 {
		t.Fatalf("批数 = %d，250 的预算装不下第三段", len(chunks))
	}
	if len(chunks[0]) != 2 || len(chunks[1]) != 1 {
		t.Errorf("分批 = %d + %d", len(chunks[0]), len(chunks[1]))
	}
}

func TestChunkUnitsOversizedGetsOwnChunk(t *testing.T) {
	units := []unit{
		{id: 1, text: "short"},
		{id: 2, text: strings.Repeat("x", 5000)},
		{id: 3, text: "short"},
	}
	chunks := chunkUnits(units, 1000)
	if len(chunks) != 3 {
		t.Fatalf("超预算的单元应当独占一批，得到 %d 批", len(chunks))
	}
}
