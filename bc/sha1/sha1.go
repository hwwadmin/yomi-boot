// Package sha1 提供 SHA-1 摘要（遗留兼容；非安全首选，禁止用于密码与新签名）
//
// 仅用于对接历史协议（如微信 JS-SDK 签名、OAuth 1.0）与校验存量数据；
// 新业务完整性校验请使用 bc/hash。
//
// @author zwei
package sha1

import (
	stdsha1 "crypto/sha1"
	"encoding/hex"
)

// Sum 计算 SHA-1 摘要（20 字节）
func Sum(data []byte) []byte {
	sum := stdsha1.Sum(data)
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}

// SumHex 计算 SHA-1 摘要并返回小写 hex
func SumHex(data []byte) string {
	return hex.EncodeToString(Sum(data))
}
