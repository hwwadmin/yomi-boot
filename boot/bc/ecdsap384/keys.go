// Package ecdsap384 提供 ECDSA P-384 + SHA-384 签名验签与密钥 PEM 管理
//
// 签名输出 ASN.1 DER，对应 JCA SHA384withECDSA；JWT ES384 使用固定宽度 R||S，需另行转换。
// 仅支持密钥 PEM（PKCS#8/SEC1/PKIX），不含证书与加密私钥。
//
// @author zwei
package ecdsap384

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// GenerateKey 生成 P-384 私钥
func GenerateKey() (*ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "ecdsap384: generate key")
	}
	return key, nil
}

// validPrivateKey 校验 P-384 私钥的曲线、标量与公钥坐标
func validPrivateKey(key *ecdsa.PrivateKey) error {
	if key == nil {
		return errors.New("ecdsap384: private key 为 nil")
	}
	if !isP384(key.Curve) {
		return errors.New("ecdsap384: private key 不是 P-384 曲线")
	}
	if key.D == nil || key.D.Sign() <= 0 || key.D.Cmp(key.Curve.Params().N) >= 0 {
		return errors.New("ecdsap384: private key 无效")
	}
	if key.X == nil || key.Y == nil || !key.IsOnCurve(key.X, key.Y) {
		return errors.New("ecdsap384: private key 公钥坐标无效")
	}
	return nil
}

// validPublicKey 校验 P-384 公钥的曲线与坐标
func validPublicKey(key *ecdsa.PublicKey) error {
	if key == nil {
		return errors.New("ecdsap384: public key 为 nil")
	}
	if !isP384(key.Curve) {
		return errors.New("ecdsap384: public key 不是 P-384 曲线")
	}
	if key.X == nil || key.Y == nil || !key.IsOnCurve(key.X, key.Y) {
		return errors.New("ecdsap384: public key 无效")
	}
	return nil
}

// ParsePrivateKeyPEM 解析 PKCS#8 或 SEC1 私钥 PEM
func ParsePrivateKeyPEM(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("ecdsap384: 无效的私钥 PEM")
	}
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, ok := parsed.(*ecdsa.PrivateKey)
		if !ok || !isP384(key.Curve) {
			return nil, errors.New("ecdsap384: PEM 不是 P-384 私钥")
		}
		return key, nil
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "ecdsap384: parse private key")
	}
	if !isP384(key.Curve) {
		return nil, errors.New("ecdsap384: PEM 不是 P-384 私钥")
	}
	return key, nil
}

// ParsePublicKeyPEM 解析 PKIX 公钥 PEM
func ParsePublicKeyPEM(pemBytes []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("ecdsap384: 无效的公钥 PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "ecdsap384: parse public key")
	}
	pub, ok := parsed.(*ecdsa.PublicKey)
	if !ok || !isP384(pub.Curve) {
		return nil, errors.New("ecdsap384: PEM 不是 P-384 公钥")
	}
	return pub, nil
}

// MarshalPrivateKeyPEM 将私钥编码为 PKCS#8 PEM
func MarshalPrivateKeyPEM(key *ecdsa.PrivateKey) ([]byte, error) {
	if err := validPrivateKey(key); err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, errors.Wrap(err, "ecdsap384: marshal private key")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// MarshalPublicKeyPEM 将公钥编码为 PKIX PEM
func MarshalPublicKeyPEM(pub *ecdsa.PublicKey) ([]byte, error) {
	if err := validPublicKey(pub); err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, errors.Wrap(err, "ecdsap384: marshal public key")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

// isP384 判断曲线是否为 P-384
func isP384(curve elliptic.Curve) bool {
	return curve == elliptic.P384()
}
