package jwt

import (
	"time"

	"flymail-core/auth"
)

// Claims is an alias for core auth.Claims.
type Claims = auth.Claims

// GenerateToken generates a JWT token with the given user ID and expiration time.
func GenerateToken(userID uint, secret string, expiration time.Duration) (string, error) {
	return auth.GenerateTokenSimple(userID, secret, expiration)
}

// ValidateToken validates a JWT token and returns the claims.
func ValidateToken(tokenString string, secret string) (*Claims, error) {
	return auth.ValidateTokenSimple(tokenString, secret)
}
