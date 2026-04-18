package core

import (
	"flymail-core/crypto"
)

var defaultCrypto *crypto.AESCrypto

func InitCrypto(appSecret string) {
	if appSecret == "" {
		appSecret = "mail2im-default-secret-key-32bytes"
	}
	var err error
	defaultCrypto, err = crypto.NewAESCrypto([]byte(appSecret))
	if err != nil {
		panic("failed to initialize crypto: " + err.Error())
	}
}

func Encrypt(text string) (string, error) { return defaultCrypto.Encrypt(text) }
func Decrypt(text string) (string, error) { return defaultCrypto.Decrypt(text) }
