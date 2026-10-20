package ecdsap384

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha512"

	"gitee.com/hq_project/yomi-boot/errors"
)

// ErrVerify 表示签名校验失败
var ErrVerify = errors.NewNoStack("ecdsap384: verify failed")

// Sign 使用 ECDSA P-384 + SHA-384 对原始消息签名，返回 ASN.1 DER 签名
func Sign(priv *ecdsa.PrivateKey, msg []byte) ([]byte, error) {
	if priv == nil {
		return nil, errors.New("ecdsap384: private key 为 nil")
	}
	if !isP384(priv.Curve) {
		return nil, errors.New("ecdsap384: private key 不是 P-384 曲线")
	}
	if priv.D == nil || priv.D.Sign() <= 0 {
		return nil, errors.New("ecdsap384: private key 无效")
	}
	sum := sha512.Sum384(msg)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, sum[:])
	if err != nil {
		return nil, errors.Wrap(err, "ecdsap384: sign")
	}
	return sig, nil
}

// Verify 校验原始消息的 ASN.1 DER 签名
func Verify(pub *ecdsa.PublicKey, msg, sig []byte) error {
	if pub == nil {
		return errors.New("ecdsap384: public key 为 nil")
	}
	if !isP384(pub.Curve) {
		return errors.New("ecdsap384: public key 不是 P-384 曲线")
	}
	sum := sha512.Sum384(msg)
	if !ecdsa.VerifyASN1(pub, sum[:], sig) {
		return errors.Sentinel(ErrVerify)
	}
	return nil
}
