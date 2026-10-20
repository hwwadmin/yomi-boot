package ed25519

import (
	stded25519 "crypto/ed25519"

	"gitee.com/hq_project/yomi-boot/errors"
)

// ErrVerify 表示签名校验失败
var ErrVerify = errors.NewNoStack("ed25519: verify failed")

// Sign 使用 Ed25519 对原始消息签名
func Sign(priv PrivateKey, msg []byte) ([]byte, error) {
	if len(priv) != stded25519.PrivateKeySize {
		return nil, errors.New("ed25519: private key 长度非法")
	}
	return stded25519.Sign(priv, msg), nil
}

// Verify 校验原始消息的 Ed25519 签名
func Verify(pub PublicKey, msg, sig []byte) error {
	if len(pub) != stded25519.PublicKeySize {
		return errors.New("ed25519: public key 长度非法")
	}
	if !stded25519.Verify(pub, msg, sig) {
		return errors.Sentinel(ErrVerify)
	}
	return nil
}
