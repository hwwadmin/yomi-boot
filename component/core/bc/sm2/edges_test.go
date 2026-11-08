package sm2

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	gmsm "github.com/emmansun/gmsm/sm2"
)

func TestSM2RejectsBadInputs(t *testing.T) {
	if _, err := MarshalPrivateKeyPEM(nil); err == nil {
		t.Fatal("nil 私钥不能编码")
	}
	if _, err := Sign(nil, []byte("m")); err == nil {
		t.Fatal("nil 私钥不能签名")
	}
	if _, err := MarshalPublicKeyPEM(nil); err == nil {
		t.Fatal("nil 公钥不能编码")
	}
	if _, err := Encrypt(nil, []byte("m")); err == nil {
		t.Fatal("nil 公钥不能加密")
	}
	if _, err := Decrypt(&PrivateKey{}, []byte{1}); err == nil {
		t.Fatal("非 SM2 私钥应拒绝")
	}
	if _, err := Encrypt(&PublicKey{Curve: elliptic.P256()}, nil); err == nil {
		t.Fatal("空明文应拒绝")
	}
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(key, nil); err == nil {
		t.Fatal("空密文应拒绝")
	}

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

	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := x509.MarshalPKCS8PrivateKey(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePrivateKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: priv})); err == nil {
		t.Fatal("P-256 私钥不应被当成 SM2")
	}
	pub, err := x509.MarshalPKIXPublicKey(&other.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePublicKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub})); err == nil {
		t.Fatal("P-256 公钥不应被当成 SM2")
	}
	if _, err := MarshalPrivateKeyPEM(&gmsm.PrivateKey{PrivateKey: *other}); err == nil {
		t.Fatal("P-256 私钥不能按 SM2 编码")
	}
}
