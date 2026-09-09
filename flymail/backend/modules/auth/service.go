package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"

	coreauth "flymail-core/auth"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type Options struct {
	JWTSecret      string
	AccessTTLMin   int
	RefreshTTLHour int
}

type Service struct {
	repo *Repository
	opts Options
}

func NewService(repo *Repository, opts Options) *Service {
	return &Service{repo: repo, opts: opts}
}

// SetAdminPassword 创建或更新管理员（用于 db init / reset-admin-password）。
func (s *Service) SetAdminPassword(username, password string) error {
	hash, err := coreauth.HashPassword(password)
	if err != nil {
		return err
	}
	existing, err := s.repo.GetByUsername(username)
	if err != nil && !errors.Is(err, ErrAdminNotFound) {
		return err
	}
	if existing == nil {
		return s.repo.Upsert(&AdminUser{Username: username, PasswordHash: hash})
	}
	existing.PasswordHash = hash
	return s.repo.Upsert(existing)
}

// EnsureAdmin 在不存在任何管理员时创建一个（桌面形态首次运行「开箱即用」）。返回是否新建。
func (s *Service) EnsureAdmin(username, password string) (bool, error) {
	n, err := s.repo.Count()
	if err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	if err := s.SetAdminPassword(username, password); err != nil {
		return false, err
	}
	return true, nil
}

// Authenticate 校验用户名密码。
func (s *Service) Authenticate(username, password string) (*AdminUser, error) {
	u, err := s.repo.GetByUsername(username)
	if errors.Is(err, ErrAdminNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !coreauth.VerifyPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

// TokenPair 包含 access token 和 refresh token。
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// Claims 是 JWT 载荷。
type Claims struct {
	Username string `json:"username"`
	Type     string `json:"type"` // "access" | "refresh"
	jwt.RegisteredClaims
}

// Login 校验密码并签发双 token；成功后尽力记录最后登录时间（失败不阻断登录）。
func (s *Service) Login(username, password string) (*TokenPair, error) {
	u, err := s.Authenticate(username, password)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	u.LastLoginAt = &now
	_ = s.repo.Upsert(u)
	return s.issuePair(u.Username)
}

// Profile 返回管理员资料。
func (s *Service) Profile(username string) (*AdminUser, error) {
	return s.repo.GetByUsername(username)
}

// UpdateProfile 更新展示名与联系邮箱（两者均可为空）。
func (s *Service) UpdateProfile(username, displayName, email string) (*AdminUser, error) {
	u, err := s.repo.GetByUsername(username)
	if err != nil {
		return nil, err
	}
	u.DisplayName = strings.TrimSpace(displayName)
	u.Email = strings.TrimSpace(email)
	if err := s.repo.Upsert(u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) issuePair(username string) (*TokenPair, error) {
	access, err := s.signToken(username, "access", time.Duration(s.opts.AccessTTLMin)*time.Minute)
	if err != nil {
		return nil, err
	}
	refresh, err := s.signToken(username, "refresh", time.Duration(s.opts.RefreshTTLHour)*time.Hour)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

func (s *Service) signToken(username, typ string, ttl time.Duration) (string, error) {
	claims := Claims{
		Username: username,
		Type:     typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.opts.JWTSecret))
}

func (s *Service) parseToken(tokenStr string) (*Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(tokenStr, &c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.opts.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// VerifyAccessToken 解析并校验 access token，返回 Claims。
func (s *Service) VerifyAccessToken(tokenStr string) (*Claims, error) {
	c, err := s.parseToken(tokenStr)
	if err != nil {
		return nil, err
	}
	if c.Type != "access" {
		return nil, errors.New("not an access token")
	}
	return c, nil
}

// attachmentTokenTTL 是附件令牌的有效期：够用户读完一封邮件并点开附件，泄露后也只剩这么久。
const attachmentTokenTTL = time.Hour

// IssueAttachmentToken 签发限定单封邮件的附件令牌。
// 内联图（cid:）的 URL 要写进邮件 HTML 所在的 iframe 文档，那是攻击者可控的内容：
// 带上完整 access token 的话，开启远程内容后可用 CSS 属性选择器逐字符外泄。
// 这个令牌只能取这一封的附件、一小时后失效，泄露的代价被压到「拿到本就已经在看的这封邮件的附件」。
func (s *Service) IssueAttachmentToken(messageID uint) (string, error) {
	return s.signToken(fmt.Sprintf("msg:%d", messageID), "attachment", attachmentTokenTTL)
}

// VerifyAttachmentAccess 校验附件端点的令牌：access token，或限定该邮件的附件令牌。
func (s *Service) VerifyAttachmentAccess(tokenStr string, messageID uint) error {
	c, err := s.parseToken(tokenStr)
	if err != nil {
		return err
	}
	switch c.Type {
	case "access":
		return nil
	case "attachment":
		if c.Username != fmt.Sprintf("msg:%d", messageID) {
			return errors.New("attachment token scoped to another message")
		}
		return nil
	}
	return errors.New("token type not accepted")
}

// ChangePassword 验证旧密码后更新为新密码。
func (s *Service) ChangePassword(username, oldPassword, newPassword string) error {
	if _, err := s.Authenticate(username, oldPassword); err != nil {
		return err // ErrInvalidCredentials
	}
	return s.SetAdminPassword(username, newPassword)
}

// Refresh 用 refresh token 签发新的 token pair。
func (s *Service) Refresh(refreshToken string) (*TokenPair, error) {
	c, err := s.parseToken(refreshToken)
	if err != nil {
		return nil, err
	}
	if c.Type != "refresh" {
		return nil, errors.New("not a refresh token")
	}
	return s.issuePair(c.Username)
}
