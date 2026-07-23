package rsa

import (
	"crypto/rand"
	stdrsa "crypto/rsa"
	"crypto/x509"
	"encoding/pem"

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

// ParsePrivateKeyPEM 解析 PKCS#1 或 PKCS#8 私钥 PEM
func ParsePrivateKeyPEM(pemBytes []byte) (*stdrsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("rsa: 无效的私钥 PEM")
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

// ParsePublicKeyPEM 解析 PKIX 公钥 PEM
func ParsePublicKeyPEM(pemBytes []byte) (*stdrsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("rsa: 无效的公钥 PEM")
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
