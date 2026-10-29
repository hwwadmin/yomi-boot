// Store 接口定义与存储配置。
//
// @author zwei

package captcha

import (
	"context"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	defaultMaxEntries = 10000
	minMaxEntries     = 1
	maxMaxEntries     = 100000

	defaultKeyPrefix = "yomi:captcha"
	maxKeyPrefixLen  = 64

	maxIDLen     = 64
	maxAnswerLen = 64
)

// MemoryStoreConfig 内存 Store 配置
type MemoryStoreConfig struct {
	MaxEntries int // MaxEntries 是最大保留条目数；0 则 10000（1..100000）
}

// RedisStoreConfig Redis Store 配置
type RedisStoreConfig struct {
	KeyPrefix string // KeyPrefix 是 Redis 键前缀；空则 yomi:captcha；小写字母、数字、冒号、下划线，长度不超过 64
}

// Store 表示验证码答案存储；实现须并发安全。
type Store interface {
	// Save 保存 id 对应的答案，并按 ttl 过期。
	Save(ctx context.Context, id, answer string, ttl time.Duration) error
	// Consume 原子取出并删除 id 对应的答案；found=false 表示不存在、已过期或已被消费。
	// 并发 Consume 同一个 id 时，至多一次返回 found=true。
	Consume(ctx context.Context, id string) (answer string, found bool, err error)
}

// validateStoreRecord 校验 Store 写入入参
func validateStoreRecord(id, answer string, ttl time.Duration) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("captcha: id 不能为空")
	}
	if len(id) > maxIDLen {
		return errors.Newf("captcha: id 长度不能超过 %d", maxIDLen)
	}
	if strings.TrimSpace(answer) == "" {
		return errors.New("captcha: answer 不能为空")
	}
	if len(answer) > maxAnswerLen {
		return errors.Newf("captcha: answer 长度不能超过 %d", maxAnswerLen)
	}
	if ttl <= 0 {
		return errors.New("captcha: ttl 必须大于 0")
	}
	return nil
}

// validateConsumeID 校验 Store 读取入参
func validateConsumeID(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("captcha: id 不能为空")
	}
	if len(id) > maxIDLen {
		return errors.Newf("captcha: id 长度不能超过 %d", maxIDLen)
	}
	return nil
}
