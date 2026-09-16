package imap

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"

	imapv2 "github.com/emersion/go-imap/v2"

	"flymail-core/types"
)

// 元数据抓取不许再依赖服务端的 ENVELOPE。
//
// ── 缘起（2026-09-16） ───────────────────────────────────────────────────────
//
// QQ 对少数邮件返回的 ENVELOPE 只有九个字段（RFC 3501 要求恰好十个，缺的值要写
// NIL 占位）。1583 封里有 3 封是这样，全是缺 To/Cc/Bcc 之一时不补占位、后面的
// 地址列表整体左移。go-imap 严格按 RFC 解析，报
//
//	fetch close: in response-data: in envelope: imapwire: expected SP, got ")"
//
// 而且这属于协议错误，go-imap 会**连带拆掉整条连接**。后果被放大到离谱：
// 那一批 200 封整批丢弃、该文件夹中止、同一轮里其余 14 个文件夹全部以
// 「连接已关闭」失败、连续 5 次后熔断。那个账户一轮同步都没成功过，
// 而且永远好不了——每次重试都撞在同一封上。
//
// 这不是新问题。MailKit 2018 年就为 QQ 的同一类缺陷加过绕行（issue #669），
// 八年过去 QQ 没修。指望服务端改是不现实的。
//
// ── 这条用例怎么钉 ──────────────────────────────────────────────────────────
//
// 假服务端**一旦被问到 ENVELOPE 就返回 QQ 那种畸形值**；只问邮件头则老实回答。
// 于是：
//
//	不要 ENVELOPE  →  抓取成功，字段齐全      ← 现在的行为
//	要了 ENVELOPE  →  解析失败，连接被拆      ← 回退的行为
//
// 比「扫源码看有没有 Envelope: true」强的地方在于，它钉的是后果而不是写法：
// 换个写法、换个库，只要不再把解析外包给服务端就照样通过。
func TestMetadataFetchSurvivesBrokenServerEnvelope(t *testing.T) {
	// QQ 收件箱 UID 331 的原样字节：九个字段，Cc 被塞进了 to 槽位
	const brokenEnvelope = `("Tue, 26 Jan 2016 14:55:38 +0800" ` +
		`"=?utf-8?B?5Lq/6L+e5omL5py65LqS6IGU5Lqn5ZOB566A5oql77yM5pWs6K+36KeC55yL77yB?=" ` +
		`(("sales" NIL "sales" "carbit.com.cn")) (("sales" NIL "sales" "carbit.com.cn")) ` +
		`(("sales" NIL "sales" "carbit.com.cn")) (("xiaol" NIL "xiaol" "carbit.com.cn")) ` +
		`NIL NIL "<2016012614553700919824@carbit.com.cn>")`

	const headerBlock = "Date: Tue, 26 Jan 2016 14:55:38 +0800\r\n" +
		"Subject: =?utf-8?B?5rWL6K+V5Li76aKY?=\r\n" +
		"From: =?utf-8?B?6ZSA5ZSu?= <sales@carbit.com.cn>\r\n" +
		"Cc: xiaol@carbit.com.cn\r\n" +
		"Message-ID: <2016012614553700919824@carbit.com.cn>\r\n"

	askedEnvelope := false
	srv := fakeFetchServer(t, func(cmdLine string, tag string, w *bufio.Writer) {
		if strings.Contains(strings.ToUpper(cmdLine), "ENVELOPE") {
			askedEnvelope = true
			fmt.Fprintf(w, "* 1 FETCH (UID 331 ENVELOPE %s)\r\n", brokenEnvelope)
			fmt.Fprintf(w, "%s OK FETCH completed\r\n", tag)
			return
		}
		writeHeaderSection(w, cmdLine, tag, 331, headerBlock)
	})

	sess := srv.dial(t)
	if _, err := sess.SelectFolder("INBOX"); err != nil {
		t.Fatalf("SELECT 失败：%v", err)
	}

	emails, err := sess.FetchByUIDs([]imapv2.UID{331}, FetchOptions{})
	if err != nil {
		t.Fatalf("元数据抓取失败：%v\n（服务端的信封是坏的，但我们本来就不该去问它）", err)
	}
	if askedEnvelope {
		t.Fatal("仍然向服务端要了 ENVELOPE——只要有一家的信封不标准，整个账户就会卡死")
	}
	if len(emails) != 1 {
		t.Fatalf("期望 1 封，拿到 %d 封", len(emails))
	}

	e := emails[0]
	if e.Subject != "测试主题" {
		t.Errorf("主题没解析出来：%q", e.Subject)
	}
	if len(e.From) != 1 || e.From[0].Email != "sales@carbit.com.cn" || e.From[0].Name != "销售" {
		t.Errorf("发件人没解析出来：%+v", e.From)
	}
	// ⚠ 这封信没有 To 头，只有 Cc。QQ 的信封把 Cc 塞进了 to 槽位；
	// 从头里解析则各归各位——宽松解析救不回这一点，只会静默记错。
	if len(e.To) != 0 {
		t.Errorf("这封信没有 To 头，却解析出了收件人：%+v", e.To)
	}
	if len(e.CC) != 1 || e.CC[0].Email != "xiaol@carbit.com.cn" {
		t.Errorf("抄送没解析出来：%+v", e.CC)
	}
	if e.MessageID != "2016012614553700919824@carbit.com.cn" {
		t.Errorf("Message-ID 没解析出来：%q", e.MessageID)
	}
}

