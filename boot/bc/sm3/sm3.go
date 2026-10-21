// Package sm3 提供 SM3 摘要（GB/T 32905-2016）
//
// @author zwei
package sm3

import (
	"encoding/hex"

	gmsm3 "github.com/emmansun/gmsm/sm3"
)

// Size SM3 摘要字节数
const Size = gmsm3.Size

// Sum 计算 SM3 摘要（32 字节）
func Sum(data []byte) []byte {
	sum := gmsm3.Sum(data)
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}

// SumHex 计算 SM3 摘要并返回小写 hex
func SumHex(data []byte) string {
	return hex.EncodeToString(Sum(data))
}
