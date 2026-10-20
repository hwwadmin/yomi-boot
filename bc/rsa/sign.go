package rsa

import (
	"crypto"
	"crypto/rand"
	stdrsa "crypto/rsa"
	"crypto/sha256"

	"gitee.com/hq_project/yomi-boot/errors"
)

// SignPSS 使用 PSS + SHA-256 签名消息摘要前的原始消息
func SignPSS(priv *stdrsa.PrivateKey, msg []byte) ([]byte, error) {
	if priv == nil {
		return nil, errors.New("rsa: private key 为 nil")
	}
	sum := sha256.Sum256(msg)
	sig, err := stdrsa.SignPSS(rand.Reader, priv, crypto.SHA256, sum[:], &stdrsa.PSSOptions{
		SaltLength: stdrsa.PSSSaltLengthEqualsHash,
	})
	if err != nil {
		return nil, errors.Wrap(err, "rsa: sign pss")
	}
	return sig, nil
}

// VerifyPSS 校验 PSS + SHA-256 签名
func VerifyPSS(pub *stdrsa.PublicKey, msg, sig []byte) error {
	if pub == nil {
		return errors.New("rsa: public key 为 nil")
	}
	sum := sha256.Sum256(msg)
	if err := stdrsa.VerifyPSS(pub, crypto.SHA256, sum[:], sig, &stdrsa.PSSOptions{
		SaltLength: stdrsa.PSSSaltLengthEqualsHash,
	}); err != nil {
		return errors.Sentinel(ErrVerify)
	}
	return nil
}
