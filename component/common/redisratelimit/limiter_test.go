package redisratelimit

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	yomiredis "github.com/hwwadmin/yomi-boot/component/common/redis"
	"github.com/hwwadmin/yomi-boot/component/common/redis/redistest"

	booterrors "github.com/hwwadmin/yomi-boot/component/core/errors"
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

// testCfg 构造限流测试配置
func testCfg() Config {
	return Config{KeyPrefix: "yomi:rl"}
}

// testRule 构造带故障策略的测试规则
func testRule(policy string) Rule {
	return Rule{Limit: 10, Window: time.Minute, RedisErrorPolicy: policy}
}

// TestNewNoop 验证 Noop 限流器恒放行
func TestNewNoop(t *testing.T) {
	lim := NewNoop()
	if err := lim.Allow(context.Background(), "login:ip:1.2.3.4", Rule{Limit: 1, Window: time.Minute}); err != nil {
		t.Fatalf("Noop 应恒成功: %v", err)
	}
}

// TestNew_InvalidConfig 验证非法配置创建时 panic
func TestNew_InvalidConfig(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("非法 Key 前缀应 panic")
		}
	}()
	New(&stubRedis{}, Config{KeyPrefix: "YOMI"})
}

// TestNew_DefaultConfig 验证零值与空白配置按默认值正常构造
func TestNew_DefaultConfig(t *testing.T) {
	if New(&stubRedis{}, Config{}) == nil {
		t.Fatal("零值配置应正常构造")
	}
	if New(&stubRedis{}, Config{KeyPrefix: "  "}) == nil {
		t.Fatal("空白配置应按默认值正常构造")
	}
}

// TestAllow_RateLimited 验证触发限流返回 ErrRateLimited
func TestAllow_RateLimited(t *testing.T) {
	stub := &stubRedis{allowed: false}
	lim := New(stub, testCfg())
	err := lim.Allow(context.Background(), "login:ip:1.2.3.4", testRule(""))
	if !booterrors.Is(err, ErrRateLimited) {
		t.Fatalf("期望 ErrRateLimited，实际 %v", err)
	}
}

// TestAllow_FailOpen 验证 Redis 故障且调用方 ctx 仍有效时 fail_open 放行
func TestAllow_FailOpen(t *testing.T) {
	stub := &stubRedis{err: errors.New("redis down")}
	lim := New(stub, testCfg())
	if err := lim.Allow(context.Background(), "login:ip:1.2.3.4", testRule(RedisErrorPolicyFailOpen)); err != nil {
		t.Fatalf("fail_open 应放行: %v", err)
	}
}

// TestAllow_FailClosed 验证 Redis 故障时 fail_closed 拒绝
func TestAllow_FailClosed(t *testing.T) {
	stub := &stubRedis{err: errors.New("redis down")}
	lim := New(stub, testCfg())
	if err := lim.Allow(context.Background(), "login:ip:1.2.3.4", testRule(RedisErrorPolicyFailClosed)); err == nil {
		t.Fatal("fail_closed 应返回错误")
	}
}

// TestAllow_DefaultPolicyFailClosed 验证空白故障策略按 fail_closed 拒绝
func TestAllow_DefaultPolicyFailClosed(t *testing.T) {
	stub := &stubRedis{err: errors.New("redis down")}
	lim := New(stub, testCfg())
	if err := lim.Allow(context.Background(), "login:ip:1.2.3.4", testRule("  ")); err == nil {
		t.Fatal("空白策略应按 fail_closed 返回错误")
	}
}

// TestAllow_InvalidPolicy 验证非法故障策略不调用 Redis
func TestAllow_InvalidPolicy(t *testing.T) {
	stub := &stubRedis{allowed: true}
	lim := New(stub, testCfg())
	if err := lim.Allow(context.Background(), "login:ip:1.2.3.4", testRule("failopen")); err == nil {
		t.Fatal("非法故障策略应失败")
	}
	if stub.calls.Load() != 0 {
		t.Fatal("非法故障策略不应调用 Redis")
	}
}

