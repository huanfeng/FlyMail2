package crypto

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	e, err := New("a-test-encryption-key-32bytes!!")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	enc, err := e.Encrypt("s3cr3t-password")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == "s3cr3t-password" {
		t.Error("密文不应等于明文")
	}
	dec, err := e.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != "s3cr3t-password" {
		t.Errorf("解密 = %q, want s3cr3t-password", dec)
	}
}

func TestEncryptEmpty(t *testing.T) {
	e, _ := New("a-test-encryption-key-32bytes!!")
	enc, err := e.Encrypt("")
	if err != nil || enc != "" {
		t.Errorf("空串加密应返回空串无错，得到 %q err=%v", enc, err)
	}
}
