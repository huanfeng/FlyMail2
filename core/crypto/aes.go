package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// AESCrypto provides AES-256-GCM encryption and decryption.
type AESCrypto struct {
	key []byte
}

// NewAESCrypto creates a new AESCrypto instance.
// The key is padded or truncated to 32 bytes for AES-256.
func NewAESCrypto(key []byte) (*AESCrypto, error) {
	if len(key) == 0 {
		return nil, errors.New("encryption key must not be empty")
	}

	k := make([]byte, 32)
	if len(key) >= 32 {
		copy(k, key[:32])
	} else {
		copy(k, key)
	}

	return &AESCrypto{key: k}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a base64-encoded string.
// Returns an empty string if plaintext is empty.
func (c *AESCrypto) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded ciphertext using AES-256-GCM.
// Returns an empty string if ciphertext is empty.
func (c *AESCrypto) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
