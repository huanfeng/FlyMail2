package auth

import (
	"errors"

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
