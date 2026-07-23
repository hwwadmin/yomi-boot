package rpc

import (
	"testing"
	"time"
)

// TestCircuitBreaker_OpenAndHalfOpen 验证熔断打开、半开探测与关闭
func TestCircuitBreaker_OpenAndHalfOpen(t *testing.T) {
	b := newCircuitBreaker(2, 50*time.Millisecond)

	if err := b.allow(); err != nil {
		t.Fatalf("closed 应允许: %v", err)
	}
	b.onFailure()
	if err := b.allow(); err != nil {
		t.Fatalf("未达阈值仍应允许: %v", err)
	}
	b.onFailure()
	if err := b.allow(); err == nil {
		t.Fatal("达到阈值后应打开熔断")
	}

	time.Sleep(60 * time.Millisecond)
	if err := b.allow(); err != nil {
		t.Fatalf("冷却后 half-open 应允许探测: %v", err)
	}
	// half-open 探测进行中，第二个请求应拒绝
	if err := b.allow(); err == nil {
		t.Fatal("half-open 探测进行中应拒绝其它请求")
	}
	b.onSuccess()
	if err := b.allow(); err != nil {
		t.Fatalf("探测成功后应关闭熔断: %v", err)
	}
}

// TestCircuitBreaker_HalfOpenFailureReopens 验证半开探测失败后重新打开
func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	b := newCircuitBreaker(1, 20*time.Millisecond)
	_ = b.allow()
	b.onFailure()
	time.Sleep(30 * time.Millisecond)
	_ = b.allow()
	b.onFailure()
	if err := b.allow(); err == nil {
		t.Fatal("half-open 失败后应重新打开")
	}
}
