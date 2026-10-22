package rsa

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestParsePKCS1PublicKeyPEM 验证 PKCS#1 RSA 公钥 PEM 解析
func TestParsePKCS1PublicKeyPEM(t *testing.T) {
	priv, err := GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}
	der := x509.MarshalPKCS1PublicKey(&priv.PublicKey)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: der})
	pub, err := ParsePublicKeyPEM(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	if pub.N.Cmp(priv.N) != 0 || pub.E != priv.E {
		t.Fatal("PKCS#1 公钥解析结果不匹配")
	}
}

// TestParseCertificatePEM 验证单证书、证书链解析及公钥提取
func TestParseCertificatePEM(t *testing.T) {
	priv, err := GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(20261020),
		Subject:      pkix.Name{CommonName: "yomi-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	cert, err := ParseCertificatePEM(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	if cert.SerialNumber.Int64() != 20261020 {
		t.Fatalf("序列号不匹配: %s", cert.SerialNumber)
	}
	pub, err := ParsePublicKeyFromCertificatePEM(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	if pub.N.Cmp(priv.N) != 0 || pub.E != priv.E {
		t.Fatal("证书公钥与私钥不匹配")
	}

	certs, err := ParseCertificatesPEM(append(append([]byte{}, certPEM...), certPEM...))
	if err != nil || len(certs) != 2 {
		t.Fatalf("证书链解析失败: len=%d err=%v", len(certs), err)
	}
	if _, err := ParseCertificatePEM([]byte("not a pem")); err == nil {
		t.Fatal("非法 PEM 应失败")
	}
}

// TestParseEncryptedPrivateKeyPEM 使用 OpenSSL 生成的密文验证两种加密私钥格式
func TestParseEncryptedPrivateKeyPEM(t *testing.T) {
	password := []byte("yomi-test")
	for _, name := range []string{"encrypted_pkcs8.pem", "encrypted_legacy.pem"} {
		t.Run(name, func(t *testing.T) {
			pemBytes, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			key, err := ParseEncryptedPrivateKeyPEM(pemBytes, password)
			if err != nil {
				t.Fatal(err)
			}
			if err := key.Validate(); err != nil {
				t.Fatalf("私钥非法: %v", err)
			}
			if key.N.BitLen() != 1024 {
				t.Fatalf("密钥长度不匹配: %d", key.N.BitLen())
			}
			if _, err := ParseEncryptedPrivateKeyPEM(pemBytes, []byte("wrong")); err == nil {
				t.Fatal("错误密码应失败")
			}
			if _, err := ParsePrivateKeyPEM(pemBytes); err == nil {
				t.Fatal("加密私钥应提示使用带密码解析")
			}
		})
	}
}

// TestParseEncryptedPrivateKeyRequiresEncrypted 验证明文或空密码输入被拒绝
func TestParseEncryptedPrivateKeyRequiresEncrypted(t *testing.T) {
	priv, err := GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes, err := MarshalPrivateKeyPEM(priv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseEncryptedPrivateKeyPEM(pemBytes, []byte("x")); err == nil {
		t.Fatal("明文私钥应失败")
	}
	if _, err := ParseEncryptedPrivateKeyPEM(pemBytes, nil); err == nil {
		t.Fatal("空密码应失败")
	}
}
