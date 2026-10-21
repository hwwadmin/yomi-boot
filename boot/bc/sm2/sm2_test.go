package sm2

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"gitee.com/hq_project/yomi-boot/boot/errors"
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
