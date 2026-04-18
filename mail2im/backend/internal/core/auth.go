package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"mail2im/internal/models"
	"strconv"
	"time"

	coreauth "flymail-core/auth"
	"flymail-core/logger"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

var (
	jwtSecret       []byte
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 14 * 24 * time.Hour
	passwordCharset = []byte("ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789@#$%")
)

type TokenPair struct {
	AccessToken      string    `json:"access_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

func InitAuth(secret string) {
	if secret == "" || secret == "change-me-in-prod" {
		logger.Warn("JWT_SECRET not set, using insecure default. Set MAIL2IM_JWT_SECRET for production.")
		secret = "change-me-in-prod"
	}
	jwtSecret = []byte(secret)
}

func IssueTokenPair(userID uint) (*TokenPair, error) {
	accessToken, accessExp, err := generateAccessToken(userID)
	if err != nil {
		return nil, err
	}

	refreshToken, refreshExp, err := generateRefreshToken(userID)
	if err != nil {
		return nil, err
	}

	if err := saveRefreshToken(userID, refreshToken, refreshExp); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:      accessToken,
		AccessExpiresAt:  accessExp,
		RefreshToken:     refreshToken,
		RefreshExpiresAt: refreshExp,
	}, nil
}

func ValidateAccessToken(tokenStr string) (uint, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		return 0, err
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return 0, errors.New("invalid token")
	}

	subject := claims.Subject
	if subject == "" {
		return 0, errors.New("missing subject")
	}

	uid, err := strconv.ParseUint(subject, 10, 64)
	if err != nil {
		return 0, err
	}

	return uint(uid), nil
}

func ValidateRefreshToken(rawToken string) (*models.RefreshToken, error) {
	hashed := hashToken(rawToken)
	var token models.RefreshToken
	if err := DB.WithContext(context.Background()).Where("token_hash = ?", hashed).First(&token).Error; err != nil {
		return nil, err
	}

	if token.Revoked {
		return nil, errors.New("refresh token revoked")
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, errors.New("refresh token expired")
	}

	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(rawToken, claims, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, errors.New("invalid refresh token signature")
	}

	if claims.Subject != fmt.Sprintf("%d", token.UserID) {
		return nil, errors.New("refresh token user mismatch")
	}

	return &token, nil
}

func RevokeRefreshToken(rawToken string) error {
	hashed := hashToken(rawToken)
	return DB.WithContext(context.Background()).
		Model(&models.RefreshToken{}).
		Where("token_hash = ?", hashed).
		Update("revoked", true).Error
}

func RevokeAllRefreshTokensForUser(userID uint) error {
	return DB.WithContext(context.Background()).
		Model(&models.RefreshToken{}).
		Where("user_id = ?", userID).
		Update("revoked", true).Error
}

func ResetDefaultUserPassword(customPassword string) (*models.User, string, error) {
	var user models.User
	err := DB.WithContext(context.Background()).Order("id asc").First(&user).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", err
		}
		user = models.User{
			Username: "admin",
			Email:    "",
		}
		if err := DB.Create(&user).Error; err != nil {
			return nil, "", err
		}
	}

	var newPwd string
	if customPassword != "" {
		if len(customPassword) < 6 {
			return nil, "", errors.New("password must be at least 6 characters")
		}
		newPwd = customPassword
	} else {
		var err error
		newPwd, err = generateRandomPassword(14)
		if err != nil {
			return nil, "", err
		}
	}

	hash, err := coreauth.HashPassword(newPwd)
	if err != nil {
		return nil, "", err
	}

	if user.Username == "" {
		user.Username = "admin"
	}

	if err := DB.WithContext(context.Background()).
		Model(&user).
		Updates(map[string]interface{}{
			"password_hash": hash,
			"username":      user.Username,
		}).Error; err != nil {
		return nil, "", err
	}

	if err := RevokeAllRefreshTokensForUser(user.ID); err != nil {
		return nil, "", err
	}

	return &user, newPwd, nil
}

// VerifyUserPassword compares the provided plain password with the stored hash of a user.
func VerifyUserPassword(user models.User, password string) error {
	if password == "" {
		return errors.New("password is required")
	}
	if user.PasswordHash == "" {
		return errors.New("user password not set")
	}
	if !coreauth.VerifyPassword(user.PasswordHash, password) {
		return errors.New("invalid password")
	}
	return nil
}

func generateRandomPassword(length int) (string, error) {
	if length <= 0 {
		length = 12
	}
	buf := make([]byte, length)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	for i := 0; i < length; i++ {
		buf[i] = passwordCharset[int(buf[i])%len(passwordCharset)]
	}
	return string(buf), nil
}

func generateAccessToken(userID uint) (string, time.Time, error) {
	expiresAt := time.Now().Add(accessTokenTTL)
	claims := jwt.RegisteredClaims{
		Subject:   fmt.Sprintf("%d", userID),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenStr, expiresAt, nil
}

func generateRefreshToken(userID uint) (string, time.Time, error) {
	expiresAt := time.Now().Add(refreshTokenTTL)
	claims := jwt.RegisteredClaims{
		Subject:   fmt.Sprintf("%d", userID),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ID:        fmt.Sprintf("%d-%d", userID, time.Now().UnixNano()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenStr, expiresAt, nil
}

func saveRefreshToken(userID uint, token string, expiresAt time.Time) error {
	rt := models.RefreshToken{
		UserID:    userID,
		TokenHash: hashToken(token),
		ExpiresAt: expiresAt,
		Revoked:   false,
	}

	return DB.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rt).Error; err != nil {
			return err
		}
		return nil
	})
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
