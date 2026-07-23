// Package hmac 提供 HMAC-SHA256 消息认证
//
// @author zwei
package hmac

import (
	stdhmac "crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"yt-boot/errors"
)

// SHA256 计算 HMAC-SHA256；key 为空时返回 error
func SHA256(key, data []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("hmac: key 不能为空")
	}
	mac := stdhmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil), nil
}

// SHA256Hex 计算 HMAC-SHA256 并返回小写 hex
func SHA256Hex(key, data []byte) (string, error) {
	sum, err := SHA256(key, data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum), nil
}

// Equal 常量时间比较两个 MAC，长度不同时返回 false
func Equal(mac1, mac2 []byte) bool {
	return subtle.ConstantTimeCompare(mac1, mac2) == 1
}
