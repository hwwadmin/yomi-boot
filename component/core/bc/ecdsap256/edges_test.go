package ecdsap256

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestP256RejectsBadKeys(t *testing.T) {
	if _, err := MarshalPublicKeyPEM(nil); err == nil {
		t.Fatal("nil 公钥应拒绝")
	}
	if _, err := ParsePrivateKeyPEM(nil); err == nil {
		t.Fatal("空私钥 PEM 应拒绝")
	}
	if _, err := ParsePublicKeyPEM(nil); err == nil {
		t.Fatal("空公钥 PEM 应拒绝")
	}
	garbage := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("nope")})
	if _, err := ParsePrivateKeyPEM(garbage); err == nil {
		t.Fatal("损坏的私钥 PEM 应拒绝")
	}
	if _, err := ParsePublicKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("nope")})); err == nil {
		t.Fatal("损坏的公钥 PEM 应拒绝")
	}

	other, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePrivateKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})); err == nil {
		t.Fatal("P-384 PKCS8 不应被当成 P-256")
	}
	sec1, err := x509.MarshalECPrivateKey(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePrivateKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1})); err == nil {
		t.Fatal("P-384 SEC1 不应被当成 P-256")
	}
	pub, err := x509.MarshalPKIXPublicKey(&other.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePublicKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})); err == nil {
		t.Fatal("P-384 公钥不应被当成 P-256")
	}
}
