// Package hash 提供现代单向摘要（SHA-256 / SHA-512）；禁止用于密码（密码见 passwd）
//
// @author zwei
package hash

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
)

// SHA256 计算 SHA-256 摘要（32 字节）
func SHA256(data []byte) []byte {
	sum := sha256.Sum256(data)
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}

// SHA256Hex 计算 SHA-256 摘要并返回小写 hex
func SHA256Hex(data []byte) string {
	return hex.EncodeToString(SHA256(data))
}

// SHA512 计算 SHA-512 摘要（64 字节）
func SHA512(data []byte) []byte {
	sum := sha512.Sum512(data)
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}

// SHA512Hex 计算 SHA-512 摘要并返回小写 hex
func SHA512Hex(data []byte) string {
	return hex.EncodeToString(SHA512(data))
}
