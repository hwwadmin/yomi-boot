// Package ed25519 提供 Ed25519（RFC 8032）签名验签与 PEM 密钥管理
//
// 签名输出 64 字节原始签名，对应 JWT EdDSA。
//
// @author zwei
package ed25519

import (
	stded25519 "crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"

	"gitee.com/hq_project/yomi-boot/errors"
)

// PrivateKey 是 Ed25519 私钥类型别名
type PrivateKey = stded25519.PrivateKey

// PublicKey 是 Ed25519 公钥类型别名
type PublicKey = stded25519.PublicKey

// GenerateKey 生成 Ed25519 密钥对
func GenerateKey() (PublicKey, PrivateKey, error) {
	pub, priv, err := stded25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, errors.Wrap(err, "ed25519: generate key")
	}
	return pub, priv, nil
}

// ParsePrivateKeyPEM 解析 PKCS#8 私钥 PEM
func ParsePrivateKeyPEM(pemBytes []byte) (PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("ed25519: 无效的私钥 PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "ed25519: parse private key")
	}
	key, ok := parsed.(PrivateKey)
	if !ok || len(key) != stded25519.PrivateKeySize {
		return nil, errors.New("ed25519: PEM 不是 Ed25519 私钥")
	}
	return key, nil
}

// ParsePublicKeyPEM 解析 PKIX 公钥 PEM
func ParsePublicKeyPEM(pemBytes []byte) (PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("ed25519: 无效的公钥 PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "ed25519: parse public key")
	}
	pub, ok := parsed.(PublicKey)
	if !ok || len(pub) != stded25519.PublicKeySize {
		return nil, errors.New("ed25519: PEM 不是 Ed25519 公钥")
	}
	return pub, nil
}

// MarshalPrivateKeyPEM 将私钥编码为 PKCS#8 PEM
func MarshalPrivateKeyPEM(key PrivateKey) ([]byte, error) {
	if len(key) != stded25519.PrivateKeySize {
		return nil, errors.New("ed25519: private key 长度非法")
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, errors.Wrap(err, "ed25519: marshal private key")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// MarshalPublicKeyPEM 将公钥编码为 PKIX PEM
func MarshalPublicKeyPEM(pub PublicKey) ([]byte, error) {
	if len(pub) != stded25519.PublicKeySize {
		return nil, errors.New("ed25519: public key 长度非法")
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, errors.Wrap(err, "ed25519: marshal public key")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}
