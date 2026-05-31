package crypto

import corecrypto "flymail-core/crypto"

// Encryptor 封装 core 的 AES-256-GCM 加解密，用于账户凭证落库加密。
type Encryptor struct {
	aes *corecrypto.AESCrypto
}

// New 用给定密钥创建 Encryptor（密钥会被补足/截断到 32 字节）。
func New(key string) (*Encryptor, error) {
	aes, err := corecrypto.NewAESCrypto([]byte(key))
	if err != nil {
		return nil, err
	}
	return &Encryptor{aes: aes}, nil
}

// Encrypt 加密明文，空串返回空串。
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	return e.aes.Encrypt(plaintext)
}

// Decrypt 解密密文，空串返回空串。
func (e *Encryptor) Decrypt(ciphertext string) (string, error) {
	return e.aes.Decrypt(ciphertext)
}
