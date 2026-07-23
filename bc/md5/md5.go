// Package md5 提供 MD5 摘要（遗留兼容；非安全完整性首选，禁止用于密码）
//
// @author zwei
package md5

import (
	stdmd5 "crypto/md5"
	"encoding/hex"
)

// Sum 计算 MD5 摘要（16 字节）
func Sum(data []byte) []byte {
	sum := stdmd5.Sum(data)
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}

// SumHex 计算 MD5 摘要并返回小写 hex
func SumHex(data []byte) string {
	return hex.EncodeToString(Sum(data))
}
