package setting

import "testing"

// 对外访问地址的校验。
//
// ── 为什么这项要认真校验 ─────────────────────────────────────────────────────
//
// 它的唯一用途是拼通知里的「打开邮件」链接。填错不会有任何即时反馈——设置页保存
// 成功、通知照发，直到某天有人点了那条链接才发现是死链。所以能在保存那一刻
// 判出来的错，必须当场拦下。
//
// 最容易漏的是 `mail.example.com` 这种没有 scheme 的写法：它看起来完全正常，
// Go 的 url.Parse 也**不会报错**（当成相对路径，Host 为空），拼出来是
// `mail.example.com/?message=1`。
func TestNormalizeBaseURL(t *testing.T) {
	ok := []struct{ in, want string }{
		{"", ""},
		{"https://mail.example.com", "https://mail.example.com"},
		// 结尾斜杠统一去掉，拼链接的地方就不必两边都判断
		{"https://mail.example.com/", "https://mail.example.com"},
		{"http://192.168.5.11:8086", "http://192.168.5.11:8086"},
		{"http://192.168.5.11:8086/", "http://192.168.5.11:8086"},
		// 反代挂在子路径下是常见部署形态
		{"https://example.com/mail", "https://example.com/mail"},
		{"  https://mail.example.com  ", "https://mail.example.com"},
		// 查询串与锚点会和我们自己的参数打架，丢掉
		{"https://mail.example.com/?x=1#top", "https://mail.example.com"},
	}
	for _, tc := range ok {
		got, err := NormalizeBaseURL(tc.in)
		if err != nil {
			t.Errorf("%q 应当合法，却报错：%v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q 规整成了 %q，想要 %q", tc.in, got, tc.want)
		}
	}

	bad := []struct{ in, why string }{
		{"mail.example.com", "没有 scheme：url.Parse 不报错，拼出来是死链"},
		{"//mail.example.com", "只有 // 也没有 scheme"},
		{"ftp://mail.example.com", "不是 http/https"},
		{"mailto:a@b.com", "不是 http/https"},
		{"http://", "缺少主机名"},
		{"https:///path", "缺少主机名"},
	}
	for _, tc := range bad {
		if got, err := NormalizeBaseURL(tc.in); err == nil {
			t.Errorf("%q 应当被拒（%s），却通过了，结果 %q", tc.in, tc.why, got)
		}
	}
}

// 规整结果再规整一次不变。设置页每次保存都会跑一遍，不幂等的话地址会被反复削短。
func TestNormalizeBaseURLIsIdempotent(t *testing.T) {
	for _, in := range []string{"https://mail.example.com/", "https://example.com/mail/", ""} {
		once, err := NormalizeBaseURL(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		twice, err := NormalizeBaseURL(once)
		if err != nil {
			t.Fatalf("%q 第二次: %v", once, err)
		}
		if once != twice {
			t.Errorf("不幂等：%q → %q → %q", in, once, twice)
		}
	}
}
