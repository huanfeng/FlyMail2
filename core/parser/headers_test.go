package parser

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"flymail-core/types"
)

// ParseHeaders 的边界情况。
//
// 这些用例针对的是「从服务端的 ENVELOPE 改成自己解析邮件头」之后**换了实现**
// 的那部分：地址不再是服务端替我们切好的 mailbox/host，而是我们自己按
// RFC 5322 解析一行原始头。下面每一条都是真实邮件里会出现、而两种实现
// 容易给出不同结果的形状。
func TestParseHeadersEdgeCases(t *testing.T) {
	addrs := func(l []types.Address) string {
		var b []string
		for _, a := range l {
			b = append(b, a.Name+"|"+a.Email)
		}
		return strings.Join(b, ",")
	}

	cases := []struct {
		name   string
		header string
		check  func(t *testing.T, e *types.ParsedEmail)
	}{
		{
			// QQ 已发送里那两封就是这样：有 To 头但值是空的（只发了密送）。
			// 服务端的信封在这种邮件上会把 Bcc 挪进 cc 槽位；自己解析则各归各位。
			name:   "To 头存在但为空",
			header: "From: a@x.com\r\nTo: \r\nBcc: b@y.com\r\nSubject: s\r\n",
			check: func(t *testing.T, e *types.ParsedEmail) {
				if len(e.To) != 0 {
					t.Errorf("空的 To 头不该解析出收件人：%+v", e.To)
				}
				if addrs(e.BCC) != "|b@y.com" {
					t.Errorf("密送错了：%q", addrs(e.BCC))
				}
			},
		},
		{
			name:   "只有邮箱没有显示名",
			header: "From: plain@x.com\r\nTo: one@y.com, two@z.com\r\n",
			check: func(t *testing.T, e *types.ParsedEmail) {
				if addrs(e.From) != "|plain@x.com" {
					t.Errorf("发件人：%q", addrs(e.From))
				}
				if addrs(e.To) != "|one@y.com,|two@z.com" {
					t.Errorf("收件人：%q", addrs(e.To))
				}
			},
		},
		{
			// 中文显示名一律是编码字，这是国内邮件的常态
			name:   "编码过的显示名",
			header: "From: =?utf-8?B?5byg5LiJ?= <zhang@x.com>\r\nSubject: =?gbk?B?u7bTrcq508M=?=\r\n",
			check: func(t *testing.T, e *types.ParsedEmail) {
				if addrs(e.From) != "张三|zhang@x.com" {
					t.Errorf("发件人：%q", addrs(e.From))
				}
				if e.Subject == "" || strings.Contains(e.Subject, "=?") {
					t.Errorf("主题没解码：%q", e.Subject)
				}
			},
		},
		{
			// 长收件人列表会被折行，续行以空白开头
			name:   "折行的收件人列表",
			header: "To: one@x.com,\r\n two@x.com,\r\n\tthree@x.com\r\n",
			check: func(t *testing.T, e *types.ParsedEmail) {
				if len(e.To) != 3 {
					t.Errorf("折行没接上，只解析出 %d 个：%q", len(e.To), addrs(e.To))
				}
			},
		},
		{
			// 群发邮件常见：To: undisclosed-recipients:;
			name:   "群组语法且无成员",
			header: "From: a@x.com\r\nTo: undisclosed-recipients:;\r\n",
			check: func(t *testing.T, e *types.ParsedEmail) {
				// 解析不出具体地址是对的，关键是别把整封的其它字段带崩
				if addrs(e.From) != "|a@x.com" {
					t.Errorf("群组语法把发件人也搞没了：%q", addrs(e.From))
				}
			},
		},
		{
			// 地址栏写得不合语法时，不能连带把主题、Message-ID 一起丢掉
			name:   "地址写法非法",
			header: "From: not-an-address\r\nSubject: still here\r\nMessage-ID: <keep@me>\r\n",
			check: func(t *testing.T, e *types.ParsedEmail) {
				if e.Subject != "still here" {
					t.Errorf("主题被连累了：%q", e.Subject)
				}
				if e.MessageID != "keep@me" {
					t.Errorf("Message-ID 被连累了：%q", e.MessageID)
				}
			},
		},
		{
			name:   "区段不以空行结尾",
			header: "Subject: no trailing blank\r\nFrom: a@x.com",
			check: func(t *testing.T, e *types.ParsedEmail) {
				if e.Subject != "no trailing blank" {
					t.Errorf("主题：%q", e.Subject)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var e types.ParsedEmail
			if err := ParseHeaders(strings.NewReader(tc.header), &e); err != nil {
				t.Fatalf("ParseHeaders 出错：%v", err)
			}
			tc.check(t, &e)
		})
	}
}

// ⚠ Date 头只在 INTERNALDATE 缺失时才用。
//
// 两者语义不同：INTERNALDATE 是这封信到达服务器的时间，由服务器盖章；
// Date 是发件方自己写的，可以是任意值。按 Date 排序的话，一封声称来自
// 2030 年的垃圾邮件会永远钉在列表最上面——这是邮件客户端的经典坑。
//
// 原先「ENVELOPE 的日期只作兜底」就是这个规则，换成从头里取之后必须保持不变。
func TestParseHeadersDateOnlyFillsWhenMissing(t *testing.T) {
	const header = "Date: Fri, 01 Jan 2100 00:00:00 +0800\r\nSubject: s\r\n"

	// 已经有 INTERNALDATE：不许被 Date 头顶掉
	arrived := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	e := types.ParsedEmail{Date: arrived}
	if err := ParseHeaders(strings.NewReader(header), &e); err != nil {
		t.Fatalf("ParseHeaders 出错：%v", err)
	}
	if !e.Date.Equal(arrived) {
		t.Fatalf("到达时间被信头里的 Date 顶掉了：%v（一封声称来自 2100 年的邮件会霸占列表顶部）", e.Date)
	}

	// 没有 INTERNALDATE：才用 Date 头兜底
	var e2 types.ParsedEmail
	if err := ParseHeaders(strings.NewReader(header), &e2); err != nil {
		t.Fatalf("ParseHeaders 出错：%v", err)
	}
	if e2.Date.IsZero() {
		t.Fatal("没有到达时间时应当用信头里的 Date 兜底，结果是零值")
	}
}

// ParseHeaders 不碰正文与附件。
//
// 元数据抓取每轮都会跑它，如果它顺手把 TextBody 写成空串，
// 已经抓下来的正文就会被一轮同步抹掉。
func TestParseHeadersLeavesBodyAlone(t *testing.T) {
	e := types.ParsedEmail{
		TextBody:    "已经抓下来的正文",
		HTMLBody:    "<p>已经抓下来的正文</p>",
		Attachments: []types.Attachment{{Filename: "a.pdf"}},
	}
	if err := ParseHeaders(strings.NewReader("Subject: s\r\nFrom: a@x.com\r\n"), &e); err != nil {
		t.Fatalf("ParseHeaders 出错：%v", err)
	}
	if e.TextBody != "已经抓下来的正文" || e.HTMLBody == "" {
		t.Errorf("正文被动了：%q / %q", e.TextBody, e.HTMLBody)
	}
	if len(e.Attachments) != 1 {
		t.Errorf("附件被动了：%+v", e.Attachments)
	}
}

// 邮件头里的原始 8 位字节（不是 RFC 2047 编码字）要能解出来。
//
// ⚠ 这是从 ENVELOPE 改成自己解析邮件头之后**必须自己承担**的一层。
// QQ 收件箱 1579 封里有 76 封是这样写的，服务端生成信封时替我们转了码，
// 自己解析就得自己转。不转的后果不只是主题乱码——Go 的地址解析器遇到非法
// UTF-8 会直接报错，**发件人整个丢失**，列表上那一行没有任何来源信息。
func TestParseHeadersDecodesRaw8BitBytes(t *testing.T) {
	// QQ 收件箱 UID 403 的原样字节：GBK 的「QQ空间项目组」与一句中文主题
	raw := []byte("From: \"QQ\xbf\xd5\xbc\xe4\xcf\xee\xc4\xbf\xd7\xe9\" <qzone@tencent.com>\r\n" +
		"Subject: \xc4\xe3\xb5\xc4\xc5\xf3\xd3\xd1\xd3\xd6\xd3\xd0\xd0\xc2\xb6\xaf\xcc\xac\xc1\xcb\r\n")

	var e types.ParsedEmail
	if err := ParseHeaders(bytes.NewReader(raw), &e); err != nil {
		t.Fatalf("ParseHeaders 出错：%v", err)
	}
	if len(e.From) != 1 || e.From[0].Email != "qzone@tencent.com" {
		t.Fatalf("发件人丢了：%+v（原始 8 位字节会让地址解析直接失败）", e.From)
	}
	if e.From[0].Name != "QQ空间项目组" {
		t.Errorf("发件人显示名没转码：%q", e.From[0].Name)
	}
	if e.Subject != "你的朋友又有新动态了" {
		t.Errorf("主题没转码：%q", e.Subject)
	}
	if !utf8.ValidString(e.Subject) {
		t.Error("主题不是合法 UTF-8，落库就是坏字节")
	}
}

// 合法 UTF-8 的头原样放行，不能被当成 GB18030 再解一遍。
//
// 没有这条，「一律按 GB18030 解」也能让上面那条通过，代价是把所有现代邮件
// 的中文主题全部变成乱码——比原来的问题波及面大得多。
func TestParseHeadersLeavesValidUTF8Alone(t *testing.T) {
	cases := []struct{ name, header, wantSubject string }{
		{"RFC 2047 编码字", "Subject: =?utf-8?B?5Lit5paH5qCH6aKY?=\r\nFrom: a@x.com\r\n", "中文标题"},
		{"原始 UTF-8 字节", "Subject: 直接写的中文\r\nFrom: a@x.com\r\n", "直接写的中文"},
		{"纯 ASCII", "Subject: plain ascii\r\nFrom: a@x.com\r\n", "plain ascii"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var e types.ParsedEmail
			if err := ParseHeaders(strings.NewReader(tc.header), &e); err != nil {
				t.Fatalf("ParseHeaders 出错：%v", err)
			}
			if e.Subject != tc.wantSubject {
				t.Errorf("主题被改坏了：想要 %q，拿到 %q", tc.wantSubject, e.Subject)
			}
			if len(e.From) != 1 || e.From[0].Email != "a@x.com" {
				t.Errorf("发件人：%+v", e.From)
			}
		})
	}
}

