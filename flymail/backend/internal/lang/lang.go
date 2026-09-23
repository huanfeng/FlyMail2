// Package lang 做**本地**语言识别，并持有可选的翻译目标语言清单。
//
// ── 为什么不把识别也交给 AI ────────────────────────────────────────────────
//
// 识别的唯一用途是回答"这封信要不要翻译"。交给 AI 的话，就得先花一次调用
// （和一次等待）才知道这次调用本来不必发生——而绝大多数邮件是用户自己
// 那门语言写的，也就是绝大多数调用都白花。
//
// 本地识别便宜到可以对每封打开的邮件都跑一遍：脚本判定就是数字符属于哪个
// Unicode 区间，拉丁字母内部再用高频虚词表分辨。
//
// ── 精度到哪一步为止 ──────────────────────────────────────────────────────
//
// 它只需要在"和目标语言是同一门"这件事上别说错。判不准时返回空串，
// 界面上就当没识别过（照常给翻译按钮），而不是硬猜一个语言把按钮灰掉——
// 猜错的代价是用户想翻译却翻译不了，比多点一次按钮严重得多。
package lang

import (
	"sort"
	"strings"
	"unicode"
)

// Language 是一个可选的翻译目标语言。
type Language struct {
	Code string `json:"code"`
	// Name 是给模型看的英文名（提示词里用）。
	Name string `json:"name"`
	// Native 是给人看的自称名。用自称而不是翻译过的名字，是因为下拉框里
	// 「日本語」对任何人都比「Japanese」的本地化译名更好认，也省掉一整套 i18n。
	Native string `json:"native"`
}

// Supported 是可选的翻译目标语言。
//
// 这个清单是**目标**语言，不是能识别的语言：能翻成什么由模型决定，
// 这里只是把常用的十来种摆出来，避免让用户自己去背语言代码。
var Supported = []Language{
	{Code: "zh", Name: "Simplified Chinese", Native: "简体中文"},
	{Code: "zh-Hant", Name: "Traditional Chinese", Native: "繁體中文"},
	{Code: "en", Name: "English", Native: "English"},
	{Code: "ja", Name: "Japanese", Native: "日本語"},
	{Code: "ko", Name: "Korean", Native: "한국어"},
	{Code: "fr", Name: "French", Native: "Français"},
	{Code: "de", Name: "German", Native: "Deutsch"},
	{Code: "es", Name: "Spanish", Native: "Español"},
	{Code: "pt", Name: "Portuguese", Native: "Português"},
	{Code: "it", Name: "Italian", Native: "Italiano"},
	{Code: "ru", Name: "Russian", Native: "Русский"},
	{Code: "ar", Name: "Arabic", Native: "العربية"},
}

// DefaultTarget 是未配置时的默认目标语言。
const DefaultTarget = "zh"

// IsSupported 报告代码是否在 Supported 里。
func IsSupported(code string) bool {
	for _, l := range Supported {
		if l.Code == code {
			return true
		}
	}
	return false
}

// NameOf 返回给模型看的英文语言名；未知代码原样返回，
// 让"清单里暂时没有但模型认得"的代码也能用。
func NameOf(code string) string {
	for _, l := range Supported {
		if l.Code == code {
			return l.Name
		}
	}
	return code
}

// SameLanguage 报告识别结果是否就是目标语言。
//
// 简繁是特例：识别侧分得出简繁，但把简体的信"翻成简体中文"确实没意义，
// 而把简体翻成繁体是有意义的，所以这里按完整代码比，不做 zh 前缀合并。
func SameLanguage(detected, target string) bool {
	return detected != "" && detected == target
}

// maxScan 限制参与识别的字符数。
//
// 识别靠的是分布，几百字和几万字给出的结论一样；而营销邮件动辄几十万字符，
// 每次打开都全量扫一遍纯属浪费。取正文开头足够——语言不会在信中途换掉。
const maxScan = 4000

// Detect 识别文本的语言，返回 Supported 中的代码，判不准时返回空串。
func Detect(text string) string {
	runes := []rune(text)
	if len(runes) > maxScan {
		runes = runes[:maxScan]
	}

	var han, kana, hangul, cyrillic, arabic, latin int
	for _, r := range runes {
		switch {
		case unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
			kana++
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case unicode.Is(unicode.Arabic, r):
			arabic++
		case r < unicode.MaxASCII && unicode.IsLetter(r), unicode.Is(unicode.Latin, r):
			latin++
		}
	}

	// 假名是日文的铁证：日文正文里汉字可以很多，但只要有假名就不是中文。
	// 阈值不设 0 是因为中文邮件里偶尔混着「の」这种装饰用字。
	if kana >= 3 || (kana > 0 && kana*20 >= han) {
		return "ja"
	}
	if hangul > 0 && hangul*4 >= han {
		return "ko"
	}
	if han > 0 && han*2 >= latin {
		return chineseVariant(runes)
	}
	if cyrillic > 0 && cyrillic*2 >= latin {
		return "ru"
	}
	if arabic > 0 && arabic*2 >= latin {
		return "ar"
	}
	if latin > 0 {
		return latinLanguage(string(runes))
	}
	return ""
}

