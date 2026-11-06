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
// 64 位小写十六进制字符串。id 与 Service 相同，仅允许 ASCII 字母和数字，最长 64；
// Redis 实现据此拼键，拒绝冒号，避免两个前缀写成同一个键。
type Store interface {
	// Save 保存 id 对应的答案摘要，并按 ttl 过期。
	Save(ctx context.Context, id, digest string, ttl time.Duration) error
	// Consume 原子取出并删除 id 对应的答案摘要；found=false 表示不存在、已过期或已被消费。
	// 并发 Consume 同一个 id 时，至多一次返回 found=true。
	Consume(ctx context.Context, id string) (digest string, found bool, err error)
}

// validateStoreRecord 校验 Store 写入入参。id 规则与 validID 相同，摘要必须是小写十六进制。
func validateStoreRecord(id, digest string, ttl time.Duration) error {
	if err := validateConsumeID(id); err != nil {
		return err
	}
	if err := validateDigest(digest); err != nil {
		return err
	}
	if ttl <= 0 {
		return invalidArgument("captcha: ttl 必须大于 0")
	}
	return nil
}

// validateConsumeID 校验 Store 读取入参。规则与 validID 相同，避免冒号把前缀和 id 拼成同一个键。
func validateConsumeID(id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidArgument("captcha: id 不能为空")
	}
	if len(id) > maxIDLen {
		return invalidArgument("captcha: id 长度不能超过 %d", maxIDLen)
	}
	if !validID(id) {
		return invalidArgument("captcha: id 仅允许 ASCII 字母和数字")
	}
	return nil
}

// validateDigest 校验答案摘要为 64 位小写十六进制
func validateDigest(digest string) error {
	if len(digest) != answerDigestLen {
		return invalidArgument("captcha: 答案摘要长度必须为 %d", answerDigestLen)
	}
	for i := 0; i < len(digest); i++ {
		c := digest[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return invalidArgument("captcha: 答案摘要必须是小写十六进制")
	}
	return nil
}
