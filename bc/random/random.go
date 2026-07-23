// Package random 提供密码学安全随机数生成
//
// @author zwei
package random

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"yt-boot/errors"
)

// Bytes 生成 n 字节密码学安全随机数据
func Bytes(n int) ([]byte, error) {
	if n <= 0 {
		return nil, errors.New("random: n 必须大于 0")
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, errors.Wrap(err, "random: read")
	}
	return buf, nil
}

// Hex 生成 n 字节随机数据并编码为小写 hex（输出长度 2n）
func Hex(n int) (string, error) {
	buf, err := Bytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// URLSafe 生成 n 字节随机数据并编码为 Raw URL-safe Base64（无 padding）
func URLSafe(n int) (string, error) {
	buf, err := Bytes(n)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
