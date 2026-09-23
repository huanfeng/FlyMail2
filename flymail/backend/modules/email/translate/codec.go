package translate

import (
	"regexp"
	"strconv"
	"strings"
)

// ── 片段怎么编号送出去、怎么收回来 ────────────────────────────────────────
//
// 一次请求里有几十个互不相干的片段，模型必须把译文一一对上号地还回来。
// 试过的两种协议里：
//
//	JSON 数组 —— 译文里的引号、换行、反斜杠都要转义，模型偶尔转错一个，
//	  整批就解析失败（而"整批失败"意味着这一段正文全是原文）。
//	编号标记 —— 译文原样放在标记后面，没有任何转义规则可犯错；解析时
//	  按标记切分，某一段坏了也只坏那一段。
//
// 所以用后者。标记选 ⟦N⟧（U+27E6/27E7）：模型能稳定复现，而正常邮件正文
// 里不会出现这两个字符——真出现了也只是让那一段多切一刀，不会串号。
const (
	markOpen  = "⟦"
	markClose = "⟧"
)

// reMark 匹配一个编号标记。
var reMark = regexp.MustCompile(`⟦(\d+)⟧`)

// encodeChunk 把一批片段编成带编号的请求文本。
//
// 编号是**全局下标**而不是批内序号：模型偶尔会把两批的内容混起来回，
// 全局编号让这种情况变成"某几段没翻"，而不是悄悄串到别的段落上去。
func encodeChunk(units []unit) string {
	var sb strings.Builder
	for _, u := range units {
		sb.WriteString(markOpen)
		sb.WriteString(strconv.Itoa(u.id))
		sb.WriteString(markClose)
		sb.WriteString(u.text)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// decodeChunk 解析模型的回复，返回编号到译文的映射。
//
// 解析刻意宽松：只找标记，标记之间的一切都算译文（含换行）。模型在最前面
// 加一句"好的，以下是译文："也不影响——那段文字在第一个标记之前，会被丢掉。
func decodeChunk(raw string) map[int]string {
	locs := reMark.FindAllStringSubmatchIndex(raw, -1)
	if len(locs) == 0 {
		return nil
	}
	out := make(map[int]string, len(locs))
	for i, loc := range locs {
		id, err := strconv.Atoi(raw[loc[2]:loc[3]])
		if err != nil {
			continue
		}
		end := len(raw)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		text := strings.TrimSpace(raw[loc[1]:end])
		// 空译文不入表：让回填那侧走"没翻出来 → 保留原文"的路径，
		// 而不是把一段正文换成空白。
		if text == "" {
			continue
		}
		// 模型偶尔会把自己的编号标记也抄进译文里（"⟦3⟧ 你好"），
		// 抄进去的那份会被上面的切分当成新的一段，这里再擦一遍尾巴。
		out[id] = reMark.ReplaceAllString(text, "")
	}
	return out
}

// unit 是一个可独立寻址的待译片段：某个 segment 的第几个 part。
type unit struct {
	id   int
	seg  *segment
	part int
	text string
}

// buildUnits 把片段摊平成带全局编号的待译单元。
func buildUnits(segs []*segment) []unit {
	var units []unit
	for _, s := range segs {
		for i, p := range s.parts {
			units = append(units, unit{id: len(units) + 1, seg: s, part: i, text: p})
		}
	}
	return units
}

// chunkUnits 按字符预算把待译单元分批。
//
// 单个单元超预算时自己独占一批——它已经被 splitText 限制在 maxPartRunes 内，
// 这里只是不让它跟别人挤。
func chunkUnits(units []unit, budget int) [][]unit {
	var chunks [][]unit
	var cur []unit
	curLen := 0
	for _, u := range units {
		n := len([]rune(u.text))
		if len(cur) > 0 && curLen+n > budget {
			chunks = append(chunks, cur)
			cur, curLen = nil, 0
		}
		cur = append(cur, u)
		curLen += n
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}
