package ratelimit

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
	yomiredis "yomi-boot/redis"
	"yomi-boot/redis/redistest"

	booterrors "yomi-boot/errors"
)

// stubRedis 仅实现 OpsRateLimit4Allow，用于策略单测
type stubRedis struct {
	yomiredis.RedisService
	allowed bool
	err     error
	calls   atomic.Int64
}

// OpsRateLimit4Allow 实现 OpsRateLimit4Allow 接口（测试桩）
func (s *stubRedis) OpsRateLimit4Allow(context.Context, string, int64, int64, time.Duration) (bool, error) {
	s.calls.Add(1)
	return s.allowed, s.err
}

// enabledCfg 构造启用限流的测试配置
func enabledCfg(policy string) Config {
	enabled := true
	return Config{
		Enabled:          &enabled,
		KeyPrefix:        "yt:rl",
		RedisErrorPolicy: policy,
	}
}

// TestNew_DisabledIsNoop 验证未启用时返回 Noop 限流器
func TestNew_DisabledIsNoop(t *testing.T) {
	lim := New(nil, Config{})
	if err := lim.Allow(context.Background(), "login:ip:1.2.3.4", Rule{Limit: 1, Window: time.Minute}); err != nil {
		t.Fatalf("Noop 应恒成功: %v", err)
	}
}

// TestAllow_RateLimited 验证触发限流返回 ErrRateLimited
func TestAllow_RateLimited(t *testing.T) {
	stub := &stubRedis{allowed: false}
	lim := New(stub, enabledCfg(RedisErrorPolicyFailOpen))
	err := lim.Allow(context.Background(), "login:ip:1.2.3.4", Rule{Limit: 10, Window: time.Minute})
	if !booterrors.Is(err, ErrRateLimited) {
		t.Fatalf("期望 ErrRateLimited，实际 %v", err)
	}
}

// TestAllow_FailOpen 验证 Redis 故障时 fail_open 放行
func TestAllow_FailOpen(t *testing.T) {
	stub := &stubRedis{err: errors.New("redis down")}
	lim := New(stub, enabledCfg(RedisErrorPolicyFailOpen))
	if err := lim.Allow(context.Background(), "login:ip:1.2.3.4", Rule{Limit: 10, Window: time.Minute}); err != nil {
		t.Fatalf("fail_open 应放行: %v", err)
	}
}

// TestAllow_FailClosed 验证 Redis 故障时 fail_closed 拒绝
func TestAllow_FailClosed(t *testing.T) {
	stub := &stubRedis{err: errors.New("redis down")}
	lim := New(stub, enabledCfg(RedisErrorPolicyFailClosed))
	if err := lim.Allow(context.Background(), "login:ip:1.2.3.4", Rule{Limit: 10, Window: time.Minute}); err == nil {
		t.Fatal("fail_closed 应返回错误")
	}
}

// TestAllow_InvalidKey 验证非法 key 不调用 Redis
func TestAllow_InvalidKey(t *testing.T) {
	stub := &stubRedis{allowed: true}
	lim := New(stub, enabledCfg(RedisErrorPolicyFailOpen))
	if err := lim.Allow(context.Background(), "Login:IP", Rule{Limit: 1, Window: time.Second}); err == nil {
		t.Fatal("大写 key 应失败")
	}
	if stub.calls.Load() != 0 {
		t.Fatal("非法 key 不应调用 Redis")
	}
}

// TestAllow_Integration 验证 Redis 集成限流计数
func TestAllow_Integration(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := yomiredis.NewRedisHelper(client)
	lim := New(helper, enabledCfg(RedisErrorPolicyFailOpen))
	ctx := context.Background()
	rule := Rule{Limit: 2, Window: time.Minute}
	key := "login:ip:203.0.113.1"

	if err := lim.Allow(ctx, key, rule); err != nil {
		t.Fatalf("第 1 次应成功: %v", err)
	}
	if err := lim.Allow(ctx, key, rule); err != nil {
		t.Fatalf("第 2 次应成功: %v", err)
	}
	if err := lim.Allow(ctx, key, rule); !booterrors.Is(err, ErrRateLimited) {
		t.Fatalf("第 3 次应限流，实际 %v", err)
	}
}
