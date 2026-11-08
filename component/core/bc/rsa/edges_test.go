package rsa

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestRSARejectsBadInputs(t *testing.T) {
	if _, err := GenerateKey(1024); err == nil {
		t.Fatal("低于 2048 位应拒绝")
	}
	if _, err := ParsePrivateKeyPEM(nil); err == nil {
		t.Fatal("空私钥 PEM 应拒绝")
	}
	if _, err := ParsePublicKeyPEM(nil); err == nil {
		t.Fatal("空公钥 PEM 应拒绝")
	}
	if _, err := ParsePrivateKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: []byte{1}})); err == nil {
		t.Fatal("加密私钥应提示改用解密解析")
	}
	if _, err := ParseEncryptedPrivateKeyPEM([]byte("not pem"), nil); err == nil {
		t.Fatal("空密码应拒绝")
	}
	if _, err := MarshalPrivateKeyPEM(nil); err == nil {
		t.Fatal("nil 私钥应拒绝")
	}
	if _, err := MarshalPublicKeyPEM(nil); err == nil {
		t.Fatal("nil 公钥应拒绝")
	}
	if _, err := EncryptOAEP(nil, []byte("a")); err == nil {
		t.Fatal("nil 公钥不能加密")
	}
	if _, err := DecryptOAEP(nil, []byte("a")); err == nil {
		t.Fatal("nil 私钥不能解密")
	}

	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := x509.MarshalPKCS8PrivateKey(other)
	if err != nil {
		t.Fatal(err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: priv})
	if _, err := ParsePrivateKeyPEM(privPEM); err == nil {
		t.Fatal("ECDSA 私钥不应被当成 RSA")
	}
	if _, err := ParseEncryptedPrivateKeyPEM(privPEM, []byte("pw")); err == nil {
		t.Fatal("明文私钥不应走加密解析")
	}
	pub, err := x509.MarshalPKIXPublicKey(&other.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePublicKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})); err == nil {
		t.Fatal("ECDSA 公钥不应被当成 RSA")
	}
	if _, err := ParsePublicKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: []byte{1}})); err == nil {
		t.Fatal("损坏的 PKCS1 公钥应拒绝")
	}
	if _, err := ParsePublicKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte{1}})); err == nil {
		t.Fatal("损坏的 PKIX 公钥应拒绝")
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &other.PublicKey, other)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	mixed := append(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: priv}), certPEM...)
	certs, err := ParseCertificatesPEM(mixed)
	if err != nil || len(certs) != 1 {
		t.Fatalf("应跳过私钥块并解析证书: %v %d", err, len(certs))
	}
	if _, err := ParsePublicKeyFromCertificatePEM(certPEM); err == nil {
		t.Fatal("ECDSA 证书公钥不应被当成 RSA")
	}
	if _, err := ParseCertificatesPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad")})); err == nil {
		t.Fatal("损坏的证书应拒绝")
	}
	if _, err := ParseCertificatesPEM(nil); err == nil {
		t.Fatal("没有证书应拒绝")
	}

	key, err := GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EncryptOAEP(&key.PublicKey, make([]byte, 512)); err == nil {
		t.Fatal("超长明文应失败")
	}
	if _, err := DecryptOAEP(key, []byte("not-ciphertext")); err == nil {
		t.Fatal("损坏的密文应失败")
	}
}
