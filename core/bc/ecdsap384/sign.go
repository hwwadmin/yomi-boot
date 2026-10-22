package ecdsap384

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha512"

	"gitee.com/hq_project/yomi-boot/core/errors"
)

// ErrVerify 表示签名校验失败
var ErrVerify = errors.NewNoStack("ecdsap384: verify failed")

// Sign 使用 ECDSA P-384 + SHA-384 对原始消息签名，返回 ASN.1 DER 签名
func Sign(priv *ecdsa.PrivateKey, msg []byte) ([]byte, error) {
	if err := validPrivateKey(priv); err != nil {
		return nil, err
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
	if err := validPublicKey(pub); err != nil {
		return err
	}
	sum := sha512.Sum384(msg)
	if !ecdsa.VerifyASN1(pub, sum[:], sig) {
		return errors.Sentinel(ErrVerify)
	}
	return nil
}
