package rpc

import (
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/core/errors"
)

// testBreakerConfig 返回测试用熔断参数
func testBreakerConfig() BreakerOptions {
	return BreakerOptions{
		WindowSize:    4,
		MinRequests:   4,
		FailureRate:   0.5,
		SlowRate:      0.5,
		SlowThreshold: 10 * time.Millisecond,
		OpenDuration:  20 * time.Millisecond,
	}
}

// TestCircuitBreaker_OpensByFailureRate 验证失败率达到阈值后打开
func TestCircuitBreaker_OpensByFailureRate(t *testing.T) {
	b := newCircuitBreaker(testBreakerConfig())
	ticket, err := b.allow()
	if err != nil {
		t.Fatalf("closed 应允许: %v", err)
	}
	b.record(ticket, attemptResult{failure: true, counted: true})
	b.record(ticket, attemptResult{failure: true, counted: true})
	b.record(ticket, attemptResult{counted: true})
	b.record(ticket, attemptResult{counted: true})
	if b.state != breakerOpen {
		t.Fatalf("失败率达到 0.5 应打开，state=%d", b.state)
	}
	if _, err := b.allow(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("打开后应返回 ErrCircuitOpen，实际 %v", err)
	}
}

// TestCircuitBreaker_OpensBySlowRate 验证慢调用率达到阈值后打开
func TestCircuitBreaker_OpensBySlowRate(t *testing.T) {
	b := newCircuitBreaker(testBreakerConfig())
	ticket, _ := b.allow()
	b.record(ticket, attemptResult{slow: true, counted: true})
	b.record(ticket, attemptResult{slow: true, counted: true})
	b.record(ticket, attemptResult{counted: true})
	b.record(ticket, attemptResult{counted: true})
	if b.state != breakerOpen {
		t.Fatalf("慢调用率达到 0.5 应打开，state=%d", b.state)
	}
}

// TestCircuitBreaker_MinRequests 验证窗口请求数不足时不打开
func TestCircuitBreaker_MinRequests(t *testing.T) {
	cfg := testBreakerConfig()
	cfg.WindowSize = 5
	cfg.MinRequests = 5
	cfg.FailureRate = 1
	cfg.SlowRate = 1
	b := newCircuitBreaker(cfg)
	ticket, _ := b.allow()
	for i := 0; i < 4; i++ {
		b.record(ticket, attemptResult{failure: true, counted: true})
	}
	if b.state != breakerClosed {
		t.Fatalf("未达到 MinRequests 不应打开，state=%d", b.state)
	}
	b.record(ticket, attemptResult{failure: true, counted: true})
	if b.state != breakerOpen {
		t.Fatalf("达到 MinRequests 且失败率为 1 应打开，state=%d", b.state)
	}
}

// TestCircuitBreaker_HalfOpenSuccessCloses 验证半开探测成功后关闭
func TestCircuitBreaker_HalfOpenSuccessCloses(t *testing.T) {
	b := newCircuitBreaker(testBreakerConfig())
	ticket, _ := b.allow()
	for i := 0; i < 4; i++ {
		b.record(ticket, attemptResult{failure: true, counted: true})
	}
	if b.state != breakerOpen {
		t.Fatalf("应已打开，state=%d", b.state)
	}
	time.Sleep(30 * time.Millisecond)
	probe, err := b.allow()
	if err != nil {
		t.Fatalf("冷却后应进入半开: %v", err)
	}
	if b.state != breakerHalfOpen {
		t.Fatalf("应处于半开，state=%d", b.state)
	}
	b.record(probe, attemptResult{})
	if b.state != breakerClosed {
		t.Fatalf("探测成功应关闭，state=%d", b.state)
	}
}

