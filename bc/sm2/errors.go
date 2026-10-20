// Package sm2 提供 SM2 密钥 PEM、加解密与签名验签（GB/T 32918）
//
// @author zwei
package sm2

import "gitee.com/hq_project/yomi-boot/errors"

// ErrVerify 表示签名校验失败
var ErrVerify = errors.NewNoStack("sm2: verify failed")

// ErrDecrypt 表示解密失败
var ErrDecrypt = errors.NewNoStack("sm2: decrypt failed")
