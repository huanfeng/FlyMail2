package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mail2im/internal/models"
	"time"
)

func GenerateOneTimeToken(emailID string, expiration time.Duration) (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)

	ott := models.OneTimeToken{
		Token:     token,
		EmailID:   emailID,
		ExpiresAt: time.Now().Add(expiration),
		Used:      false,
	}

	if err := DB.Create(&ott).Error; err != nil {
		return "", err
	}

	return token, nil
}

func RedeemOneTimeToken(tokenStr string) (*models.Email, error) {
	var ott models.OneTimeToken
	if err := DB.Where("token = ? AND used = ? AND expires_at > ?", tokenStr, false, time.Now()).First(&ott).Error; err != nil {
		return nil, fmt.Errorf("invalid or expired token")
	}

	// Invalidate token immediately (one-time use)
	// Or should we allow multiple reads for a short duration?
	// "OneTime" implies once. But for web loading (HTML + images?), usually the token is for the page load.
	// Let's stick to strict one-time for now, or rename to TemporaryToken.
	// User requirement said "Temporary Token", but I named it OneTimeToken.
	// For better UX (reload page), let's NOT mark it used immediately, but rely on Expiration.
	// If strict one-time is needed, uncomment below:
	// DB.Model(&ott).Update("used", true)

	var email models.Email
	if err := DB.First(&email, ott.EmailID).Error; err != nil {
		return nil, err
	}

	// Mark as read asynchronously
	go func() {
		if Watcher != nil {
			_ = Watcher.MarkAsRead(email.AccountID, email.UID)
		}
	}()

	return &email, nil
}