// TestCircuitBreaker_HalfOpenFailureReopens 验证半开探测失败后重新打开
func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	b := newCircuitBreaker(testBreakerConfig())
	ticket, _ := b.allow()
	for i := 0; i < 4; i++ {
		b.record(ticket, attemptResult{failure: true, counted: true})
	}
	time.Sleep(30 * time.Millisecond)
	probe, err := b.allow()
	if err != nil {
		t.Fatalf("冷却后应进入半开: %v", err)
	}
	b.record(probe, attemptResult{failure: true, counted: true})
	if b.state != breakerOpen {
		t.Fatalf("探测失败应重新打开，state=%d", b.state)
	}
	if _, err := b.allow(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("重新打开后应返回 ErrCircuitOpen，实际 %v", err)
	}
}

// TestCircuitBreaker_HalfOpenSlowReopens 验证半开探测慢调用后重新打开
func TestCircuitBreaker_HalfOpenSlowReopens(t *testing.T) {
	b := newCircuitBreaker(testBreakerConfig())
	ticket, _ := b.allow()
	for i := 0; i < 4; i++ {
		b.record(ticket, attemptResult{failure: true, counted: true})
	}
	time.Sleep(30 * time.Millisecond)
	probe, err := b.allow()
	if err != nil {
		t.Fatalf("冷却后应进入半开: %v", err)
	}
	b.record(probe, attemptResult{slow: true, counted: true})
	if b.state != breakerOpen {
		t.Fatalf("探测慢调用应重新打开，state=%d", b.state)
	}
}

// TestCircuitBreaker_HalfOpen429Reopens 验证半开探测遇到 429 后重新打开
func TestCircuitBreaker_HalfOpen429Reopens(t *testing.T) {
	b := newCircuitBreaker(testBreakerConfig())
	ticket, _ := b.allow()
	for i := 0; i < 4; i++ {
		b.record(ticket, attemptResult{failure: true, counted: true})
	}
	time.Sleep(30 * time.Millisecond)
	probe, err := b.allow()
	if err != nil {
		t.Fatalf("冷却后应进入半开: %v", err)
	}
	b.record(probe, attemptResult{throttled: true})
	if b.state != breakerOpen {
		t.Fatalf("探测 429 应重新打开，state=%d", b.state)
	}
}

// TestCircuitBreaker_4xxDoesNotDiluteFailureRate 验证 4xx 不稀释失败率
func TestCircuitBreaker_4xxDoesNotDiluteFailureRate(t *testing.T) {
	cfg := testBreakerConfig()
	cfg.WindowSize = 4
	cfg.MinRequests = 1
	cfg.FailureRate = 1
	cfg.SlowRate = 1
	b := newCircuitBreaker(cfg)
	ticket, _ := b.allow()
	b.record(ticket, attemptResult{failure: true, counted: true})
	for i := 0; i < 3; i++ {
		b.record(ticket, attemptResult{counted: false})
	}
	if b.state != breakerOpen {
		t.Fatalf("1 个 5xx + 3 个 4xx 应按 1/1 失败率打开，state=%d", b.state)
	}
}

// TestCircuitBreaker_4xxNotCountedTowardMinRequests 验证 4xx 不满足最小请求数
func TestCircuitBreaker_4xxNotCountedTowardMinRequests(t *testing.T) {
	cfg := testBreakerConfig()
	cfg.WindowSize = 4
	cfg.MinRequests = 2
	cfg.FailureRate = 1
	cfg.SlowRate = 1
	b := newCircuitBreaker(cfg)
	ticket, _ := b.allow()
	b.record(ticket, attemptResult{failure: true, counted: true})
	for i := 0; i < 3; i++ {
		b.record(ticket, attemptResult{counted: false})
	}
	if b.state != breakerClosed {
		t.Fatalf("失败率样本只有 1 个，不应达到 MinRequests，state=%d", b.state)
	}
	b.record(ticket, attemptResult{failure: true, counted: true})
	if b.state != breakerOpen {
		t.Fatalf("失败率样本达到 2 个且全失败应打开，state=%d", b.state)
	}
}

