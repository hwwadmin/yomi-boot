package aes

import (
	stdaes "crypto/aes"
	"crypto/cipher"

	"gitee.com/hq_project/yomi-boot/component/core/bc/random"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// CBC AES-CBC + PKCS7（兼容对接；无完整性保证，禁止用于网络或攻击者可控密文）
type CBC struct {
	block cipher.Block
}

// NewCBC 使用 16/24/32 字节密钥创建 AES-CBC
func NewCBC(key []byte) (*CBC, error) {
	if err := validAESKey(key); err != nil {
		return nil, err
	}
	block, err := stdaes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "aes: new cipher")
	}
	return &CBC{block: block}, nil
}

// Seal 加密：返回 iv || ciphertext
func (c *CBC) Seal(plain []byte) ([]byte, error) {
	if c == nil || c.block == nil {
		return nil, errors.New("aes: CBC 未初始化")
	}
	blockSize := c.block.BlockSize()
	padded, err := pkcs7Pad(plain, blockSize)
	if err != nil {
		return nil, err
	}
	iv, err := random.Bytes(blockSize)
	if err != nil {
		return nil, err
	}
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(c.block, iv).CryptBlocks(ciphertext, padded)
	out := make([]byte, 0, len(iv)+len(ciphertext))
	out = append(out, iv...)
	out = append(out, ciphertext...)
	return out, nil
}

// Open 解密 Seal 产出的密文
func (c *CBC) Open(sealed []byte) ([]byte, error) {
	if c == nil || c.block == nil {
		return nil, errors.New("aes: CBC 未初始化")
	}
	blockSize := c.block.BlockSize()
	if len(sealed) < blockSize*2 || len(sealed)%blockSize != 0 {
		return nil, errors.Sentinel(ErrOpen)
	}
	iv := sealed[:blockSize]
	ciphertext := sealed[blockSize:]
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(c.block, iv).CryptBlocks(plain, ciphertext)
	return pkcs7Unpad(plain, blockSize)
}
