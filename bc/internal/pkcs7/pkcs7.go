// Package pkcs7 提供 PKCS#7 填充与去填充，供 bc 内部块加密实现复用
package pkcs7

import (
	"bytes"
	"errors"
)

// ErrPadding 表示填充非法
var ErrPadding = errors.New("pkcs7: invalid padding")

// Pad 按 PKCS#7 填充到 blockSize 整数倍（已对齐时补满一个块）
func Pad(plain []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 || blockSize > 255 {
		return nil, errors.New("pkcs7: invalid block size")
	}
	pad := blockSize - len(plain)%blockSize
	out := make([]byte, len(plain)+pad)
	copy(out, plain)
	for i := len(plain); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out, nil
}

// Unpad 去除 PKCS#7 填充；填充非法时返回 ErrPadding
func Unpad(data []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 || len(data) == 0 || len(data)%blockSize != 0 {
		return nil, ErrPadding
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > blockSize || pad > len(data) {
		return nil, ErrPadding
	}
	for i := len(data) - pad; i < len(data); i++ {
		if data[i] != byte(pad) {
			return nil, ErrPadding
		}
	}
	return bytes.Clone(data[:len(data)-pad]), nil
}
