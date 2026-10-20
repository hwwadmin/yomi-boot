package sm2

import (
	"crypto/rand"

	gmsm "github.com/emmansun/gmsm/sm2"

	"gitee.com/hq_project/yomi-boot/errors"
)

// Sign 使用默认 UID（1234567812345678）对原始消息签名，返回 ASN.1 编码签名
func Sign(priv *PrivateKey, msg []byte) ([]byte, error) {
	return SignWithUID(priv, nil, msg)
}

// SignWithUID 使用指定 UID 对原始消息签名；uid 为空时使用默认 UID
func SignWithUID(priv *PrivateKey, uid, msg []byte) ([]byte, error) {
	if priv == nil {
		return nil, errors.New("sm2: private key 为 nil")
	}
	sig, err := priv.Sign(rand.Reader, msg, gmsm.NewSM2SignerOption(true, uid))
	if err != nil {
		return nil, errors.Wrap(err, "sm2: sign")
	}
	return sig, nil
}

// Verify 使用默认 UID 校验原始消息的 ASN.1 签名
func Verify(pub *PublicKey, msg, sig []byte) error {
	return VerifyWithUID(pub, nil, msg, sig)
}

// VerifyWithUID 使用指定 UID 校验原始消息的 ASN.1 签名；uid 为空时使用默认 UID
func VerifyWithUID(pub *PublicKey, uid, msg, sig []byte) error {
	if pub == nil {
		return errors.New("sm2: public key 为 nil")
	}
	if !gmsm.VerifyASN1WithSM2(pub, uid, msg, sig) {
		return errors.Sentinel(ErrVerify)
	}
	return nil
}
