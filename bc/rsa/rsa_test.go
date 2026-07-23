package rsa

import (
	"bytes"
	"testing"
	"yt-boot/errors"
)

// TestOAEPAndSign 验证 OAEP 加解密及 PSS/PKCS1v15 签名验签
func TestOAEPAndSign(t *testing.T) {
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

	msg := []byte("sign-me")
	sig, err := SignPSS(priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPSS(pub, msg, sig); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPSS(pub, []byte("other"), sig); !errors.Is(err, ErrVerify) {
		t.Fatalf("期望 ErrVerify，实际 %v", err)
	}

	sig2, err := SignPKCS1v15(priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPKCS1v15(pub, msg, sig2); err != nil {
		t.Fatal(err)
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
