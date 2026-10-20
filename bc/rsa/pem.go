// Package rsa 提供 RSA PEM 解析（含证书、带密码私钥）与 OAEP 加解密
//
// 签名请使用 bc/sha256withrsa（PKCS#1 v1.5）或 bc/rsapss（PSS）。
//
// @author zwei
package rsa

import (
	"crypto/rand"
	stdrsa "crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"github.com/youmark/pkcs8"

	"gitee.com/hq_project/yomi-boot/errors"
)

// GenerateKey 生成 RSA 私钥；bits 须 >= 2048
func GenerateKey(bits int) (*stdrsa.PrivateKey, error) {
	if bits < 2048 {
		return nil, errors.New("rsa: bits 必须 >= 2048")
	}
	key, err := stdrsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, errors.Wrap(err, "rsa: generate key")
	}
	return key, nil
}

// ParsePrivateKeyPEM 解析 PKCS#1 或 PKCS#8 明文私钥 PEM
func ParsePrivateKeyPEM(pemBytes []byte) (*stdrsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("rsa: 无效的私钥 PEM")
	}
	if isEncryptedPEMBlock(block) {
		return nil, errors.New("rsa: 私钥已加密，请使用 ParseEncryptedPrivateKeyPEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "rsa: parse private key")
	}
	key, ok := parsed.(*stdrsa.PrivateKey)
	if !ok {
		return nil, errors.New("rsa: PEM 不是 RSA 私钥")
	}
	return key, nil
}

// ParseEncryptedPrivateKeyPEM 解析带密码的私钥 PEM：
// PKCS#8 的 ENCRYPTED PRIVATE KEY（PBES2）或 OpenSSL 传统加密的 RSA PRIVATE KEY。
// 传统 PEM 使用 MD5 KDF，仅用于存量兼容；新密钥优先 PKCS#8 PBES2。
func ParseEncryptedPrivateKeyPEM(pemBytes, password []byte) (*stdrsa.PrivateKey, error) {
	if len(password) == 0 {
		return nil, errors.New("rsa: password 不能为空")
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("rsa: 无效的私钥 PEM")
	}
	switch block.Type {
	case "ENCRYPTED PRIVATE KEY":
		key, err := pkcs8.ParsePKCS8PrivateKeyRSA(block.Bytes, password)
		if err != nil {
			return nil, errors.Wrap(err, "rsa: parse encrypted private key")
		}
		return key, nil
	case "RSA PRIVATE KEY":
		if block.Headers["DEK-Info"] == "" {
			return nil, errors.New("rsa: 私钥未加密，请使用 ParsePrivateKeyPEM")
		}
		//nolint:staticcheck // 仅用于读取 OpenSSL 传统加密 PEM，兼容存量密钥
		der, err := x509.DecryptPEMBlock(block, password)
		if err != nil {
			return nil, errors.Wrap(err, "rsa: decrypt private key")
		}
		key, err := x509.ParsePKCS1PrivateKey(der)
		if err != nil {
			return nil, errors.Wrap(err, "rsa: parse private key")
		}
		return key, nil
	default:
		return nil, errors.Newf("rsa: 不支持的加密私钥 PEM 类型 %q", block.Type)
	}
}

// ParseCertificatePEM 解析 X.509 证书 PEM；含多张证书时取第一张，不校验证书链与有效期
func ParseCertificatePEM(pemBytes []byte) (*x509.Certificate, error) {
	certs, err := ParseCertificatesPEM(pemBytes)
	if err != nil {
		return nil, err
	}
	return certs[0], nil
}

// ParseCertificatesPEM 解析 PEM 中的全部 X.509 证书（仅解析，不校验证书链与有效期）
func ParseCertificatesPEM(pemBytes []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := pemBytes
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.Wrap(err, "rsa: parse certificate")
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, errors.New("rsa: PEM 中没有证书")
	}
	return certs, nil
}

// ParsePublicKeyFromCertificatePEM 从第一张 X.509 证书提取 RSA 公钥；多证书请先用 ParseCertificatesPEM 选择
func ParsePublicKeyFromCertificatePEM(pemBytes []byte) (*stdrsa.PublicKey, error) {
	cert, err := ParseCertificatePEM(pemBytes)
	if err != nil {
		return nil, err
	}
	pub, ok := cert.PublicKey.(*stdrsa.PublicKey)
	if !ok {
		return nil, errors.New("rsa: 证书公钥不是 RSA")
	}
	return pub, nil
}

// isEncryptedPEMBlock 判断 PEM 块是否为加密私钥
func isEncryptedPEMBlock(block *pem.Block) bool {
	return block.Type == "ENCRYPTED PRIVATE KEY" || block.Headers["DEK-Info"] != ""
}

// ParsePublicKeyPEM 解析 PKIX 或 PKCS#1 公钥 PEM
func ParsePublicKeyPEM(pemBytes []byte) (*stdrsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("rsa: 无效的公钥 PEM")
	}
	if block.Type == "RSA PUBLIC KEY" {
		pub, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, errors.Wrap(err, "rsa: parse public key")
		}
		return pub, nil
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "rsa: parse public key")
	}
	pub, ok := parsed.(*stdrsa.PublicKey)
	if !ok {
		return nil, errors.New("rsa: PEM 不是 RSA 公钥")
	}
	return pub, nil
}

// MarshalPrivateKeyPEM 将私钥编码为 PKCS#8 PEM（测试/导出辅助）
func MarshalPrivateKeyPEM(key *stdrsa.PrivateKey) ([]byte, error) {
	if key == nil {
		return nil, errors.New("rsa: private key 为 nil")
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, errors.Wrap(err, "rsa: marshal private key")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// MarshalPublicKeyPEM 将公钥编码为 PKIX PEM
func MarshalPublicKeyPEM(pub *stdrsa.PublicKey) ([]byte, error) {
	if pub == nil {
		return nil, errors.New("rsa: public key 为 nil")
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, errors.Wrap(err, "rsa: marshal public key")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}
