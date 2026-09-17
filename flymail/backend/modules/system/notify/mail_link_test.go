package notify

import (
	"net/url"
	"strings"
	"testing"
)

// 通知里那条「打开邮件」链接的形状。
func TestMailLink(t *testing.T) {
	const base = "https://mail.example.com"

	got := MailLink(base, 1, 7, 42)
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("拼出来的不是合法 URL：%q（%v）", got, err)
	}
	if u.Scheme != "https" || u.Host != "mail.example.com" {
		t.Errorf("主机部分不对：%q", got)
	}
	q := u.Query()
	for k, want := range map[string]string{"account": "1", "folder": "7", "message": "42"} {
		if q.Get(k) != want {
			t.Errorf("参数 %s：想要 %q，拿到 %q（完整链接 %q）", k, want, q.Get(k), got)
		}
	}
}

// ⚠ 没配对外访问地址就不带链接。
//
// 这是主要的反向：一个「拼不出就用相对路径」的实现会发出 `/?message=42` 这种
// 在飞书里点了打不开的东西，而用户要到点击那一刻才发现——比不带链接更糟。
func TestMailLinkWithoutBase(t *testing.T) {
	for _, base := range []string{"", "   "} {
		if got := MailLink(base, 1, 7, 42); got != "" {
			t.Errorf("base=%q 却拼出了链接：%q", base, got)
		}
	}
}

// 文件夹拿不到时也要能用。
//
// 邮件刚好被规则移走、或查库失败时 folderID 是 0，此时链接只带 account 与 message：
// 右边的邮件能打开，左边那列由前端自己兜底，总比整条链接不发强。
func TestMailLinkWithoutFolder(t *testing.T) {
	got := MailLink("https://mail.example.com", 1, 0, 42)
	if !strings.Contains(got, "message=42") || !strings.Contains(got, "account=1") {
		t.Errorf("缺文件夹时链接不完整：%q", got)
	}
	if strings.Contains(got, "folder=") {
		t.Errorf("不该带空的 folder 参数：%q", got)
	}
}

// 结尾斜杠不该拼出双斜杠。设置那边已经规整过，这里再兜一次——
// 它是个导出函数，调用方不止一个。
func TestMailLinkTrailingSlash(t *testing.T) {
	a := MailLink("https://mail.example.com", 1, 7, 42)
	b := MailLink("https://mail.example.com/", 1, 7, 42)
	if a != b {
		t.Errorf("结尾斜杠影响了结果：\n  %q\n  %q", a, b)
	}
	if strings.Contains(a, "//?") {
		t.Errorf("拼出了双斜杠：%q", a)
	}
}
