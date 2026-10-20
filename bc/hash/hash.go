// Package hash 提供 MD5 / SHA-1（遗留兼容）与 SHA-256 / SHA-512 摘要；禁止用于密码（密码见 passwd）
//
// @author zwei
package hash

import (
	stdmd5 "crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
)

// MD5 计算 MD5 摘要（16 字节；遗留兼容，非安全首选）
func MD5(data []byte) []byte {
	sum := stdmd5.Sum(data)
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}

// MD5Hex 计算 MD5 摘要并返回小写 hex
func MD5Hex(data []byte) string {
	return hex.EncodeToString(MD5(data))
}

// SHA1 计算 SHA-1 摘要（20 字节；遗留兼容，禁止用于新签名）
func SHA1(data []byte) []byte {
	sum := sha1.Sum(data)
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}

// SHA1Hex 计算 SHA-1 摘要并返回小写 hex
func SHA1Hex(data []byte) string {
	return hex.EncodeToString(SHA1(data))
}

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