// TestCircuitBreaker_NeutralReleasesProbe 验证中性结果释放半开探测位且不改变状态
func TestCircuitBreaker_NeutralReleasesProbe(t *testing.T) {
	b := newCircuitBreaker(testBreakerConfig())
	ticket, _ := b.allow()
	for i := 0; i < 4; i++ {
		b.record(ticket, attemptResult{failure: true, counted: true})
	}
	time.Sleep(30 * time.Millisecond)
	probe, err := b.allow()
	if err != nil {
		t.Fatalf("冷却后应进入半开: %v", err)
	}
	b.onNeutral(probe)
	if b.state != breakerHalfOpen {
		t.Fatalf("中性结果不应改变状态，state=%d", b.state)
	}
	if _, err := b.allow(); err != nil {
		t.Fatalf("中性结果应释放半开探测位，实际 %v", err)
	}
}

// TestCircuitBreaker_StaleProbeIgnored 验证上一轮半开探测结果不影响新一轮
func TestCircuitBreaker_StaleProbeIgnored(t *testing.T) {
	b := newCircuitBreaker(testBreakerConfig())
	ticket, _ := b.allow()
	for i := 0; i < 4; i++ {
		b.record(ticket, attemptResult{failure: true, counted: true})
	}
	time.Sleep(30 * time.Millisecond)
	oldProbe, err := b.allow()
	if err != nil {
		t.Fatalf("冷却后应进入半开: %v", err)
	}
	b.onNeutral(oldProbe)
	newProbe, err := b.allow()
	if err != nil {
		t.Fatalf("释放探测位后应允许新探测: %v", err)
	}
	b.record(oldProbe, attemptResult{})
	if b.state != breakerHalfOpen {
		t.Fatalf("旧探测结果不应关闭熔断，state=%d", b.state)
	}
	b.record(newProbe, attemptResult{})
	if b.state != breakerClosed {
		t.Fatalf("新探测成功后应关闭，state=%d", b.state)
	}
}

// TestCircuitBreaker_RecordReportsOpen 验证 record 返回记录后熔断是否处于打开状态
func TestCircuitBreaker_RecordReportsOpen(t *testing.T) {
	cfg := testBreakerConfig()
	cfg.WindowSize = 1
	cfg.MinRequests = 1
	cfg.FailureRate = 1
	cfg.SlowRate = 1
	b := newCircuitBreaker(cfg)

	ticket, _ := b.allow()
	if !b.record(ticket, attemptResult{failure: true, counted: true}) {
		t.Fatal("首次失败打开熔断后应返回 true")
	}
	if !b.record(ticket, attemptResult{failure: true, counted: true}) {
		t.Fatal("open 态应返回 true")
	}

	time.Sleep(30 * time.Millisecond)
	probe, err := b.allow()
	if err != nil {
		t.Fatalf("冷却后应进入半开: %v", err)
	}
	if b.record(probe, attemptResult{counted: true}) {
		t.Fatal("半开探测成功应返回 false")
	}
}

// TestCircuitBreaker_HalfOpenOnlyOneProbeInFlight 验证半开只放行一个探测请求
func TestCircuitBreaker_HalfOpenOnlyOneProbeInFlight(t *testing.T) {
	cfg := testBreakerConfig()
	cfg.WindowSize = 1
	cfg.MinRequests = 1
	cfg.FailureRate = 1
	cfg.SlowRate = 1
	b := newCircuitBreaker(cfg)

	ticket, _ := b.allow()
	b.record(ticket, attemptResult{failure: true, counted: true})
	time.Sleep(30 * time.Millisecond)

	probe, err := b.allow()
	if err != nil {
		t.Fatalf("冷却后应放行一个探测: %v", err)
	}
	if !probe.probe {
		t.Fatal("半开应返回探测票据")
	}
	if _, err := b.allow(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("半开只允许一个探测，实际 %v", err)
	}
	if b.record(probe, attemptResult{counted: true}) {
		t.Fatal("探测成功应关闭熔断")
	}
	if b.state != breakerClosed {
		t.Fatalf("探测成功后应关闭，state=%d", b.state)
	}
}