// TestAllow_InvalidKey 验证非法 key 不调用 Redis
func TestAllow_InvalidKey(t *testing.T) {
	stub := &stubRedis{allowed: true}
	lim := New(stub, testCfg())
	if err := lim.Allow(context.Background(), "Login:IP", testRule(RedisErrorPolicyFailOpen)); err == nil {
		t.Fatal("大写 key 应失败")
	}
	if stub.calls.Load() != 0 {
		t.Fatal("非法 key 不应调用 Redis")
	}
}

// TestAllow_CanceledCtx 验证调用方已取消时不放行、不调用 Redis
func TestAllow_CanceledCtx(t *testing.T) {
	stub := &stubRedis{err: errors.New("redis down")}
	lim := New(stub, testCfg())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := lim.Allow(ctx, "login:ip:1.2.3.4", testRule(RedisErrorPolicyFailOpen))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("已取消应返回 context.Canceled，实际 %v", err)
	}
	if stub.calls.Load() != 0 {
		t.Fatal("已取消不应调用 Redis")
	}
}

// TestAllow_DeadlineExceeded 验证调用方截止时间已到时不放行
func TestAllow_DeadlineExceeded(t *testing.T) {
	stub := &stubRedis{allowed: true}
	lim := New(stub, testCfg())
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	err := lim.Allow(ctx, "login:ip:1.2.3.4", testRule(RedisErrorPolicyFailOpen))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("截止时间已到应返回 context.DeadlineExceeded，实际 %v", err)
	}
	if stub.calls.Load() != 0 {
		t.Fatal("截止时间已到不应调用 Redis")
	}
}

// cancelOnCall 在原语返回时结束调用方 ctx，用来覆盖调用过程中被取消
type cancelOnCall struct {
	stubRedis
	cancel context.CancelFunc
}

func (s *cancelOnCall) OpsRateLimit4Allow(context.Context, string, int64, int64, time.Duration) (bool, error) {
	s.calls.Add(1)
	s.cancel()
	return false, context.Canceled
}

// TestAllow_CanceledDuringCall 验证调用过程中 ctx 被取消时不按 fail_open 放行
func TestAllow_CanceledDuringCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stub := &cancelOnCall{cancel: cancel}
	lim := New(stub, testCfg())
	err := lim.Allow(ctx, "login:ip:1.2.3.4", testRule(RedisErrorPolicyFailOpen))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("调用过程中取消应返回 context.Canceled，实际 %v", err)
	}
	if stub.calls.Load() != 1 {
		t.Fatalf("应调用一次 Redis，实际 %d", stub.calls.Load())
	}
}

// TestAllow_CommandTimeoutStillFailOpen 验证调用方 ctx 仍有效时，Redis 超时按 fail_open 放行
func TestAllow_CommandTimeoutStillFailOpen(t *testing.T) {
	stub := &stubRedis{err: context.DeadlineExceeded}
	lim := New(stub, testCfg())
	if err := lim.Allow(context.Background(), "login:ip:1.2.3.4", testRule(RedisErrorPolicyFailOpen)); err != nil {
		t.Fatalf("调用方 ctx 仍有效时，Redis 超时应按 fail_open 放行: %v", err)
	}
}

// TestAllowN_AboveMax 验证超过单次上限时 fail_open 也不放行、不调用 Redis
func TestAllowN_AboveMax(t *testing.T) {
	stub := &stubRedis{allowed: true}
	lim := New(stub, testCfg())
	rule := Rule{
		Limit:            yomiredis.MaxRateLimitN + 10,
		Window:           time.Minute,
		RedisErrorPolicy: RedisErrorPolicyFailOpen,
	}
	err := lim.AllowN(context.Background(), "login:ip:1.2.3.4", yomiredis.MaxRateLimitN+1, rule)
	if err == nil {
		t.Fatal("n 超过单次上限应返回错误")
	}
	if booterrors.Is(err, ErrRateLimited) {
		t.Fatal("n 超过单次上限不是限流结果")
	}
	if stub.calls.Load() != 0 {
		t.Fatal("n 超过单次上限不应调用 Redis")
	}
}

// TestAllow_Integration 验证 Redis 集成限流计数
func TestAllow_Integration(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	helper := yomiredis.NewRedisHelper(client)
	lim := New(helper, testCfg())
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
