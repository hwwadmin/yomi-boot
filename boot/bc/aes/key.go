// Package aes 提供 AES 加解密：GCM（推荐）、CBC、ECB（仅遗留）
//
// @author zwei
package aes

import "gitee.com/hq_project/yomi-boot/boot/errors"

// validAESKey 校验 AES 密钥长度是否合法
func validAESKey(key []byte) error {
	switch len(key) {
	case 16, 24, 32:
		return nil
	default:
		return errors.New("aes: key 长度必须为 16/24/32 字节")
	}
}
