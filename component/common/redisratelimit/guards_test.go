package redisratelimit

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNew_NilRedisPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("RedisService 为 nil 应 panic")
		}
	}()
	New(nil, Config{})
}

func TestAllowN_NilAndNonPositive(t *testing.T) {
	var lim *limiter
	if err := lim.AllowN(context.Background(), "login:ip:1", 1, Rule{Limit: 1, Window: time.Second}); err == nil {
		t.Fatal("未初始化的 limiter 应返回错误")
	}
	lim = &limiter{redis: &stubRedis{allowed: true}, prefix: "yomi:rl"}
	if err := lim.AllowN(context.Background(), "login:ip:1", 0, Rule{Limit: 1, Window: time.Second}); err == nil {
		t.Fatal("n 小于等于 0 应返回错误")
	}
	if err := (noopLimiter{}).AllowN(context.Background(), "login:ip:1", 3, Rule{}); err != nil {
		t.Fatalf("Noop AllowN 应成功: %v", err)
	}
	if contextErr(context.Background()) != nil {
		t.Fatal("未取消的 ctx 应无错误")
	}
}

func TestRuleAndKeyGuards(t *testing.T) {
	if err := (Rule{}).Validate(); err == nil {
		t.Fatal("Limit 小于等于 0 应失败")
	}
	if err := (Rule{Limit: 1}).Validate(); err == nil {
		t.Fatal("Window 小于等于 0 应失败")
	}
	if err := (Rule{Limit: 1, Window: time.Microsecond}).Validate(); err == nil {
		t.Fatal("Window 换成毫秒后为 0 应失败")
	}
	if _, err := buildFullKey("yomi:rl", " "); err == nil {
		t.Fatal("空白 key 应失败")
	}
	if _, err := buildFullKey(" ", "login:ip:1"); err == nil {
		t.Fatal("空白前缀应失败")
	}
	if _, err := buildFullKey("yomi:rl", strings.Repeat("a", 300)); err == nil {
		t.Fatal("超长 key 应失败")
	}
	if err := validateRateLimitKeyPrefix(""); err == nil {
		t.Fatal("空前缀应失败")
	}
	if err := validateRateLimitKeyPrefix(strings.Repeat("a", maxRateLimitKeyPrefixLen+1)); err == nil {
		t.Fatal("超长前缀应失败")
	}
}
