package ed25519

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestEd25519RejectsBadKeys(t *testing.T) {
	if _, err := ParsePrivateKeyPEM(nil); err == nil {
		t.Fatal("空私钥 PEM 应拒绝")
	}
	if _, err := ParsePublicKeyPEM(nil); err == nil {
		t.Fatal("空公钥 PEM 应拒绝")
	}
	if _, err := ParsePrivateKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("nope")})); err == nil {
		t.Fatal("损坏的私钥应拒绝")
	}
	if _, err := ParsePublicKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("nope")})); err == nil {
		t.Fatal("损坏的公钥应拒绝")
	}
	if _, err := MarshalPrivateKeyPEM([]byte("short")); err == nil {
		t.Fatal("过短私钥应拒绝")
	}
	if _, err := MarshalPublicKeyPEM([]byte("short")); err == nil {
		t.Fatal("过短公钥应拒绝")
	}

	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := x509.MarshalPKCS8PrivateKey(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePrivateKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: priv})); err == nil {
		t.Fatal("ECDSA 私钥不应被当成 Ed25519")
	}
	pub, err := x509.MarshalPKIXPublicKey(&other.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePublicKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})); err == nil {
		t.Fatal("ECDSA 公钥不应被当成 Ed25519")
	}
}