// tradOnly / simpOnly 是简繁各自独有的高频字。
//
// 只收"另一侧写法不同"的字：像「中」「文」这种两边同形的字出现再多也不带信息。
// 数量不必多，十几个高频字在一封正常长度的中文信里几乎必然出现若干个。
var (
	tradOnly = []rune("個們這時國學會實點當後經發對說麼樣種體聽讀寫產業務開關門問題還應該從專車輛儘為無與於")
	simpOnly = []rune("个们这时国学会实点当后经发对说么样种体听读写产业务开关门问题还应该从专车辆尽为无与于")
)

// chineseVariant 在简体与繁体之间做选择。
//
// 判不出（两边都没命中）时返回 "zh"：中文邮件里简体是压倒性多数，
// 而这个结论只用于"要不要提示已是目标语言"，猜偏的代价很小。
func chineseVariant(runes []rune) string {
	inTrad := make(map[rune]bool, len(tradOnly))
	for _, r := range tradOnly {
		inTrad[r] = true
	}
	inSimp := make(map[rune]bool, len(simpOnly))
	for _, r := range simpOnly {
		inSimp[r] = true
	}
	var trad, simp int
	for _, r := range runes {
		if inTrad[r] {
			trad++
		} else if inSimp[r] {
			simp++
		}
	}
	if trad > simp {
		return "zh-Hant"
	}
	return "zh"
}

// stopwords 是各拉丁语言的高频虚词。
//
// 虚词而不是实词：实词跟着话题走（一封讲发票的英文信里可能一个常用实词都没有），
// 虚词跟着语法走，任何一段成句的文字都会大量出现。
//
// 词表之间刻意留了区分度高的条目（de/der/il/é 各家不同），但西班牙语与
// 葡萄牙语本就高度重叠，判不准时由下面的"领先幅度"闸门统一退回空串。
var stopwords = map[string][]string{
	"en": {"the", "and", "is", "are", "of", "to", "in", "for", "you", "that", "with", "this", "have", "from", "will", "your", "was", "has"},
	"fr": {"le", "la", "les", "des", "une", "est", "pour", "vous", "avec", "dans", "que", "qui", "sur", "nous", "votre", "être", "cette", "aux"},
	"de": {"der", "die", "das", "und", "ist", "nicht", "mit", "für", "sie", "ein", "eine", "den", "dem", "auf", "wir", "haben", "ihre", "sich"},
	"es": {"el", "los", "las", "que", "para", "con", "una", "por", "más", "su", "está", "como", "pero", "este", "son", "nuestro"},
	"pt": {"os", "as", "que", "para", "com", "uma", "por", "não", "se", "do", "da", "você", "está", "são", "mais", "nosso"},
	"it": {"il", "la", "le", "di", "che", "per", "con", "una", "non", "del", "della", "sono", "questo", "anche", "come", "nostro"},
}

// latinLanguage 在拉丁字母的几门语言之间做选择。
func latinLanguage(text string) string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	if len(words) == 0 {
		return ""
	}
	freq := make(map[string]int, len(words))
	for _, w := range words {
		freq[w]++
	}

	scores := make(map[string]int, len(stopwords))
	for code, list := range stopwords {
		for _, w := range list {
			scores[code] += freq[w]
		}
	}

	codes := make([]string, 0, len(scores))
	for code := range scores {
		codes = append(codes, code)
	}
	// 分数相同时按代码排序，让结果稳定——否则同一封信每次打开可能给出不同语言。
	sort.Strings(codes)
	best, second := "", 0
	bestScore := 0
	for _, code := range codes {
		if s := scores[code]; s > bestScore {
			second = bestScore
			best, bestScore = code, s
		} else if s > second {
			second = s
		}
	}

	// 两道闸门，都是为了"宁可不报，不可报错"：
	//   命中太少 —— 三五个词的短信息（"Thanks!"）本就不够判
	//   领先不够 —— 西/葡这种高度重叠的语言对，平局时谁也不选
	if bestScore < 3 || bestScore < second*2 {
		return ""
	}
	return best
}
