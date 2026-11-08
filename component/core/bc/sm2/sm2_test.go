package sm2

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/emmansun/gmsm/smx509"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

// TestSignVerify 验证默认 UID 与自定义 UID 的签名验签及篡改检测
func TestSignVerify(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey
	msg := []byte("sm2-sign-me")

	sig, err := Sign(priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, msg, sig); err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, []byte("other"), sig); !errors.Is(err, ErrVerify) {
		t.Fatalf("期望 ErrVerify，实际 %v", err)
	}

	uid := []byte("example@test.com")
	uidSig, err := SignWithUID(priv, uid, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyWithUID(pub, uid, msg, uidSig); err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, msg, uidSig); !errors.Is(err, ErrVerify) {
		t.Fatalf("UID 不一致应失败，实际 %v", err)
	}
}

// TestEncryptDecrypt 验证 C1C3C2 与 ASN.1 两种密文格式的加解密及错误密钥检测
func TestEncryptDecrypt(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey
	plain := []byte("sm2-encrypt-hello")

	ct, err := Encrypt(pub, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(priv, ct)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("plain roundtrip got %q err=%v", got, err)
	}

	asn1CT, err := EncryptASN1(pub, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err = Decrypt(priv, asn1CT)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("asn1 roundtrip got %q err=%v", got, err)
	}

	other, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(other, ct); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("期望 ErrDecrypt，实际 %v", err)
	}
}

// TestInvalidKeys 验证零值、缺坐标与非 SM2 曲线密钥返回错误而非 panic
func TestInvalidKeys(t *testing.T) {
	if _, err := Sign(&PrivateKey{}, []byte("m")); err == nil {
		t.Fatal("零值私钥签名应失败")
	}
	if err := Verify(&PublicKey{}, []byte("m"), []byte("sig")); err == nil {
		t.Fatal("零值公钥验签应失败")
	}
	if _, err := Encrypt(&PublicKey{}, []byte("m")); err == nil {
		t.Fatal("零值公钥加密应失败")
	}
	if _, err := Decrypt(&PrivateKey{}, make([]byte, 128)); err == nil {
		t.Fatal("零值私钥解密应失败")
	}
	if _, err := MarshalPrivateKeyPEM(&PrivateKey{}); err == nil {
		t.Fatal("零值私钥导出应失败")
	}

	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	badPub := &PublicKey{Curve: priv.Curve}
	if _, err := Encrypt(badPub, []byte("m")); err == nil {
		t.Fatal("缺坐标公钥加密应失败")
	}
	if err := Verify(badPub, []byte("m"), []byte("sig")); err == nil {
		t.Fatal("缺坐标公钥验签应失败")
	}

	p256, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	wrong := &PrivateKey{PrivateKey: *p256}
	if _, err := Sign(wrong, []byte("m")); err == nil {
		t.Fatal("非 SM2 私钥签名应失败")
	}
	if _, err := MarshalPrivateKeyPEM(wrong); err == nil {
		t.Fatal("非 SM2 私钥导出应失败")
	}
	if _, err := Encrypt(&p256.PublicKey, []byte("m")); err == nil {
		t.Fatal("非 SM2 公钥加密应失败")
	}
}

// TestPEMRoundTrip 验证 PKCS#8 / PKIX PEM 序列化与解析后的签名验签
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

	msg := []byte("sm2-pem")
	sig, err := Sign(parsedPriv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(parsedPub, msg, sig); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePrivateKeyPEM([]byte("not a pem")); err == nil {
		t.Fatal("非法 PEM 应失败")
	}
}

func TestEncrypt_RejectsEmptyPlain(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Encrypt(&priv.PublicKey, nil); err == nil {
		t.Fatal("空明文应失败")
	}
}

func TestSign_RejectsInvalidScalar(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	priv.D = nil
	if _, err := Sign(priv, []byte("m")); err == nil {
		t.Fatal("标量为空的私钥应失败")
	}
}

func TestParsePrivateKeyPEM_RejectsNonSM2SEC1(t *testing.T) {
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(ec)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	if _, err := ParsePrivateKeyPEM(pemBytes); err == nil {
		t.Fatal("P-256 SEC1 私钥不应被当成 SM2")
	}
}

func TestParsePrivateKeyPEM_SEC1RoundTrip(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	der, err := smx509.MarshalSM2PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	parsed, err := ParsePrivateKeyPEM(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.D.Cmp(priv.D) != 0 {
		t.Fatal("SEC1 私钥解析结果不一致")
	}
}
