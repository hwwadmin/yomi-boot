package ed25519

import (
	"crypto/ecdsa"
	stded25519 "crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

// TestSignVerify 验证签名验签、标准库兼容性、篡改检测与密钥长度校验
func TestSignVerify(t *testing.T) {
	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("ed25519")

	sig, err := Sign(priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, msg, sig); err != nil {
		t.Fatal(err)
	}
	if !stded25519.Verify(pub, msg, sig) {
		t.Fatal("签名与标准库不兼容")
	}
	if err := Verify(pub, []byte("other"), sig); !errors.Is(err, ErrVerify) {
		t.Fatalf("期望 ErrVerify，实际 %v", err)
	}
	if _, err := Sign(priv[:32], msg); err == nil {
		t.Fatal("非法私钥长度应失败")
	}
	if err := Verify(pub[:16], msg, sig); err == nil {
		t.Fatal("非法公钥长度应失败")
	}
}

// TestPEMRoundTrip 验证 PKCS#8 私钥与 PKIX 公钥 PEM 往返
func TestPEMRoundTrip(t *testing.T) {
	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	privPEM, err := MarshalPrivateKeyPEM(priv)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM, err := MarshalPublicKeyPEM(pub)
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

	msg := []byte("ed25519-pem")
	sig, err := Sign(parsedPriv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(parsedPub, msg, sig); err != nil {
		t.Fatal(err)
	}
}

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
