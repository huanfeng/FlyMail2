package auth_test

import "testing"

func TestIssueAndVerifyToken(t *testing.T) {
	s := newTestService(t)
	if err := s.SetAdminPassword("admin", "p@ssw0rd"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}
	pair, err := s.Login("admin", "p@ssw0rd")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("token 不应为空")
	}
	claims, err := s.VerifyAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("VerifyAccessToken: %v", err)
	}
	if claims.Username != "admin" {
		t.Errorf("claims.Username = %q", claims.Username)
	}
	refreshed, err := s.Refresh(pair.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if refreshed.AccessToken == "" {
		t.Error("刷新后 access token 不应为空")
	}
}
