package sm4

import (
	"crypto/cipher"

	gmsm4 "github.com/emmansun/gmsm/sm4"

	"gitee.com/hq_project/yomi-boot/boot/bc/internal/pkcs7"
	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// ECB SM4-ECB + PKCS7（仅遗留对接；新业务与网络场景禁止）
type ECB struct {
	block cipher.Block
}

// NewECB 使用 16 字节密钥创建 SM4-ECB
func NewECB(key []byte) (*ECB, error) {
	if err := validSM4Key(key); err != nil {
		return nil, err
	}
	block, err := gmsm4.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "sm4: new cipher")
	}
	return &ECB{block: block}, nil
}

// Seal 加密（无 IV）
func (e *ECB) Seal(plain []byte) ([]byte, error) {
	if e == nil || e.block == nil {
		return nil, errors.New("sm4: ECB 未初始化")
	}
	blockSize := e.block.BlockSize()
	padded, err := pkcs7.Pad(plain, blockSize)
	if err != nil {
		return nil, errors.Wrap(err, "sm4: pad")
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
		return nil, errors.New("sm4: ECB 未初始化")
	}
	blockSize := e.block.BlockSize()
	if len(ciphertext) == 0 || len(ciphertext)%blockSize != 0 {
		return nil, errors.Sentinel(ErrOpen)
	}
	plain := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += blockSize {
		e.block.Decrypt(plain[i:i+blockSize], ciphertext[i:i+blockSize])
	}
	out, err := pkcs7.Unpad(plain, blockSize)
	if err != nil {
		return nil, errors.Sentinel(ErrOpen)
	}
	return out, nil
}