// TestCircuitBreaker_HalfOpenFailureResetsTimer 验证探测失败后重新打开并重置计时
func TestCircuitBreaker_HalfOpenFailureResetsTimer(t *testing.T) {
	cfg := testBreakerConfig()
	cfg.WindowSize = 1
	cfg.MinRequests = 1
	cfg.FailureRate = 1
	cfg.SlowRate = 1
	cfg.OpenDuration = 50 * time.Millisecond
	b := newCircuitBreaker(cfg)

	ticket, _ := b.allow()
	b.record(ticket, attemptResult{failure: true, counted: true})
	firstOpenUntil := b.openUntil

	time.Sleep(60 * time.Millisecond)
	probe, err := b.allow()
	if err != nil {
		t.Fatalf("冷却后应放行探测: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if !b.record(probe, attemptResult{failure: true, counted: true}) {
		t.Fatal("探测失败应返回打开状态")
	}
	if b.state != breakerOpen {
		t.Fatalf("探测失败后应重新打开，state=%d", b.state)
	}
	if !b.openUntil.After(firstOpenUntil) {
		t.Fatalf("探测失败后应重置打开计时，first=%v now=%v", firstOpenUntil, b.openUntil)
	}
}

// TestCircuitBreaker_RecordOutcome 验证 recordOutcome 区分本次打开、已打开和未打开
func TestCircuitBreaker_RecordOutcome(t *testing.T) {
	cfg := testBreakerConfig()
	cfg.WindowSize = 1
	cfg.MinRequests = 1
	cfg.FailureRate = 1
	cfg.SlowRate = 1
	b := newCircuitBreaker(cfg)

	ticket, _ := b.allow()
	if got := b.recordOutcome(ticket, attemptResult{failure: true, counted: true}); got != breakerRecordOpenedByFailure {
		t.Fatalf("首次失败打开应返回 OpenedByFailure，实际 %d", got)
	}
	if got := b.recordOutcome(ticket, attemptResult{failure: true, counted: true}); got != breakerRecordAlreadyOpen {
		t.Fatalf("已打开时应返回 AlreadyOpen，实际 %d", got)
	}

	time.Sleep(30 * time.Millisecond)
	probe, err := b.allow()
	if err != nil {
		t.Fatalf("冷却后应进入半开: %v", err)
	}
	if got := b.recordOutcome(probe, attemptResult{counted: true}); got != breakerRecordClosed {
		t.Fatalf("半开探测成功应返回 Closed，实际 %d", got)
	}

	slowBreaker := newCircuitBreaker(cfg)
	slowTicket, _ := slowBreaker.allow()
	if got := slowBreaker.recordOutcome(slowTicket, attemptResult{slow: true, counted: true}); got != breakerRecordOpenedBySlow {
		t.Fatalf("慢成功打开应返回 OpenedBySlow，实际 %d", got)
	}

	failCfg := testBreakerConfig()
	failCfg.WindowSize = 2
	failCfg.MinRequests = 2
	failCfg.FailureRate = 0.5
	failCfg.SlowRate = 1
	failBreaker := newCircuitBreaker(failCfg)
	failTicket, _ := failBreaker.allow()
	if got := failBreaker.recordOutcome(failTicket, attemptResult{failure: true, counted: true}); got != breakerRecordClosed {
		t.Fatalf("样本不足时不应打开，实际 %d", got)
	}
	if got := failBreaker.recordOutcome(failTicket, attemptResult{counted: true}); got != breakerRecordOpenedByFailure {
		t.Fatalf("成功样本补足窗口后应由失败率打开，实际 %d", got)
	}
}
