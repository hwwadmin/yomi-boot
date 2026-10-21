// Package sha256withrsa 提供 SHA256withRSA（RSASSA-PKCS1-v1_5 + SHA-256）签名与验签
//
// 对应 Java SHA256withRSA、支付宝 RSA2、微信支付 v3 等常见对接方式；
// 原始消息在包内先做 SHA-256 摘要。
//
// @author zwei
package sha256withrsa

import (
	"crypto"
	"crypto/rand"
	stdrsa "crypto/rsa"
	"crypto/sha256"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// ErrVerify 表示签名校验失败
var ErrVerify = errors.NewNoStack("sha256withrsa: verify failed")

// Sign 使用 SHA256withRSA 对原始消息签名
func Sign(priv *stdrsa.PrivateKey, msg []byte) ([]byte, error) {
	if priv == nil {
		return nil, errors.New("sha256withrsa: private key 为 nil")
	}
	sum := sha256.Sum256(msg)
	sig, err := stdrsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, sum[:])
	if err != nil {
		return nil, errors.Wrap(err, "sha256withrsa: sign")
	}
	return sig, nil
}

// Verify 校验原始消息的 SHA256withRSA 签名
func Verify(pub *stdrsa.PublicKey, msg, sig []byte) error {
	if pub == nil {
		return errors.New("sha256withrsa: public key 为 nil")
	}
	sum := sha256.Sum256(msg)
	if err := stdrsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return errors.Sentinel(ErrVerify)
	}
	return nil
}
