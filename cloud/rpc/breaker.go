// @author zwei

package rpc

import (
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// breakerState 熔断器状态
type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

// breakerTicket 标记一次调用是否持有半开探测位
// 非探测调用的零值 ticket 不会影响半开状态
type breakerTicket struct {
	probe bool
	seq   uint64
}

// attemptResult 一次传输尝试的结果，由客户端按状态码、错误和耗时归类
type attemptResult struct {
	failure bool // failure 表示传输失败
	slow    bool // slow 表示响应耗时超过阈值
}

// circuitBreaker 基于滑动窗口的失败率/慢调用率熔断器
type circuitBreaker struct {
	mu sync.Mutex

	cfg BreakerOptions

	state         breakerState
	openUntil     time.Time
	probeSeq      uint64
	probeInFlight bool
	window        []attemptResult
}

// newCircuitBreaker 创建熔断器
func newCircuitBreaker(cfg BreakerOptions) *circuitBreaker {
	return &circuitBreaker{
		cfg:   normalizeBreakerOptions(cfg),
		state: breakerClosed,
	}
}

// allow 判断当前是否允许发起调用，并返回本次调用的探测票据
// 熔断打开时返回 ErrCircuitOpen
func (b *circuitBreaker) allow() (breakerTicket, error) {
	if b == nil {
		return breakerTicket{}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	switch b.state {
	case breakerOpen:
		if now.Before(b.openUntil) {
			return breakerTicket{}, errors.Sentinel(ErrCircuitOpen)
		}
		b.state = breakerHalfOpen
		b.probeSeq++
		b.probeInFlight = true
		b.window = nil
		return breakerTicket{probe: true, seq: b.probeSeq}, nil
	case breakerHalfOpen:
		if b.probeInFlight {
			return breakerTicket{}, errors.Sentinel(ErrCircuitOpen)
		}
		b.probeSeq++
		b.probeInFlight = true
		return breakerTicket{probe: true, seq: b.probeSeq}, nil
	default:
		return breakerTicket{}, nil
	}
}

// record 记录一次传输尝试；closed 态累计窗口，half-open 态只认当前探测票据
// 半开探测失败或慢调用都重新打开，避免慢下游仅凭一次探测就被放行
func (b *circuitBreaker) record(ticket breakerTicket, result attemptResult) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case breakerOpen:
		return
	case breakerHalfOpen:
		if !ticket.probe || ticket.seq != b.probeSeq {
			return
		}
		b.probeInFlight = false
		if result.failure || result.slow {
			b.open()
			return
		}
		b.close()
	default:
		// 丢弃上一轮半开遗留的探测结果
		if ticket.probe && ticket.seq != b.probeSeq {
			return
		}
		b.window = append(b.window, result)
		if len(b.window) > b.cfg.WindowSize {
			b.window = b.window[len(b.window)-b.cfg.WindowSize:]
		}
		if len(b.window) < b.cfg.MinRequests {
			return
		}
		if b.failureRate() >= b.cfg.FailureRate || b.slowRate() >= b.cfg.SlowRate {
			b.open()
		}
	}
}

// onNeutral 记录中性结果：调用方取消等不反映下游健康度，不累计窗口，只释放当前探测位
func (b *circuitBreaker) onNeutral(ticket breakerTicket) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == breakerHalfOpen && ticket.probe && ticket.seq == b.probeSeq {
		b.probeInFlight = false
	}
}

// slowThreshold 返回慢调用阈值
func (b *circuitBreaker) slowThreshold() time.Duration {
	if b == nil || b.cfg.SlowThreshold <= 0 {
		return defaultBreakerSlowThreshold
	}
	return b.cfg.SlowThreshold
}

// open 打开熔断并清空窗口
func (b *circuitBreaker) open() {
	b.state = breakerOpen
	b.openUntil = time.Now().Add(b.cfg.OpenDuration)
	b.probeInFlight = false
	b.window = nil
}

// close 关闭熔断并清空窗口
func (b *circuitBreaker) close() {
	b.state = breakerClosed
	b.probeInFlight = false
	b.window = nil
}

// failureRate 返回窗口内的失败率
func (b *circuitBreaker) failureRate() float64 {
	return b.rate(func(result attemptResult) bool { return result.failure })
}

// slowRate 返回窗口内的慢调用率
func (b *circuitBreaker) slowRate() float64 {
	return b.rate(func(result attemptResult) bool { return result.slow })
}

// rate 返回窗口内满足条件的比例
func (b *circuitBreaker) rate(match func(attemptResult) bool) float64 {
	total := len(b.window)
	if total == 0 {
		return 0
	}
	count := 0
	for _, result := range b.window {
		if match(result) {
			count++
		}
	}
	return float64(count) / float64(total)
}
