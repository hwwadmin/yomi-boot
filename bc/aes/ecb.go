package aes

import (
	stdaes "crypto/aes"
	"crypto/cipher"

	"gitee.com/hq_project/yomi-boot/errors"
)

// ECB AES-ECB + PKCS7（仅遗留对接；新业务与网络场景禁止）
type ECB struct {
	block cipher.Block
}

// NewECB 使用 16/24/32 字节密钥创建 AES-ECB
func NewECB(key []byte) (*ECB, error) {
	if err := validAESKey(key); err != nil {
		return nil, err
	}
	block, err := stdaes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "aes: new cipher")
	}
	return &ECB{block: block}, nil
}

// Seal 加密（无 IV）
func (e *ECB) Seal(plain []byte) ([]byte, error) {
	if e == nil || e.block == nil {
		return nil, errors.New("aes: ECB 未初始化")
	}
	blockSize := e.block.BlockSize()
	padded, err := pkcs7Pad(plain, blockSize)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += blockSize {
		e.block.Encrypt(out[i:i+blockSize], padded[i:i+blockSize])
	}
	return out, nil
}

// Open 解密
func (e *ECB) Open(ciphertext []byte) ([]byte, error) {
	if e == nil || e.block == nil {
		return nil, errors.New("aes: ECB 未初始化")
	}
	blockSize := e.block.BlockSize()
	if len(ciphertext) == 0 || len(ciphertext)%blockSize != 0 {
		return nil, errors.Sentinel(ErrOpen)
	}
	plain := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += blockSize {
		e.block.Decrypt(plain[i:i+blockSize], ciphertext[i:i+blockSize])
	}
	return pkcs7Unpad(plain, blockSize)
}
