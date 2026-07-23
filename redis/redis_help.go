// Package redis 提供 Redis 客户端连接与常用操作封装
//
// @author zwei
package redis

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"yomi-boot/errors"

	"github.com/google/uuid"
	redisv9 "github.com/redis/go-redis/v9"
)

// RedisService 表示 Redis 常用读写能力接口
type RedisService interface {
	// OpsStr4Set 写入 Redis String 值，并按需设置过期时间
	OpsStr4Set(ctx context.Context, key string, value string, ttl time.Duration) error
	// OpsStr4SetNoTTL 写入 Redis String 值，不设置过期时间
	OpsStr4SetNoTTL(ctx context.Context, key string, value string) error
	// OpsStr4SetJSON 将对象序列化为 JSON 后写入 Redis String
	OpsStr4SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
	// OpsStr4SetJSONNoTTL 将对象序列化为 JSON 后写入 Redis String，不设置过期时间
	OpsStr4SetJSONNoTTL(ctx context.Context, key string, value any) error
	// OpsStr4Get 读取 Redis String 值
	OpsStr4Get(ctx context.Context, key string) (value string, exists bool, err error)
	// OpsStr4GetJSON 读取 Redis String 中的 JSON 值并反序列化到目标对象
	OpsStr4GetJSON(ctx context.Context, key string, target any) (exists bool, err error)
	// OpsHash4Set 写入 Redis Hash 字段，并按需设置过期时间
	OpsHash4Set(ctx context.Context, key string, fields map[string]any, ttl time.Duration) error
	// OpsHash4Get 读取 Redis Hash 的单个字段值
	OpsHash4Get(ctx context.Context, key string, field string) (value string, exists bool, err error)
	// OpsHash4GetAll 读取 Redis Hash 的全部字段
	OpsHash4GetAll(ctx context.Context, key string) (values map[string]string, exists bool, err error)
	// OpsKey4Delete 删除一个或多个 Key
	OpsKey4Delete(ctx context.Context, keys ...string) error
	// OpsKey4Exists 判断指定 Key 是否存在
	OpsKey4Exists(ctx context.Context, key string) (bool, error)
	// OpsKey4Expire 为指定 Key 设置新的过期时间
	OpsKey4Expire(ctx context.Context, key string, ttl time.Duration) (applied bool, err error)
	// OpsSet4Add 向 Redis Set 写入一个或多个成员
	OpsSet4Add(ctx context.Context, key string, members ...string) error
	// OpsSet4Remove 从 Redis Set 中移除一个或多个成员
	OpsSet4Remove(ctx context.Context, key string, members ...string) error
	// OpsScript4Eval 执行 Lua 脚本，keys 与 args 分别映射为 KEYS 与 ARGV
	OpsScript4Eval(ctx context.Context, script string, keys []string, args ...any) (any, error)
	// OpsRateLimit4Allow 滑动窗口限流：尝试消耗 n 次配额；allowed=false 且 err=nil 表示超限
	OpsRateLimit4Allow(ctx context.Context, key string, limit, n int64, window time.Duration) (allowed bool, err error)
	// OpsSet4Members 读取 Redis Set 中的全部成员
	OpsSet4Members(ctx context.Context, key string) ([]string, error)
}

// RedisHelper 封装 Redis 常用读写操作
type RedisHelper struct {
	client *redisv9.Client // client 是底层 Redis 客户端
}

var _ RedisService = (*RedisHelper)(nil)

// NewRedisHelper 创建 Redis 常用操作助手
func NewRedisHelper(client *redisv9.Client) *RedisHelper {
	if client == nil {
		panic("redis 客户端未初始化: client 为 nil")
	}
	return &RedisHelper{client: client}
}

// OpsStr4Set 写入 Redis String 值，并按需设置过期时间
func (h *RedisHelper) OpsStr4Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if err := h.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return errors.Wrap(err, "写入 Redis String 失败")
	}

	return nil
}

