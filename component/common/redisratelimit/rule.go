// Package redisratelimit 提供调用式分布式限流能力
//
// @author zwei
package redisratelimit

import (
	"strings"
	"time"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

const (
	// RedisErrorPolicyFailOpen Redis 故障时放行
	RedisErrorPolicyFailOpen = "fail_open"
	// RedisErrorPolicyFailClosed Redis 故障时拒绝
	RedisErrorPolicyFailClosed = "fail_closed"

	defaultRedisErrorPolicy = RedisErrorPolicyFailClosed
)

// Rule 描述一次限流判定（由调用方按场景传入）
type Rule struct {
	Limit            int64         // Limit 是窗口内允许的最大次数
	Window           time.Duration // Window 是滑动窗口长度
	RedisErrorPolicy string        // RedisErrorPolicy 是 Redis 故障策略：fail_open / fail_closed；空白默认 fail_closed
}

// RedisErrorPolicyOrDefault 返回本次调用的 Redis 故障策略，空白为 fail_closed
func (r Rule) RedisErrorPolicyOrDefault() string {
	policy := strings.TrimSpace(strings.ToLower(r.RedisErrorPolicy))
	if policy == "" {
		return defaultRedisErrorPolicy
	}
	return policy
}

// Validate 校验规则参数
func (r Rule) Validate() error {
	if r.Limit <= 0 {
		return errors.New("redisratelimit: Rule.Limit 必须大于 0")
	}
	if r.Window <= 0 {
		return errors.New("redisratelimit: Rule.Window 必须大于 0")
	}
	if r.Window.Milliseconds() <= 0 {
		return errors.New("redisratelimit: Rule.Window 转换为毫秒后必须大于 0")
	}
	policy := r.RedisErrorPolicyOrDefault()
	if policy != RedisErrorPolicyFailOpen && policy != RedisErrorPolicyFailClosed {
		return errors.New("redisratelimit: redis_error_policy 仅支持 fail_open 或 fail_closed")
	}
	return nil
}
