package core

import (
	"flymail-core/crypto"
	"os"
)

var defaultCrypto *crypto.AESCrypto

func init() {
	key := os.Getenv("APP_SECRET")
	if key == "" {
		key = "mail2im-default-secret-key-32bytes" // 32 bytes for AES-256
	}
	var err error
	defaultCrypto, err = crypto.NewAESCrypto([]byte(key))
	if err != nil {
		panic("failed to initialize crypto: " + err.Error())
	}
}

func Encrypt(text string) (string, error) { return defaultCrypto.Encrypt(text) }
func Decrypt(text string) (string, error) { return defaultCrypto.Decrypt(text) }
