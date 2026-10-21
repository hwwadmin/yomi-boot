package sm2

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/pem"

	gmsm "github.com/emmansun/gmsm/sm2"
	"github.com/emmansun/gmsm/smx509"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// PrivateKey 是 SM2 私钥类型别名（底层 github.com/emmansun/gmsm/sm2.PrivateKey）
type PrivateKey = gmsm.PrivateKey

// PublicKey 是 SM2 公钥类型别名（SM2 曲线上的 ECDSA 公钥）
type PublicKey = ecdsa.PublicKey

// validPrivateKey 校验 SM2 私钥的曲线、公钥坐标与标量
func validPrivateKey(key *PrivateKey) error {
	if key == nil {
		return errors.New("sm2: private key 为 nil")
	}
	if !gmsm.IsSM2PublicKey(&key.PublicKey) {
		return errors.New("sm2: private key 不是 SM2 曲线")
	}
	if key.X == nil || key.Y == nil || key.D == nil || key.D.Sign() <= 0 || key.D.Cmp(key.Curve.Params().N) >= 0 {
		return errors.New("sm2: private key 无效")
	}
	return nil
}

// validPublicKey 校验 SM2 公钥的曲线与坐标
func validPublicKey(key *PublicKey) error {
	if key == nil {
		return errors.New("sm2: public key 为 nil")
	}
	if !gmsm.IsSM2PublicKey(key) {
		return errors.New("sm2: public key 不是 SM2 曲线")
	}
	if key.X == nil || key.Y == nil {
		return errors.New("sm2: public key 无效")
	}
	return nil
}

// GenerateKey 生成新的 SM2 私钥
func GenerateKey() (*PrivateKey, error) {
	key, err := gmsm.GenerateKey(rand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "sm2: generate key")
	}
	return key, nil
}

// ParsePrivateKeyPEM 解析 PKCS#8 或 SEC1 私钥 PEM
func ParsePrivateKeyPEM(pemBytes []byte) (*PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("sm2: 无效的私钥 PEM")
	}
	if parsed, err := smx509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, ok := parsed.(*gmsm.PrivateKey)
		if !ok {
			return nil, errors.New("sm2: PEM 不是 SM2 私钥")
		}
		return key, nil
	}
	ecKey, err := smx509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "sm2: parse private key")
	}
	key, err := new(gmsm.PrivateKey).FromECPrivateKey(ecKey)
	if err != nil {
		return nil, errors.Wrap(err, "sm2: parse private key")
	}
	return key, nil
}

// ParsePublicKeyPEM 解析 PKIX 公钥 PEM
func ParsePublicKeyPEM(pemBytes []byte) (*PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("sm2: 无效的公钥 PEM")
	}
	parsed, err := smx509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "sm2: parse public key")
	}
	pub, ok := parsed.(*ecdsa.PublicKey)
	if !ok || !gmsm.IsSM2PublicKey(pub) {
		return nil, errors.New("sm2: PEM 不是 SM2 公钥")
	}
	return pub, nil
}

// MarshalPrivateKeyPEM 将私钥编码为 PKCS#8 PEM
func MarshalPrivateKeyPEM(key *PrivateKey) ([]byte, error) {
	if err := validPrivateKey(key); err != nil {
		return nil, err
	}
	der, err := smx509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, errors.Wrap(err, "sm2: marshal private key")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// MarshalPublicKeyPEM 将公钥编码为 PKIX PEM
func MarshalPublicKeyPEM(pub *PublicKey) ([]byte, error) {
	if err := validPublicKey(pub); err != nil {
		return nil, err
	}
	der, err := smx509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, errors.Wrap(err, "sm2: marshal public key")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}
