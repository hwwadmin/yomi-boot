// 文档写入版本生成。
//
// @author zwei

package rag

import (
	"crypto/rand"
	"encoding/hex"
)

// newVersion 生成文档写入版本；检索侧据此区分新旧块
func newVersion() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
