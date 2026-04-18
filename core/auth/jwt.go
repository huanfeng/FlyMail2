package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims with user information.
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username,omitempty"`
	jwt.RegisteredClaims
}

// JWTManager handles JWT token generation and validation.
type JWTManager struct {
	secret        []byte
	expireSeconds int
	issuer        string
}

// NewJWTManager creates a new JWTManager with the given secret and expiration in seconds.
func NewJWTManager(secret string, expireSeconds int) *JWTManager {
	return &JWTManager{
		secret:        []byte(secret),
		expireSeconds: expireSeconds,
		issuer:        "maildev",
	}
}

// SetIssuer sets the token issuer field.
func (m *JWTManager) SetIssuer(issuer string) {
	m.issuer = issuer
}

// GenerateToken generates a signed JWT token for the given user.
func (m *JWTManager) GenerateToken(userID uint, username string) (string, error) {
	claims := &Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(m.expireSeconds) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    m.issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// ValidateToken parses and validates a JWT token string, returning the claims if valid.
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	return validateTokenWithSecret(tokenString, m.secret)
}

// --- Package-level convenience functions ---

// GenerateTokenSimple generates a JWT with just a userID (no username).
// Matches the signature pattern used by flymail.
func GenerateTokenSimple(userID uint, secret string, expiration time.Duration) (string, error) {
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "maildev",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateTokenSimple validates a JWT and returns claims.
// Matches the signature pattern used by flymail.
func ValidateTokenSimple(tokenString string, secret string) (*Claims, error) {
	return validateTokenWithSecret(tokenString, []byte(secret))
}

func validateTokenWithSecret(tokenString string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}
