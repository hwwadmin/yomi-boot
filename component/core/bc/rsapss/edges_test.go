package rsapss

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
)

func TestPSSRejectsNilAndBadSignature(t *testing.T) {
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
