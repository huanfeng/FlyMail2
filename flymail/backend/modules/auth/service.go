// TODO: migrate to flymail-core/auth for JWT
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"flymail/pkg/jwt"
	"flymail/shared/config"
)

// Service interface for authentication operations
type Service interface {
	Login(ctx context.Context, username, password string) (*AuthResponse, error)
	RefreshToken(ctx context.Context, refreshToken string) (*AuthResponse, error)
	ValidateToken(ctx context.Context, token string) (uint, error)
	GetUser(ctx context.Context, userID uint) (*User, error)
	UpdatePassword(ctx context.Context, userID uint, oldPassword, newPassword string) error
	UpdateAdminCredentials(ctx context.Context, adminUserID uint, newUsername, newEmail, newPassword, oldPassword string) error
}

// service implements Service interface
type service struct {
	repo   Repository
	config *config.Config
}

// NewService creates a new auth service
func NewService(repo Repository, config *config.Config) Service {
	return &service{
		repo:   repo,
		config: config,
	}
}

// Login authenticates a user and returns tokens
func (s *service) Login(ctx context.Context, username, password string) (*AuthResponse, error) {
	// Get user by username
	user, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Generate tokens
	accessToken, err := jwt.GenerateToken(user.UserID, s.config.Auth.JWTSecret, time.Duration(s.config.Auth.JWTExpiration)*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := jwt.GenerateToken(user.UserID, s.config.Auth.JWTSecret, 7*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Update last login
	now := time.Now()
	user.LastLogin = &now
	if err := s.repo.Update(ctx, user); err != nil {
		// Log error but don't fail login
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.config.Auth.JWTExpiration,
		TokenType:    "Bearer",
	}, nil
}

// RefreshToken refreshes the access token using a refresh token
func (s *service) RefreshToken(ctx context.Context, refreshToken string) (*AuthResponse, error) {
	// Validate refresh token
	claims, err := jwt.ValidateToken(refreshToken, s.config.Auth.JWTSecret)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	// Get user to ensure they still exist
	user, err := s.repo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Generate new tokens
	accessToken, err := jwt.GenerateToken(user.UserID, s.config.Auth.JWTSecret, time.Duration(s.config.Auth.JWTExpiration)*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	newRefreshToken, err := jwt.GenerateToken(user.UserID, s.config.Auth.JWTSecret, time.Duration(s.config.Auth.JWTRefreshExpirationHours)*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    s.config.Auth.JWTExpiration,
		TokenType:    "Bearer",
	}, nil
}

// ValidateToken validates an access token and returns the user ID
func (s *service) ValidateToken(ctx context.Context, token string) (uint, error) {
	claims, err := jwt.ValidateToken(token, s.config.Auth.JWTSecret)
	if err != nil {
		return 0, err
	}

	// Check if user still exists
	if _, err := s.repo.GetByID(ctx, claims.UserID); err != nil {
		return 0, errors.New("user not found")
	}

	return claims.UserID, nil
}

// GetUser retrieves user information by ID
func (s *service) GetUser(ctx context.Context, userID uint) (*User, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// UpdatePassword updates the user's password
func (s *service) UpdatePassword(ctx context.Context, userID uint, oldPassword, newPassword string) error {
	// Get user
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return errors.New("invalid old password")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	if err := s.repo.UpdatePassword(ctx, userID, string(hashedPassword)); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

// UpdateAdminCredentials updates admin user's username, email and password
func (s *service) UpdateAdminCredentials(ctx context.Context, adminUserID uint, newUsername, newEmail, newPassword, oldPassword string) error {
	// Get current user to verify it's admin
	user, err := s.repo.GetByID(ctx, adminUserID)
	if err != nil {
		return fmt.Errorf("admin user not found: %w", err)
	}

	// Verify user is admin
	if !user.IsAdmin {
		return errors.New("permission denied: user is not admin")
	}

	// If changing password, verify old password
	if newPassword != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
			return errors.New("incorrect old password")
		}
	}

	// Check if new username is already taken (if changing username)
	if newUsername != "" && newUsername != user.Username {
		existingUser, err := s.repo.GetByUsername(ctx, newUsername)
		if err == nil && existingUser.UserID != adminUserID {
			return errors.New("username already exists")
		}
		user.Username = newUsername
	}

	// Update email if provided
	if newEmail != "" && newEmail != user.Email {
		user.Email = newEmail
	}

	// Hash new password if provided
	if newPassword != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}
		user.Password = string(hashedPassword)
	}

	// Update user
	if err := s.repo.Update(ctx, user); err != nil {
		return fmt.Errorf("failed to update admin credentials: %w", err)
	}

	return nil
}
