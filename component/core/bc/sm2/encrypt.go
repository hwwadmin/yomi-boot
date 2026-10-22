package sm2

import (
	"crypto/rand"

	gmsm "github.com/emmansun/gmsm/sm2"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Encrypt 使用 SM2 公钥加密，输出 C1C3C2 明文拼接格式
func Encrypt(pub *PublicKey, plain []byte) ([]byte, error) {
	return encrypt(pub, plain, false)
}

// EncryptASN1 使用 SM2 公钥加密，输出 ASN.1 编码密文
func EncryptASN1(pub *PublicKey, plain []byte) ([]byte, error) {
	return encrypt(pub, plain, true)
}

// Decrypt 解密 Encrypt 或 EncryptASN1 产出的密文（自动识别格式）
func Decrypt(priv *PrivateKey, ciphertext []byte) ([]byte, error) {
	if err := validPrivateKey(priv); err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 {
		return nil, errors.Sentinel(ErrDecrypt)
	}
	plain, err := gmsm.Decrypt(priv, ciphertext)
	if err != nil {
		return nil, errors.Sentinel(ErrDecrypt)
	}
	return plain, nil
}

// encrypt 校验公钥后执行 SM2 加密
func encrypt(pub *PublicKey, plain []byte, asn1 bool) ([]byte, error) {
	if err := validPublicKey(pub); err != nil {
		return nil, err
	}
	if len(plain) == 0 {
		return nil, errors.New("sm2: plain 不能为空")
	}
	var (
		ciphertext []byte
		err        error
	)
	if asn1 {
		ciphertext, err = gmsm.EncryptASN1(rand.Reader, pub, plain)
	} else {
		ciphertext, err = gmsm.Encrypt(rand.Reader, pub, plain, nil)
	}
	if err != nil {
		return nil, errors.Wrap(err, "sm2: encrypt")
	}
	return ciphertext, nil
}
