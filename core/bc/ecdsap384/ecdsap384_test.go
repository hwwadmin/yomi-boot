package ecdsap384

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"gitee.com/hq_project/yomi-boot/core/errors"
)

// TestSignVerify 验证签名验签、标准库兼容性、篡改检测与曲线校验
func TestSignVerify(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey
	msg := []byte("ecdsap384")

	sig, err := Sign(priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, msg, sig); err != nil {
		t.Fatal(err)
	}

	sum := sha512.Sum384(msg)
	if !ecdsa.VerifyASN1(pub, sum[:], sig) {
		t.Fatal("签名与标准库不兼容")
	}
	if err := Verify(pub, []byte("other"), sig); !errors.Is(err, ErrVerify) {
		t.Fatalf("期望 ErrVerify，实际 %v", err)
	}
	if _, err := Sign(nil, msg); err == nil {
		t.Fatal("nil 私钥应失败")
	}

	wrong, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sign(wrong, msg); err == nil {
		t.Fatal("非 P-384 私钥应失败")
	}
	if err := Verify(&wrong.PublicKey, msg, sig); err == nil {
		t.Fatal("非 P-384 公钥应失败")
	}
}

// TestInvalidKey 验证零值、缺公钥坐标的密钥返回错误而非 panic
func TestInvalidKey(t *testing.T) {
	zero := &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: elliptic.P384()}}
	if _, err := Sign(zero, []byte("m")); err == nil {
		t.Fatal("零值私钥应失败")
	}

	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	noPub := *priv
	noPub.X = nil
	noPub.Y = nil
	if _, err := Sign(&noPub, []byte("m")); err == nil {
		t.Fatal("缺公钥坐标私钥签名应失败")
	}
	if _, err := MarshalPrivateKeyPEM(&noPub); err == nil {
		t.Fatal("缺公钥坐标私钥导出应失败")
	}
	badPub := &ecdsa.PublicKey{Curve: priv.Curve}
	if _, err := MarshalPublicKeyPEM(badPub); err == nil {
		t.Fatal("缺坐标公钥导出应失败")
	}
	if err := Verify(badPub, []byte("m"), []byte("sig")); err == nil {
		t.Fatal("缺坐标公钥验签应失败")
	}
}

// TestPEMRoundTrip 验证 PKCS#8 / SEC1 私钥与 PKIX 公钥 PEM 往返
func TestPEMRoundTrip(t *testing.T) {
	priv, err := GenerateKey()
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

	msg := []byte("ecdsap384-pem")
	sig, err := Sign(parsedPriv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(parsedPub, msg, sig); err != nil {
		t.Fatal(err)
	}

	sec1, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	sec1PEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1})
	parsedSEC1, err := ParsePrivateKeyPEM(sec1PEM)
	if err != nil {
		t.Fatal(err)
	}
	if parsedSEC1.D.Cmp(priv.D) != 0 {
		t.Fatal("SEC1 私钥解析结果不一致")
	}
}
