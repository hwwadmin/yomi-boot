package rsa

import (
	"crypto/rand"
	stdrsa "crypto/rsa"
	"crypto/sha256"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// EncryptOAEP 使用 OAEP + SHA-256 加密
func EncryptOAEP(pub *stdrsa.PublicKey, plain []byte) ([]byte, error) {
	if pub == nil {
		return nil, errors.New("rsa: public key 为 nil")
	}
	cipher, err := stdrsa.EncryptOAEP(sha256.New(), rand.Reader, pub, plain, nil)
	if err != nil {
		return nil, errors.Wrap(err, "rsa: encrypt oaep")
	}
	return cipher, nil
}

// DecryptOAEP 使用 OAEP + SHA-256 解密
func DecryptOAEP(priv *stdrsa.PrivateKey, ciphertext []byte) ([]byte, error) {
	if priv == nil {
		return nil, errors.New("rsa: private key 为 nil")
	}
	plain, err := stdrsa.DecryptOAEP(sha256.New(), rand.Reader, priv, ciphertext, nil)
	if err != nil {
		return nil, errors.Wrap(err, "rsa: decrypt oaep")
	}
	return plain, nil
}
