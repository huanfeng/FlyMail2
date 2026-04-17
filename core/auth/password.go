package auth

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a password using bcrypt with the default cost.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// VerifyPassword checks if a plaintext password matches the given bcrypt hash.
func VerifyPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
