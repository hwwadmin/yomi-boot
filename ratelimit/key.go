// Package ratelimit 提供调用式分布式限流能力
//
// @author zwei
package ratelimit

import (
	"gitee.com/hq_project/yomi-boot/errors"
	"strings"
	"unicode"
)

const maxRedisKeyLen = 256

// buildFullKey 将配置前缀与逻辑 key 拼成完整 Redis Key
func buildFullKey(prefix, logicalKey string) (string, error) {
	logicalKey = strings.TrimSpace(logicalKey)
	if logicalKey == "" {
		return "", errors.New("ratelimit: key 不能为空")
	}
	if err := validateLogicalKey(logicalKey); err != nil {
		return "", err
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", errors.New("ratelimit: key_prefix 不能为空")
	}
	fullKey := prefix + ":" + logicalKey
	if len(fullKey) > maxRedisKeyLen {
		return "", errors.Newf("ratelimit: 完整 key 长度不能超过 %d", maxRedisKeyLen)
	}
	return fullKey, nil
}

// validateLogicalKey 校验逻辑 key：小写字母/数字/冒号/下划线
func validateLogicalKey(key string) error {
	for _, r := range key {
		if unicode.IsUpper(r) {
			return errors.New("ratelimit: key 必须小写")
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ':' || r == '_' || r == '.' {
			continue
		}
		return errors.New("ratelimit: key 仅允许小写字母、数字、冒号、下划线和点")
	}
	return nil
}
