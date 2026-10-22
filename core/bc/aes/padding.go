package aes

import (
	"gitee.com/hq_project/yomi-boot/core/bc/internal/pkcs7"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

// pkcs7Pad 按 PKCS7 填充到 blockSize 整数倍
func pkcs7Pad(plain []byte, blockSize int) ([]byte, error) {
	out, err := pkcs7.Pad(plain, blockSize)
	if err != nil {
		return nil, errors.Wrap(err, "aes: pad")
	}
	return out, nil
}

// pkcs7Unpad 去除 PKCS7 填充
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	out, err := pkcs7.Unpad(data, blockSize)
	if err != nil {
		return nil, errors.Sentinel(ErrOpen)
	}
	return out, nil
}