// 信封字段该有的都要有。
//
// 没有这条，上面那条可以靠「什么都不解析」通过：不问 ENVELOPE、也不解析头，
// 抓取当然不报错，代价是整个邮件列表没有主题和发件人。
func TestMetadataFetchFillsAllEnvelopeFields(t *testing.T) {
	const headerBlock = "Date: Mon, 30 Oct 2017 11:05:21 +0800\r\n" +
		"Subject: =?utf-8?B?5aSa5pS25Lu25Lq6?=\r\n" +
		"From: Alice <alice@example.com>\r\n" +
		"Reply-To: list@example.com\r\n" +
		"To: Bob <bob@example.com>, carol@example.com\r\n" +
		"Cc: dave@example.com\r\n" +
		"Bcc: eve@example.com\r\n" +
		"Message-ID: <mid-1@example.com>\r\n" +
		"In-Reply-To: <parent@example.com>\r\n" +
		"References: <root@example.com> <parent@example.com>\r\n"

	srv := fakeFetchServer(t, func(cmdLine, tag string, w *bufio.Writer) {
		writeHeaderSection(w, cmdLine, tag, 7, headerBlock)
	})

	sess := srv.dial(t)
	if _, err := sess.SelectFolder("INBOX"); err != nil {
		t.Fatalf("SELECT 失败：%v", err)
	}
	emails, err := sess.FetchByUIDs([]imapv2.UID{7}, FetchOptions{})
	if err != nil || len(emails) != 1 {
		t.Fatalf("抓取失败：%v（%d 封）", err, len(emails))
	}
	e := emails[0]

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"主题", e.Subject, "多收件人"},
		{"Message-ID", e.MessageID, "mid-1@example.com"},
		{"In-Reply-To", e.InReplyTo, "parent@example.com"},
		{"References", e.References, "root@example.com parent@example.com"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s：想要 %q，拿到 %q", c.name, c.want, c.got)
		}
	}

	addrs := func(l []types.Address) string {
		var b []string
		for _, a := range l {
			b = append(b, a.Name+"|"+a.Email)
		}
		return strings.Join(b, ",")
	}
	for _, c := range []struct {
		name string
		got  string
		want string
	}{
		{"发件人", addrs(e.From), "Alice|alice@example.com"},
		{"收件人", addrs(e.To), "Bob|bob@example.com,|carol@example.com"},
		{"抄送", addrs(e.CC), "|dave@example.com"},
		{"密送", addrs(e.BCC), "|eve@example.com"},
		{"回复至", addrs(e.ReplyTo), "|list@example.com"},
	} {
		if c.got != c.want {
			t.Errorf("%s：想要 %q，拿到 %q", c.name, c.want, c.got)
		}
	}
}

// ── 假服务端 ────────────────────────────────────────────────────────────────

type fetchServer struct{ ln net.Listener }

