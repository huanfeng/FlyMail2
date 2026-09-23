package lang

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"简体中文", "您好，附件是本月的对账单，请查收。如有问题请随时与我们联系。", "zh"},
		{"繁体中文", "您好，這個月的對帳單已經發出，請查收。有問題隨時與我們聯繫。", "zh-Hant"},
		{"英文", "Hello, please find the invoice attached. Let me know if you have any questions about this.", "en"},
		{"日文", "お世話になっております。請求書を添付いたしましたのでご確認ください。", "ja"},
		{"韩文", "안녕하세요. 첨부된 청구서를 확인해 주시기 바랍니다.", "ko"},
		{"俄文", "Здравствуйте! Во вложении счёт за текущий месяц. Пожалуйста, подтвердите получение.", "ru"},
		{"阿拉伯文", "مرحبا، تجد الفاتورة مرفقة. يرجى تأكيد الاستلام في أقرب وقت ممكن.", "ar"},
		{"法文", "Bonjour, vous trouverez la facture dans les pièces jointes. Merci de nous confirmer que vous avez bien reçu ce document.", "fr"},
		{"德文", "Guten Tag, die Rechnung ist als Anhang beigefügt. Bitte teilen Sie uns mit, ob Sie noch Fragen haben.", "de"},
	}
	for _, c := range cases {
		if got := Detect(c.text); got != c.want {
			t.Errorf("%s: Detect = %q，想要 %q", c.name, got, c.want)
		}
	}
}

func TestDetectJapaneseWithManyKanji(t *testing.T) {
	// 汉字比假名多得多的日文（公文体）：只要有假名就不是中文
	const s = "株式会社山田商事 御中 請求書送付の件 平素は格別のご高配を賜り厚く御礼申し上げます。"
	if got := Detect(s); got != "ja" {
		t.Errorf("汉字为主的日文应识别为 ja，得到 %q", got)
	}
}

func TestDetectChineseWithStrayKana(t *testing.T) {
	// 中文里混着「の」这种装饰用字，不能因此判成日文
	const s = "这是我们の年度总结报告，请各位同事在本周五之前完成阅读并提交反馈意见。"
	if got := Detect(s); got != "zh" {
		t.Errorf("夹杂个别假名的中文应仍为 zh，得到 %q", got)
	}
}

func TestDetectUnsureReturnsEmpty(t *testing.T) {
	// 判不准时必须返回空串：硬猜一个语言会把翻译按钮错误地灰掉，
	// 而"没识别出来"最多是多让用户点一次。
	for _, s := range []string{"", "   ", "Thanks!", "12345 67890", "OK", "https://example.com/a/b"} {
		if got := Detect(s); got != "" {
			t.Errorf("Detect(%q) = %q，判不准时应返回空串", s, got)
		}
	}
}

func TestSameLanguage(t *testing.T) {
	if SameLanguage("", "zh") {
		t.Error("识别失败（空串）不能算作与目标语言相同")
	}
	if !SameLanguage("zh", "zh") {
		t.Error("同代码应相同")
	}
	// 简体信翻成繁体是有意义的，不能因为都是中文就当成同一门
	if SameLanguage("zh", "zh-Hant") {
		t.Error("简繁之间不应合并")
	}
}

func TestSupportedAndName(t *testing.T) {
	if !IsSupported(DefaultTarget) {
		t.Errorf("默认目标语言 %q 必须在清单里", DefaultTarget)
	}
	if IsSupported("xx") {
		t.Error("未知代码不应被接受")
	}
	if NameOf("ja") != "Japanese" {
		t.Errorf("NameOf(ja) = %q", NameOf("ja"))
	}
	if NameOf("xx") != "xx" {
		t.Error("未知代码应原样返回，留给模型自己认")
	}
	for _, l := range Supported {
		if l.Code == "" || l.Name == "" || l.Native == "" {
			t.Errorf("清单项 %+v 有空字段", l)
		}
	}
}
