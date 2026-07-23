// Package rsa 提供 RSA PEM 解析、OAEP 加解密与签名验签
//
// @author zwei
package rsa

import "yomi-boot/errors"

// ErrVerify 表示签名校验失败
var ErrVerify = errors.NewNoStack("rsa: verify failed")
