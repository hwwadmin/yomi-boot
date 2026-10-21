// Package utf8util 提供 UTF-8 安全的字节截断
//
// @author zwei
package utf8util

import (
	"bytes"
	"unicode/utf8"
)

// Unlimited 表示不按字节上限截断（仍会清洗非法 UTF-8 序列）
const Unlimited = -1

// Cut 将 b 截到最多 maxBytes 字节，并回退到合法 UTF-8 边界，同时丢弃非法序列。
// maxBytes == Unlimited 时不截断，仅清洗；maxBytes == 0 且 b 非空视为截断为空。
func Cut(b []byte, maxBytes int) (out []byte, truncated bool) {
	if maxBytes < 0 {
		return bytes.ToValidUTF8(b, nil), false
	}
	if maxBytes == 0 {
		if len(b) == 0 {
			return b, false
		}
		return nil, true
	}
	if len(b) <= maxBytes {
		return bytes.ToValidUTF8(b, nil), false
	}
	cut := b[:maxBytes]
	for len(cut) > 0 {
		r, size := utf8.DecodeLastRune(cut)
		if r != utf8.RuneError || size != 1 {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return bytes.ToValidUTF8(cut, nil), true
}
