package rsa

import (
	"bytes"
	"testing"
)

// TestOAEP 验证 OAEP 加解密
func TestOAEP(t *testing.T) {
	priv, err := GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey

	plain := []byte("rsa-oaep-hello")
	ct, err := EncryptOAEP(pub, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptOAEP(priv, ct)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("oaep got %q err=%v", got, err)
	}
}

// TestPEMRoundTrip 验证 PEM 密钥序列化与解析后的加解密
func TestPEMRoundTrip(t *testing.T) {
	priv, err := GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}
	privPEM, err := MarshalPrivateKeyPEM(priv)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM, err := MarshalPublicKeyPEM(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	parsedPriv, err := ParsePrivateKeyPEM(privPEM)
	if err != nil {
		t.Fatal(err)
	}
	parsedPub, err := ParsePublicKeyPEM(pubPEM)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("pem")
	ct, err := EncryptOAEP(parsedPub, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptOAEP(parsedPriv, ct)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("pem roundtrip failed: %q %v", got, err)
	}
}
