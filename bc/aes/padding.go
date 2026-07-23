package aes

import (
	"bytes"
	"yomi-boot/errors"
)

// pkcs7Pad 按 PKCS7 填充到 blockSize 整数倍
func pkcs7Pad(plain []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 || blockSize > 255 {
		return nil, errors.New("aes: invalid block size")
	}
	pad := blockSize - (len(plain) % blockSize)
	if pad == 0 {
		pad = blockSize
	}
	out := make([]byte, len(plain)+pad)
	copy(out, plain)
	for i := len(plain); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out, nil
}

// pkcs7Unpad 去除 PKCS7 填充
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 || len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.Sentinel(ErrOpen)
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > blockSize || pad > len(data) {
		return nil, errors.Sentinel(ErrOpen)
	}
	for i := len(data) - pad; i < len(data); i++ {
		if data[i] != byte(pad) {
			return nil, errors.Sentinel(ErrOpen)
		}
	}
	return bytes.Clone(data[:len(data)-pad]), nil
}
