// Package hmac 提供 HMAC-SHA1 / HMAC-SHA256 / HMAC-SHA512 消息认证
//
// HMAC-SHA1 仅用于遗留协议兼容；新业务优先 HMAC-SHA256。
//
// @author zwei
package hmac

import (
	stdhmac "crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"hash"

	"gitee.com/hq_project/yomi-boot/errors"
)

// SHA1 计算 HMAC-SHA1（遗留兼容）；key 为空时返回 error
func SHA1(key, data []byte) ([]byte, error) {
	return sum(sha1.New, key, data)
}

// SHA1Hex 计算 HMAC-SHA1 并返回小写 hex
func SHA1Hex(key, data []byte) (string, error) {
	return sumHex(sha1.New, key, data)
}

// SHA256 计算 HMAC-SHA256；key 为空时返回 error
func SHA256(key, data []byte) ([]byte, error) {
	return sum(sha256.New, key, data)
}

// SHA256Hex 计算 HMAC-SHA256 并返回小写 hex
func SHA256Hex(key, data []byte) (string, error) {
	return sumHex(sha256.New, key, data)
}

// SHA512 计算 HMAC-SHA512；key 为空时返回 error
func SHA512(key, data []byte) ([]byte, error) {
	return sum(sha512.New, key, data)
}

// SHA512Hex 计算 HMAC-SHA512 并返回小写 hex
func SHA512Hex(key, data []byte) (string, error) {
	return sumHex(sha512.New, key, data)
}

// Equal 常量时间比较两个 MAC，长度不同时返回 false
func Equal(mac1, mac2 []byte) bool {
	return subtle.ConstantTimeCompare(mac1, mac2) == 1
}

// sum 使用指定哈希构造器计算 HMAC
func sum(newHash func() hash.Hash, key, data []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("hmac: key 不能为空")
	}
	mac := stdhmac.New(newHash, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil), nil
}

// sumHex 计算 HMAC 并返回小写 hex
func sumHex(newHash func() hash.Hash, key, data []byte) (string, error) {
	mac, err := sum(newHash, key, data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(mac), nil
}
