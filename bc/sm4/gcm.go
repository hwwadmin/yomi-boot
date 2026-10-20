package sm4

import (
	"crypto/cipher"

	gmsm4 "github.com/emmansun/gmsm/sm4"

	"gitee.com/hq_project/yomi-boot/bc/random"
	"gitee.com/hq_project/yomi-boot/errors"
)

const gcmNonceSize = 12

// GCM SM4-GCM 加解密器（推荐用于新业务；支持 AAD 附加认证数据）
type GCM struct {
	aead cipher.AEAD
}

// NewGCM 使用 16 字节密钥创建 SM4-GCM
func NewGCM(key []byte) (*GCM, error) {
	if err := validSM4Key(key); err != nil {
		return nil, err
	}
	block, err := gmsm4.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "sm4: new cipher")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "sm4: new gcm")
	}
	return &GCM{aead: aead}, nil
}

// Seal 加密：返回 nonce || ciphertext||tag
func (g *GCM) Seal(plain []byte) ([]byte, error) {
	return g.SealWithAAD(plain, nil)
}

// SealWithAAD 加密并绑定附加认证数据：返回 nonce || ciphertext||tag
//
// aad 不随密文存储，OpenWithAAD 必须提供相同内容才能解密。
func (g *GCM) SealWithAAD(plain, aad []byte) ([]byte, error) {
	if g == nil || g.aead == nil {
		return nil, errors.New("sm4: GCM 未初始化")
	}
	nonce, err := random.Bytes(gcmNonceSize)
	if err != nil {
		return nil, err
	}
	sealed := g.aead.Seal(nil, nonce, plain, aad)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// Open 解密 Seal 产出的密文
func (g *GCM) Open(sealed []byte) ([]byte, error) {
	return g.OpenWithAAD(sealed, nil)
}

// OpenWithAAD 解密 SealWithAAD 产出的密文并校验附加认证数据
func (g *GCM) OpenWithAAD(sealed, aad []byte) ([]byte, error) {
	if g == nil || g.aead == nil {
		return nil, errors.New("sm4: GCM 未初始化")
	}
	if len(sealed) < gcmNonceSize+g.aead.Overhead() {
		return nil, errors.Sentinel(ErrOpen)
	}
	nonce := sealed[:gcmNonceSize]
	ciphertext := sealed[gcmNonceSize:]
	plain, err := g.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.Sentinel(ErrOpen)
	}
	return plain, nil
}
