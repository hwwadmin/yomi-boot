// Store 接口定义与存储配置。
//
// @author zwei

package captcha

import (
	"context"
	"strings"
	"time"
)

const (
	defaultKeyPrefix = "yomi:captcha:default"
	maxKeyPrefixLen  = 64

	maxIDLen = 64

	// answerDigestLen 是答案摘要的十六进制长度（SHA-256 输出 32 字节）
	answerDigestLen = 64
)

// RedisStoreConfig Redis Store 配置
type RedisStoreConfig struct {
	KeyPrefix string // KeyPrefix 是 Redis 键前缀；空则 yomi:captcha:default；小写字母、数字、冒号、下划线，长度不超过 64
}

// Store 表示验证码答案摘要存储；实现须并发安全。
//
// Service 只把答案的 SHA-256 摘要交给 Store，明文答案不落任何存储；摘要固定为
// 64 位小写十六进制字符串。
type Store interface {
	// Save 保存 id 对应的答案摘要，并按 ttl 过期。
	Save(ctx context.Context, id, digest string, ttl time.Duration) error
	// Consume 原子取出并删除 id 对应的答案摘要；found=false 表示不存在、已过期或已被消费。
	// 并发 Consume 同一个 id 时，至多一次返回 found=true。
	Consume(ctx context.Context, id string) (digest string, found bool, err error)
}

// validateStoreRecord 校验 Store 写入入参
func validateStoreRecord(id, digest string, ttl time.Duration) error {
	if strings.TrimSpace(id) == "" {
		return invalidArgument("captcha: id 不能为空")
	}
	if len(id) > maxIDLen {
		return invalidArgument("captcha: id 长度不能超过 %d", maxIDLen)
	}
	if len(digest) != answerDigestLen {
		return invalidArgument("captcha: 答案摘要长度必须为 %d", answerDigestLen)
	}
	if ttl <= 0 {
		return invalidArgument("captcha: ttl 必须大于 0")
	}
	return nil
}

// validateConsumeID 校验 Store 读取入参
func validateConsumeID(id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidArgument("captcha: id 不能为空")
	}
	if len(id) > maxIDLen {
		return invalidArgument("captcha: id 长度不能超过 %d", maxIDLen)
	}
	return nil
}
