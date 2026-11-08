package sha256withrsa

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

// TestSignVerify 验证签名验签、标准库兼容性及篡改检测
func TestSignVerify(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey
	msg := []byte("sha256withrsa")

	sig, err := Sign(priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, msg, sig); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(msg)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("签名与标准库不兼容: %v", err)
	}
	if err := Verify(pub, []byte("other"), sig); !errors.Is(err, ErrVerify) {
		t.Fatalf("期望 ErrVerify，实际 %v", err)
	}
	if _, err := Sign(nil, msg); err == nil {
		t.Fatal("nil 私钥应失败")
	}
}

func TestSign_RejectsIncompleteKey(t *testing.T) {
	priv := &rsa.PrivateKey{
		PublicKey: rsa.PublicKey{N: new(big.Int).Lsh(big.NewInt(1), 2048), E: 65537},
		D:         big.NewInt(1),
	}
	if _, err := Sign(priv, []byte("m")); err == nil {
		t.Fatal("缺少素数的私钥签名应失败")
	}
}

func TestPKCS1RejectsNilAndBadSignature(t *testing.T) {
	if _, err := Sign(nil, []byte("m")); err == nil {
		t.Fatal("nil 私钥应拒绝")
	}
	if err := Verify(nil, []byte("m"), nil); err == nil {
		t.Fatal("nil 公钥应拒绝")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(&key.PublicKey, []byte("m"), []byte("bad")); err == nil {
		t.Fatal("损坏的签名应失败")
	}
}
