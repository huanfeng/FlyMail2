package parser

import (
	"strings"
	"testing"
	"time"

	"flymail-core/types"
)

// 畸形 multipart 必须让解析**结束**，不能空转。
//
// ── 缘起（2026-09-16 线上事故） ───────────────────────────────────────────────
//
// 163 账户的同步从 11:07 起彻底停住：没有新日志、没有报错、诊断接口还显示
// connected=true，容器里却已经没有到 163 的 socket。SIGQUIT 拿到的栈显示，
// runner 那条 goroutine 状态是 **runnable**（在跑，不是在等）：
//
//	parser.walkParts → mail.Reader.NextPart → textproto.MultipartReader.NextPart
//	                 → fmt.Errorf(...)                     ← 一直在造同一个错误
//
// 根因是 walkParts 里这段「跳过畸形部件」：
//
//	p, err := mr.NextPart()
//	if err == io.EOF { break }
//	if err != nil { continue }   // ← 死循环
//
// go-message 的 mail.Reader.NextPart 只有在 io.EOF 时才把读完的 multipart
// **弹出栈**；返回其它错误时栈原样不动。而底层 textproto.MultipartReader 在
// 读到数据末尾却没见到结束边界时，每次都返回同一个
// `multipart: NextPart: unexpected EOF`，且**不消耗任何输入**——错误是粘性的。
// 于是 continue 一次就再错一次，一条 goroutine 占满一个核，那个账户从此不再同步。
//
// 例外是未知字符集与未知传输编码：坏的只是那一个部件的头，边界已经吃掉了，
// 读取器确实推进了，跳过它是安全的。所以判据不是「错误就停」，而是
// 「读取器还会不会前进」——见 TestUnknownEncodingPartSkippedButRestParsed。
//
// ── 这些用例钉的是「会结束」 ─────────────────────────────────────────────────
//
// 每条都带独立超时。不加超时的话，缺陷复发时测试不是失败而是**挂住**，
// CI 上表现为跑不完，没人知道是哪一条。
func TestMalformedMultipartTerminates(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{
			// 事故现场那一类：正文被截断，结束边界 --b-- 根本没出现。
			// 邮件在传输中被截断、或服务端给出的 literal 长度与实际不符时就是这样。
			name: "缺少结束边界",
			raw: "MIME-Version: 1.0\r\n" +
				"Content-Type: multipart/mixed; boundary=b\r\n" +
				"\r\n" +
				"--b\r\n" +
				"Content-Type: text/plain\r\n" +
				"\r\n" +
				"正文被截断了",
		},
		{
			// 头写着 multipart，底下一个边界都没有
			name: "一个边界都没有",
			raw: "MIME-Version: 1.0\r\n" +
				"Content-Type: multipart/mixed; boundary=b\r\n" +
				"\r\n" +
				"根本不是 multipart 的内容",
		},
		{
			// 嵌套的内层缺结束边界：外层还有后续部件，考验「读到哪算哪」
			name: "嵌套内层截断",
			raw: "MIME-Version: 1.0\r\n" +
				"Content-Type: multipart/mixed; boundary=out\r\n" +
				"\r\n" +
				"--out\r\n" +
				"Content-Type: multipart/alternative; boundary=in\r\n" +
				"\r\n" +
				"--in\r\n" +
				"Content-Type: text/plain\r\n" +
				"\r\n" +
				"内层第一段",
		},
		{
			// 结束边界写错了名字
			name: "结束边界名字不匹配",
			raw: "MIME-Version: 1.0\r\n" +
				"Content-Type: multipart/mixed; boundary=b\r\n" +
				"\r\n" +
				"--b\r\n" +
				"Content-Type: text/plain\r\n" +
				"\r\n" +
				"一段正文\r\n" +
				"--other--\r\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan struct{})
			var email types.ParsedEmail
			go func() {
				defer close(done)
				_ = ParseBody(strings.NewReader(tc.raw), &email, true)
			}()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				// 复发时这里是唯一能说清「卡在哪」的地方
				t.Fatal("ParseBody 没有返回：畸形 multipart 让解析陷入死循环，" +
					"线上表现为该账户的同步 goroutine 占满一个核后永不再同步")
			}
		})
	}
}

// 截断之前已经读到的部件要留下来，不能因为后面坏了就整封丢掉。
//
// 「遇错就停」如果写成「遇错就把结果清空」，用户看到的是一封空邮件——
// 比死循环好不到哪去。截断点之前的内容是完好的，应当照常呈现。
func TestTruncatedMultipartKeepsPartsReadSoFar(t *testing.T) {
	raw := "MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=b\r\n" +
		"\r\n" +
		"--b\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"截断前就读到的正文\r\n" +
		"--b\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"第二段还没写完就断了"

	done := make(chan struct{})
	var email types.ParsedEmail
	go func() {
		defer close(done)
		_ = ParseBody(strings.NewReader(raw), &email, true)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ParseBody 没有返回")
	}

	if !strings.Contains(email.TextBody, "截断前就读到的正文") {
		t.Fatalf("截断点之前的正文丢了，用户会看到一封空邮件；实际拿到 %q", email.TextBody)
	}
}

// 头坏掉的部件跳过之后，后面的部件还要照常解析出来。
//
// 「遇到非 EOF 错误就停」是个看起来很稳、实则有回归的修法：未知字符集、
// 未知传输编码这两类错误**不粘**——坏的只是那一个部件的头，边界已经吃掉了，
// 下一次调用能拿到后面的部件。就地停下会把后面完好的正文一起丢掉。
// 这条用例正是拿来挡那种修法的。
func TestUnknownEncodingPartSkippedButRestParsed(t *testing.T) {
	raw := "MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=b\r\n" +
		"\r\n" +
		"--b\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: x-weird-encoding\r\n" +
		"\r\n" +
		"这一段的传输编码没人认识\r\n" +
		"--b\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"这一段完全正常\r\n" +
		"--b--\r\n"

	var email types.ParsedEmail
	if err := ParseBody(strings.NewReader(raw), &email, true); err != nil {
		t.Fatalf("ParseBody 出错：%v", err)
	}
	if !strings.Contains(email.TextBody, "这一段完全正常") {
		t.Fatalf("坏部件之后的正文被一起丢掉了，实际拿到 %q", email.TextBody)
	}
}

// 正常的 multipart 不受影响——这条是前提，没有它「不死循环」可以靠
// 「第一个部件就返回」来作弊通过。
func TestWellFormedMultipartStillParsed(t *testing.T) {
	raw := "MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=b\r\n" +
		"\r\n" +
		"--b\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"纯文本正文\r\n" +
		"--b\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<p>HTML 正文</p>\r\n" +
		"--b--\r\n"

	var email types.ParsedEmail
	if err := ParseBody(strings.NewReader(raw), &email, true); err != nil {
		t.Fatalf("ParseBody 出错：%v", err)
	}
	if !strings.Contains(email.TextBody, "纯文本正文") {
		t.Fatalf("纯文本正文没解析出来：%q", email.TextBody)
	}
	if !strings.Contains(email.HTMLBody, "HTML 正文") {
		t.Fatalf("HTML 正文没解析出来：%q", email.HTMLBody)
	}
}