// fakeFetchServer 起一个最小 IMAP 服务端，FETCH 的应答交给 onFetch 决定。
func fakeFetchServer(t *testing.T, onFetch func(cmdLine, tag string, w *bufio.Writer)) *fetchServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				w := bufio.NewWriter(c)
				fmt.Fprintf(w, "* OK [CAPABILITY IMAP4rev1] ready\r\n")
				w.Flush()
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimRight(line, "\r\n")
					f := strings.SplitN(line, " ", 3)
					if len(f) < 2 {
						continue
					}
					tag, cmd := f[0], strings.ToUpper(f[1])
					switch cmd {
					case "LOGIN":
						fmt.Fprintf(w, "%s OK [CAPABILITY IMAP4rev1] ok\r\n", tag)
					case "SELECT":
						fmt.Fprintf(w, "* 1 EXISTS\r\n* OK [UIDVALIDITY 1]\r\n* OK [UIDNEXT 999]\r\n%s OK [READ-WRITE] ok\r\n", tag)
					case "UID":
						onFetch(line, tag, w)
					case "LOGOUT":
						fmt.Fprintf(w, "* BYE\r\n%s OK ok\r\n", tag)
						w.Flush()
						return
					default:
						fmt.Fprintf(w, "%s OK ok\r\n", tag)
					}
					w.Flush()
				}
			}(c)
		}
	}()
	return &fetchServer{ln: ln}
}

func (f *fetchServer) dial(t *testing.T) *Session {
	t.Helper()
	a := f.ln.Addr().(*net.TCPAddr)
	sess, err := Dial(types.IMAPConfig{
		Host: "127.0.0.1", Port: a.Port,
		Username: "u", Password: "p",
		Security: types.SecurityNone,
	})
	if err != nil {
		t.Fatalf("Dial 失败：%v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

// writeHeaderSection 模仿真实服务器对两种取头写法的差别。
//
// ⚠ 对 HEADER.FIELDS 返回**空内容**是照着实测来的，不是刁难：go-imap 把字段名
// 写成带引号的字符串，而 GreenMail 与 QQ 对这种写法一律回空
// （实测 0 字节 / 2 字节，见 envelopeHeaderSection 的说明）。
func writeHeaderSection(w *bufio.Writer, cmdLine, tag string, uid int, header string) {
	if strings.Contains(strings.ToUpper(cmdLine), "HEADER.FIELDS") {
		fmt.Fprintf(w, "* 1 FETCH (UID %d BODY[HEADER.FIELDS (\"Date\" \"Subject\")] {0}\r\n)\r\n", uid)
		fmt.Fprintf(w, "%s OK FETCH completed\r\n", tag)
		return
	}
	fmt.Fprintf(w, "* 1 FETCH (UID %d BODY[HEADER] {%d}\r\n%s)\r\n", uid, len(header), header)
	fmt.Fprintf(w, "%s OK FETCH completed\r\n", tag)
}

// 不许用 HEADER.FIELDS 只取需要的几个字段。
//
// 省带宽的写法在这里是行不通的：go-imap 一律给字段名加引号，而 GreenMail 和 QQ
// 对加引号的字段表返回空内容。失败方式还特别阴——不报错，只是每封邮件都没有
// 主题和发件人，同步照常「成功」。
//
// 上面两条用例的假服务端已经照这个行为写了，所以这条其实是把判据讲明白：
// 一旦有人为了省流量改回字段表，那两条会立刻变红。
func TestHeaderFieldListIsNotUsed(t *testing.T) {
	if len(envelopeHeaderSection.HeaderFields) != 0 {
		t.Fatalf("用了 HEADER.FIELDS 字段表 %v：go-imap 会给字段名加引号，"+
			"GreenMail 与 QQ 对这种写法返回空内容，结果是所有邮件都没有主题和发件人",
			envelopeHeaderSection.HeaderFields)
	}
	if envelopeHeaderSection.Specifier != imapv2.PartSpecifierHeader {
		t.Fatalf("取头的区段规格不对：%q", envelopeHeaderSection.Specifier)
	}
	if !envelopeHeaderSection.Peek {
		t.Fatal("少了 Peek，光是同步就会把服务端的未读邮件标成已读")
	}
}