// 地址头不合语法时也要尽量把地址捞出来。
//
// ⚠ Go 的地址解析器是全有或全无：一个字符不合 RFC 5322，**整个头一个地址都不返回**，
// 那封邮件在列表上就没有任何来源信息。以前这类由服务端的 ENVELOPE 兜着
// （服务端自己宽松解析过一遍），改成自己解析之后必须自己兜。
func TestParseHeadersLenientAddressFallback(t *testing.T) {
	cases := []struct {
		name      string
		header    string
		wantName  string
		wantEmail string
	}{
		{
			// 实测：QQ 邮件归档 UID 21。显示名是个没加引号的邮箱，@ 在 phrase 里非法
			name:      "显示名是没加引号的邮箱",
			header:    "From: 458889595@qq.com <n428b3992826@sina.com>\r\n",
			wantName:  "458889595@qq.com",
			wantEmail: "n428b3992826@sina.com",
		},
		{
			name:      "显示名里有未转义的逗号",
			header:    "From: 张三, 技术部 <zhang@x.com>\r\n",
			wantEmail: "zhang@x.com",
		},
		{
			name:      "显示名里有未转义的方括号",
			header:    "From: [自动通知] <noreply@x.com>\r\n",
			wantEmail: "noreply@x.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var e types.ParsedEmail
			if err := ParseHeaders(strings.NewReader(tc.header), &e); err != nil {
				t.Fatalf("ParseHeaders 出错：%v", err)
			}
			if len(e.From) == 0 {
				t.Fatalf("发件人整个丢了，这一行在列表上没有来源信息")
			}
			if e.From[0].Email != tc.wantEmail {
				t.Errorf("地址：想要 %q，拿到 %q", tc.wantEmail, e.From[0].Email)
			}
			if tc.wantName != "" && e.From[0].Name != tc.wantName {
				t.Errorf("显示名：想要 %q，拿到 %q", tc.wantName, e.From[0].Name)
			}
		})
	}
}

// 宽松兜底不许把「确实没有地址」变出地址来。
//
// 只钉「捞得出来」是危险的：一个「见 @ 就当地址」的实现能让上面全过，
// 代价是把正文片段、空地址、群组语法都存成收件人。
func TestLenientFallbackDoesNotInventAddresses(t *testing.T) {
	cases := []struct{ name, header string }{
		{"空的 To 头", "To: \r\nFrom: a@x.com\r\n"},
		{"空的尖括号", "From: \"\" <>\r\n"}, // 实测：QQ 收件箱 UID 40
		{"群组语法无成员", "To: undisclosed-recipients:;\r\nFrom: a@x.com\r\n"},
		{"根本不是地址", "To: 详见正文说明\r\nFrom: a@x.com\r\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var e types.ParsedEmail
			if err := ParseHeaders(strings.NewReader(tc.header), &e); err != nil {
				t.Fatalf("ParseHeaders 出错：%v", err)
			}
			got := e.To
			if strings.HasPrefix(tc.header, "From:") {
				got = e.From
			}
			if len(got) != 0 {
				t.Errorf("凭空造出了地址：%+v", got)
			}
		})
	}
}