// OpsStr4SetNoTTL 写入 Redis String 值，不设置过期时间
func (h *RedisHelper) OpsStr4SetNoTTL(ctx context.Context, key string, value string) error {
	return h.OpsStr4Set(ctx, key, value, 0)
}

// OpsStr4SetJSON 将对象序列化为 JSON 后写入 Redis String
func (h *RedisHelper) OpsStr4SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return errors.Wrap(err, "序列化 Redis JSON 值失败")
	}

	return h.OpsStr4Set(ctx, key, string(payload), ttl)
}

// OpsStr4SetJSONNoTTL 将对象序列化为 JSON 后写入 Redis String，不设置过期时间
func (h *RedisHelper) OpsStr4SetJSONNoTTL(ctx context.Context, key string, value any) error {
	return h.OpsStr4SetJSON(ctx, key, value, 0)
}

// OpsStr4Get 读取 Redis String 值
// key 不存在时返回 exists=false 和 nil error
func (h *RedisHelper) OpsStr4Get(ctx context.Context, key string) (value string, exists bool, err error) {
	value, err = h.client.Get(ctx, key).Result()
	if errors.Is(err, redisv9.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.Wrap(err, "读取 Redis String 失败")
	}

	return value, true, nil
}

// OpsStr4GetJSON 读取 Redis String 中的 JSON 值并反序列化到目标对象
// key 不存在时返回 exists=false 和 nil error
func (h *RedisHelper) OpsStr4GetJSON(ctx context.Context, key string, target any) (exists bool, err error) {
	if target == nil {
		return false, errors.New("redis JSON 反序列化目标不能为空")
	}

	payload, exists, err := h.OpsStr4Get(ctx, key)
	if err != nil {
		return exists, err
	}
	if !exists {
		return false, nil
	}

	if err := json.Unmarshal([]byte(payload), target); err != nil {
		return false, errors.Wrap(err, "反序列化 Redis JSON 值失败")
	}

	return true, nil
}

// OpsHash4Set 写入 Redis Hash 字段，并按需设置过期时间
func (h *RedisHelper) OpsHash4Set(ctx context.Context, key string, fields map[string]any, ttl time.Duration) error {
	if len(fields) == 0 {
		return nil
	}

	args := make([]any, 0, 1+len(fields)*2)
	args = append(args, ttl.Milliseconds())
	for field, value := range fields {
		args = append(args, field, value)
	}

	if _, err := h.client.Eval(ctx, luaHashSetWithExpire, []string{key}, args...).Result(); err != nil {
		return errors.Wrap(err, "写入 Redis Hash 失败")
	}

	return nil
}

// OpsHash4Get 读取 Redis Hash 的单个字段值
// 字段不存在时返回 exists=false 和 nil error
func (h *RedisHelper) OpsHash4Get(ctx context.Context, key string, field string) (value string, exists bool, err error) {
	value, err = h.client.HGet(ctx, key, field).Result()
	if errors.Is(err, redisv9.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.Wrap(err, "读取 Redis Hash 字段失败")
	}

	return value, true, nil
}

// OpsHash4GetAll 读取 Redis Hash 的全部字段
// key 不存在时返回 exists=false 和 nil error
func (h *RedisHelper) OpsHash4GetAll(ctx context.Context, key string) (values map[string]string, exists bool, err error) {
	values, err = h.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, false, errors.Wrap(err, "读取 Redis Hash 全字段失败")
	}
	if len(values) == 0 {
		return nil, false, nil
	}

	return values, true, nil
}

// OpsKey4Delete 删除一个或多个 Key
func (h *RedisHelper) OpsKey4Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	if err := h.client.Del(ctx, keys...).Err(); err != nil {
		return errors.Wrap(err, "删除 Redis Key 失败")
	}

	return nil
}

