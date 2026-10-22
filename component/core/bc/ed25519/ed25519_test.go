package ed25519

import (
	stded25519 "crypto/ed25519"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
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
