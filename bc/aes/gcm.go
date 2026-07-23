package aes

import (
	stdaes "crypto/aes"
	"crypto/cipher"
	"yomi-boot/bc/random"
	"yomi-boot/errors"
)

const gcmNonceSize = 12

// GCM AES-GCM 加解密器（推荐用于新业务）
type GCM struct {
	aead cipher.AEAD
}

// NewGCM 使用 16/24/32 字节密钥创建 AES-GCM
func NewGCM(key []byte) (*GCM, error) {
	if err := validAESKey(key); err != nil {
		return nil, err
	}
	block, err := stdaes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "aes: new cipher")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "aes: new gcm")
	}
	return &GCM{aead: aead}, nil
}

// Seal 加密：返回 nonce || ciphertext||tag
func (g *GCM) Seal(plain []byte) ([]byte, error) {
	if g == nil || g.aead == nil {
		return nil, errors.New("aes: GCM 未初始化")
	}
	nonce, err := random.Bytes(gcmNonceSize)
	if err != nil {
		return nil, err
	}
	sealed := g.aead.Seal(nil, nonce, plain, nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// Open 解密 Seal 产出的密文
func (g *GCM) Open(sealed []byte) ([]byte, error) {
	if g == nil || g.aead == nil {
		return nil, errors.New("aes: GCM 未初始化")
	}
	if len(sealed) < gcmNonceSize+g.aead.Overhead() {
		return nil, errors.Sentinel(ErrOpen)
	}
	nonce := sealed[:gcmNonceSize]
	ciphertext := sealed[gcmNonceSize:]
	plain, err := g.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.Sentinel(ErrOpen)
	}
	return plain, nil
}
