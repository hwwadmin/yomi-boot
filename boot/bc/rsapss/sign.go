// Package rsapss 提供 RSASSA-PSS + SHA-256 签名与验签
//
// 签名盐长度固定为摘要长度（32 字节）；原始消息在包内先做 SHA-256 摘要。
// 对接使用最大盐长度的 JCA/HSM 时需另行适配。
//
// @author zwei
package rsapss

import (
	"crypto"
	"crypto/rand"
	stdrsa "crypto/rsa"
	"crypto/sha256"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// ErrVerify 表示签名校验失败
var ErrVerify = errors.NewNoStack("rsapss: verify failed")

// Sign 使用 RSASSA-PSS + SHA-256 对原始消息签名
func Sign(priv *stdrsa.PrivateKey, msg []byte) ([]byte, error) {
	if priv == nil {
		return nil, errors.New("rsapss: private key 为 nil")
	}
	sum := sha256.Sum256(msg)
	sig, err := stdrsa.SignPSS(rand.Reader, priv, crypto.SHA256, sum[:], &stdrsa.PSSOptions{
		SaltLength: stdrsa.PSSSaltLengthEqualsHash,
	})
	if err != nil {
		return nil, errors.Wrap(err, "rsapss: sign")
	}
	return sig, nil
}

// Verify 校验原始消息的 RSASSA-PSS + SHA-256 签名
func Verify(pub *stdrsa.PublicKey, msg, sig []byte) error {
	if pub == nil {
		return errors.New("rsapss: public key 为 nil")
	}
	sum := sha256.Sum256(msg)
	if err := stdrsa.VerifyPSS(pub, crypto.SHA256, sum[:], sig, &stdrsa.PSSOptions{
		SaltLength: stdrsa.PSSSaltLengthEqualsHash,
	}); err != nil {
		return errors.Sentinel(ErrVerify)
	}
	return nil
}
