package auth

import (
	"testing"
	"time"
)

// TestAttachmentTokenScope：附件令牌只对签发时那一封有效；access token 仍可用；refresh token 不行。
func TestAttachmentTokenScope(t *testing.T) {
	s := &Service{opts: Options{JWTSecret: "test-secret", AccessTTLMin: 5, RefreshTTLHour: 1}}
	tok, err := s.IssueAttachmentToken(42)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyAttachmentAccess(tok, 42); err != nil {
		t.Errorf("own message: %v", err)
	}
	if err := s.VerifyAttachmentAccess(tok, 43); err == nil {
		t.Errorf("attachment token must not open another message's attachments")
	}
	pair, err := s.issuePair("admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyAttachmentAccess(pair.AccessToken, 42); err != nil {
		t.Errorf("access token should still work: %v", err)
	}
	if err := s.VerifyAttachmentAccess(pair.RefreshToken, 42); err == nil {
		t.Errorf("refresh token must be rejected")
	}
	// 附件令牌不能当 access token 用（不能拿它去调其它接口）
	if _, err := s.VerifyAccessToken(tok); err == nil {
		t.Errorf("attachment token must not pass as access token")
	}
	// 过期
	expired, _ := s.signToken("msg:42", "attachment", -time.Minute)
	if err := s.VerifyAttachmentAccess(expired, 42); err == nil {
		t.Errorf("expired attachment token must be rejected")
	}
}
