package rsapss

import (
	"crypto"
	"crypto/rand"
	stdrsa "crypto/rsa"
	"crypto/sha256"
	"testing"

	"gitee.com/hq_project/yomi-boot/errors"
)

// TestSignVerify 验证签名验签、标准库兼容性及篡改检测
func TestSignVerify(t *testing.T) {
	priv, err := stdrsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey
	msg := []byte("rsapss")

	sig, err := Sign(priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, msg, sig); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(msg)
	if err := stdrsa.VerifyPSS(pub, crypto.SHA256, sum[:], sig, &stdrsa.PSSOptions{
		SaltLength: stdrsa.PSSSaltLengthEqualsHash,
	}); err != nil {
		t.Fatalf("签名与标准库不兼容: %v", err)
	}
	if err := Verify(pub, []byte("other"), sig); !errors.Is(err, ErrVerify) {
		t.Fatalf("期望 ErrVerify，实际 %v", err)
	}
	if _, err := Sign(nil, msg); err == nil {
		t.Fatal("nil 私钥应失败")
	}
}
