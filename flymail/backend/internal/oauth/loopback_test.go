package oauth

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fetch 访问回调地址并返回状态码与页面内容。
func fetch(t *testing.T, base string, q url.Values) (int, string) {
	t.Helper()
	resp, err := http.Get(base + "?" + q.Encode())
	if err != nil {
		t.Fatalf("请求回调失败: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// waitResult 取一次回调结果，超时即失败（避免测试挂死）。
func waitResult(t *testing.T, s *LoopbackServer) CallbackResult {
	t.Helper()
	select {
	case res := <-s.Results():
		return res
	case <-time.After(3 * time.Second):
		t.Fatal("等待回调结果超时")
		return CallbackResult{}
	}
}

func TestLoopback_Success(t *testing.T) {
	s, err := StartLoopback("the-state")
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer s.Close()

	uri := s.RedirectURI()
	if !strings.HasPrefix(uri, "http://127.0.0.1:") {
		t.Fatalf("回调地址必须绑定回环地址，实际 %s", uri)
	}
	if !strings.HasSuffix(uri, CallbackPath) {
		t.Fatalf("回调路径不符: %s", uri)
	}

	status, page := fetch(t, uri, url.Values{"state": {"the-state"}, "code": {"the-code"}})
	if status != http.StatusOK {
		t.Errorf("状态码 = %d", status)
	}
	if !strings.Contains(page, "授权成功") {
		t.Errorf("结果页内容不符: %s", page)
	}
	res := waitResult(t, s)
	if res.Err != nil || res.Code != "the-code" {
		t.Fatalf("应拿到授权码，实际 %+v", res)
	}
}

// TestLoopback_StateMismatch state 不匹配时必须拒绝：否则本机任意页面都能
// 向这个端口投递伪造的授权码。
func TestLoopback_StateMismatch(t *testing.T) {
	s, err := StartLoopback("right")
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer s.Close()

	status, _ := fetch(t, s.RedirectURI(), url.Values{"state": {"wrong"}, "code": {"c"}})
	if status != http.StatusBadRequest {
		t.Errorf("状态码 = %d，期望 400", status)
	}
	res := waitResult(t, s)
	if res.Err == nil {
		t.Fatal("state 不匹配必须报错")
	}
	if res.Code != "" {
		t.Fatalf("不应带回授权码，实际 %q", res.Code)
	}
}

func TestLoopback_UserDenied(t *testing.T) {
	s, err := StartLoopback("st")
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer s.Close()

	_, page := fetch(t, s.RedirectURI(), url.Values{
		"state": {"st"}, "error": {"access_denied"}, "error_description": {"用户取消了授权"},
	})
	if !strings.Contains(page, "用户取消了授权") {
		t.Errorf("应展示提供方给出的原因: %s", page)
	}
	if res := waitResult(t, s); res.Err == nil {
		t.Fatal("用户拒绝应报错")
	}
}

// TestLoopback_EscapesErrorDescription error_description 来自回调 URL（外部可控），
// 必须转义后再写入页面，否则就是一个本地 XSS 面。
func TestLoopback_EscapesErrorDescription(t *testing.T) {
	s, err := StartLoopback("st")
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer s.Close()

	_, page := fetch(t, s.RedirectURI(), url.Values{
		"state": {"st"}, "error": {"x"}, "error_description": {"<script>alert(1)</script>"},
	})
	if strings.Contains(page, "<script>alert(1)</script>") {
		t.Fatalf("错误描述未转义: %s", page)
	}
	if !strings.Contains(page, "&lt;script&gt;") {
		t.Fatalf("应以转义形式出现: %s", page)
	}
	waitResult(t, s)
}

func TestLoopback_MissingCode(t *testing.T) {
	s, err := StartLoopback("st")
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer s.Close()

	status, _ := fetch(t, s.RedirectURI(), url.Values{"state": {"st"}})
	if status != http.StatusBadRequest {
		t.Errorf("状态码 = %d，期望 400", status)
	}
	if res := waitResult(t, s); res.Err == nil {
		t.Fatal("缺少授权码应报错")
	}
}

// TestLoopback_RepeatCallback 用户刷新回调页会重复触发处理，此时结果通道已满；
// 投递必须是非阻塞的，否则 HTTP 处理协程会永久卡住。
func TestLoopback_RepeatCallback(t *testing.T) {
	s, err := StartLoopback("st")
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer s.Close()

	q := url.Values{"state": {"st"}, "code": {"c"}}
	fetch(t, s.RedirectURI(), q)

	done := make(chan struct{})
	go func() {
		fetch(t, s.RedirectURI(), q)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("重复回调阻塞了处理协程")
	}
	if res := waitResult(t, s); res.Code != "c" {
		t.Fatalf("首个结果应保留，实际 %+v", res)
	}
}

// TestLoopback_CloseIsIdempotent 流程可能同时因成功与超时走到 Close，重复关闭不得 panic。
func TestLoopback_CloseIsIdempotent(t *testing.T) {
	s, err := StartLoopback("st")
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	s.Close()
	s.Close()
	if _, err := http.Get(s.RedirectURI()); err == nil {
		t.Fatal("关闭后端口应不再可用")
	}
}
