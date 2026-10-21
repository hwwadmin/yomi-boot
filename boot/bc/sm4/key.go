// Package sm4 提供 SM4 加解密：GCM（推荐）、CBC、ECB（仅遗留）
//
// @author zwei
package sm4

import "gitee.com/hq_project/yomi-boot/boot/errors"

// validSM4Key 校验 SM4 密钥长度是否合法
func validSM4Key(key []byte) error {
	if len(key) != 16 {
		return errors.New("sm4: key 长度必须为 16 字节")
	}
	return nil
}