// OpsKey4Exists 判断指定 Key 是否存在
func (h *RedisHelper) OpsKey4Exists(ctx context.Context, key string) (bool, error) {
	count, err := h.client.Exists(ctx, key).Result()
	if err != nil {
		return false, errors.Wrap(err, "检查 Redis Key 是否存在失败")
	}

	return count > 0, nil
}

// OpsKey4Expire 为指定 Key 设置新的过期时间
// key 不存在时返回 applied=false
func (h *RedisHelper) OpsKey4Expire(ctx context.Context, key string, ttl time.Duration) (applied bool, err error) {
	applied, err = h.client.Expire(ctx, key, ttl).Result()
	if err != nil {
		return false, errors.Wrap(err, "设置 Redis Key 过期时间失败")
	}

	return applied, nil
}

// OpsSet4Add 向 Redis Set 写入一个或多个成员
func (h *RedisHelper) OpsSet4Add(ctx context.Context, key string, members ...string) error {
	if len(members) == 0 {
		return nil
	}

	if err := h.client.SAdd(ctx, key, toStringSlice(members)...).Err(); err != nil {
		return errors.Wrap(err, "写入 Redis Set 失败")
	}

	return nil
}

// OpsSet4Remove 从 Redis Set 中移除一个或多个成员
func (h *RedisHelper) OpsSet4Remove(ctx context.Context, key string, members ...string) error {
	if len(members) == 0 {
		return nil
	}

	if err := h.client.SRem(ctx, key, toStringSlice(members)...).Err(); err != nil {
		return errors.Wrap(err, "移除 Redis Set 成员失败")
	}

	return nil
}

// OpsScript4Eval 执行 Lua 脚本，keys 与 args 分别映射为 KEYS 与 ARGV
func (h *RedisHelper) OpsScript4Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	result, err := h.client.Eval(ctx, script, keys, args...).Result()
	if err != nil {
		return nil, errors.Wrap(err, "执行 Redis Lua 脚本失败")
	}

	return result, nil
}

// OpsRateLimit4Allow 滑动窗口限流：尝试消耗 n 次配额
// allowed=true 表示放行；allowed=false 且 err=nil 表示超限；Redis/脚本故障时返回 err
func (h *RedisHelper) OpsRateLimit4Allow(ctx context.Context, key string, limit, n int64, window time.Duration) (bool, error) {
	if h == nil || h.client == nil {
		return false, errors.New("RedisHelper 未初始化")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false, errors.New("限流 key 不能为空")
	}
	if limit <= 0 {
		return false, errors.New("限流 limit 必须大于 0")
	}
	if n <= 0 {
		return false, errors.New("限流 n 必须大于 0")
	}
	if window <= 0 {
		return false, errors.New("限流 window 必须大于 0")
	}
	if n > limit {
		return false, nil
	}

	windowMs := window.Milliseconds()
	if windowMs <= 0 {
		return false, errors.New("限流 window 转换为毫秒后必须大于 0")
	}

	result, err := h.OpsScript4Eval(
		ctx,
		luaRateLimitAllow,
		[]string{key},
		limit,
		n,
		windowMs,
		uuid.NewString(),
	)
	if err != nil {
		return false, err
	}

	switch v := result.(type) {
	case int64:
		return v == 1, nil
	case int:
		return v == 1, nil
	default:
		return false, errors.Newf("限流脚本返回类型异常: %T", result)
	}
}

// OpsSet4Members 读取 Redis Set 中的全部成员
func (h *RedisHelper) OpsSet4Members(ctx context.Context, key string) ([]string, error) {
	members, err := h.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, errors.Wrap(err, "读取 Redis Set 成员失败")
	}

	return members, nil
}

// toStringSlice 将 []string 转换为 []any，用于适配 go-redis 可变参数接口
func toStringSlice(v []string) []any {
	result := make([]any, 0, len(v))
	for _, s := range v {
		result = append(result, s)
	}
	return result
}
