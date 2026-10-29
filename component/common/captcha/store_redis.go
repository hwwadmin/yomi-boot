// 基于 Redis 的验证码存储。
//
// @author zwei

package captcha

import (
	"context"
	"strings"
	"time"

	yomiredis "gitee.com/hq_project/yomi-boot/component/common/redis"
	"gitee.com/hq_project/yomi-boot/component/core/errors"

	redisv9 "github.com/redis/go-redis/v9"
)

// luaConsumeCaptcha 原子读取并删除验证码；返回 nil 表示不存在
const luaConsumeCaptcha = `
local value = redis.call('GET', KEYS[1])
if value then
    redis.call('DEL', KEYS[1])
end
return value
`

// redisStore 是基于 Redis String 的验证码存储
type redisStore struct {
	redis  yomiredis.RedisService
	prefix string
}

var _ Store = (*redisStore)(nil)

// NewRedisStore 创建基于 Redis 的验证码存储
func NewRedisStore(redisService yomiredis.RedisService, cfg RedisStoreConfig) (Store, error) {
	if redisService == nil {
		return nil, errors.New("captcha: redis service 不能为 nil")
	}

	prefix := strings.TrimSpace(cfg.KeyPrefix)
	if prefix == "" {
		prefix = defaultKeyPrefix
	}
	if err := validateKeyPrefix(prefix); err != nil {
		return nil, err
	}

	return &redisStore{redis: redisService, prefix: prefix}, nil
}

// Save 写入答案摘要并设置 TTL
func (s *redisStore) Save(ctx context.Context, id, digest string, ttl time.Duration) error {
	if err := validateStoreRecord(id, digest, ttl); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.redis.OpsStr4Set(ctx, s.key(id), digest, ttl); err != nil {
		return errors.Wrap(err, "captcha: 保存验证码失败")
	}
	return nil
}

// Consume 原子读取并删除答案摘要
func (s *redisStore) Consume(ctx context.Context, id string) (string, bool, error) {
	if err := validateConsumeID(id); err != nil {
		return "", false, err
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}

	result, err := s.redis.OpsScript4Eval(ctx, luaConsumeCaptcha, []string{s.key(id)})
	if err != nil {
		if errors.Is(err, redisv9.Nil) {
			return "", false, nil
		}
		return "", false, errors.Wrap(err, "captcha: 读取验证码失败")
	}

	var digest string
	switch value := result.(type) {
	case string:
		digest = value
	case []byte:
		digest = string(value)
	default:
		return "", false, nil
	}
	if digest == "" {
		return "", false, nil
	}
	return digest, true, nil
}

// key 组装 Redis 键
func (s *redisStore) key(id string) string {
	return s.prefix + ":" + id
}

// validateKeyPrefix 校验 Redis 键前缀
func validateKeyPrefix(prefix string) error {
	if prefix == "" {
		return errors.New("captcha: key_prefix 不能为空")
	}
	if len(prefix) > maxKeyPrefixLen {
		return errors.Newf("captcha: key_prefix 长度不能超过 %d", maxKeyPrefixLen)
	}
	if strings.HasPrefix(prefix, ":") || strings.HasSuffix(prefix, ":") {
		return errors.New("captcha: key_prefix 不能以冒号开头或结尾")
	}
	for _, r := range prefix {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ':' || r == '_' {
			continue
		}
		return errors.New("captcha: key_prefix 仅允许小写字母、数字、冒号和下划线")
	}
	return nil
}
