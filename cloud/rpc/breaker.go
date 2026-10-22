// @author zwei

package rpc

import (
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/core/errors"
)

// breakerState 熔断器状态
type breakerState int

// 状态取值：closed 关闭、open 打开、half-open 半开
const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

// breakerTicket 标记一次调用是否持有半开探测位；非探测调用使用零值
type breakerTicket struct {
	probe bool
	seq   uint64
}

// attemptResult 一次传输尝试的结果，由客户端按状态码、错误和耗时归类
type attemptResult struct {
	failure   bool // failure 表示传输失败
	slow      bool // slow 表示响应耗时超过阈值
	counted   bool // counted 表示结果进入失败率样本（5xx/408、2xx、网络/协议错误）
	throttled bool // throttled 表示下游限流；半开探测遇到时重新打开
}

// circuitBreaker 基于滑动窗口的失败率/慢调用率熔断器
type circuitBreaker struct {
	mu sync.Mutex

	cfg BreakerOptions

	state         breakerState
	openUntil     time.Time
	probeSeq      uint64
	probeInFlight bool
	window        []attemptResult // window 记录全部结果，用于慢调用率
	failureWindow []attemptResult // failureWindow 只记录计入失败率的结果
}

// newCircuitBreaker 创建熔断器
func newCircuitBreaker(cfg BreakerOptions) *circuitBreaker {
	return &circuitBreaker{
		cfg:   normalizeBreakerOptions(cfg),
		state: breakerClosed,
	}
}

// allow 返回本次调用的探测票据；熔断打开时返回 ErrCircuitOpen，冷却结束后放行一个探测请求
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
		b.window = nil
		b.failureWindow = nil
		return b.acquireProbe(), nil
	case breakerHalfOpen:
		if b.probeInFlight {
			return breakerTicket{}, errors.Sentinel(ErrCircuitOpen)
		}
		return b.acquireProbe(), nil
	default:
		return breakerTicket{}, nil
	}
}

// acquireProbe 占用唯一探测位并返回探测票据；调用方须持有 b.mu
func (b *circuitBreaker) acquireProbe() breakerTicket {
	b.probeSeq++
	b.probeInFlight = true
	return breakerTicket{probe: true, seq: b.probeSeq}
}

// breakerRecordResult 描述一次 record 对熔断状态的影响
type breakerRecordResult int

// 记录结果：未打开、本次打开、已打开
const (
	breakerRecordClosed          breakerRecordResult = iota // 记录后未打开
	breakerRecordOpenedByFailure                            // 本次记录触发失败率打开
	breakerRecordOpenedBySlow                               // 本次记录触发慢调用率打开
	breakerRecordOpenedByProbe                              // 半开探测不健康，重新打开
	breakerRecordAlreadyOpen                                // 记录时已经打开
)

// record 记录一次尝试；熔断打开时返回 true，调用方停止重试
func (b *circuitBreaker) record(ticket breakerTicket, result attemptResult) bool {
	return b.recordOutcome(ticket, result) != breakerRecordClosed
}

// recordOutcome 记录一次尝试并返回结果；closed 态累计窗口，half-open 态只认当前探测票据
func (b *circuitBreaker) recordOutcome(ticket breakerTicket, result attemptResult) breakerRecordResult {
	if b == nil {
		return breakerRecordClosed
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case breakerOpen:
		return breakerRecordAlreadyOpen
	case breakerHalfOpen:
		if !ticket.probe || ticket.seq != b.probeSeq {
			return breakerRecordClosed
		}
		b.probeInFlight = false
		if probePassed(result) {
			b.close()
			return breakerRecordClosed
		}
		b.open()
		return breakerRecordOpenedByProbe
	default:
		// 过期探测票据的结果不进入窗口
		if ticket.probe && ticket.seq != b.probeSeq {
			return breakerRecordClosed
		}
		b.window = append(b.window, result)
		if len(b.window) > b.cfg.WindowSize {
			b.window = b.window[len(b.window)-b.cfg.WindowSize:]
		}
		if result.counted {
			b.failureWindow = append(b.failureWindow, result)
			if len(b.failureWindow) > b.cfg.WindowSize {
				b.failureWindow = b.failureWindow[len(b.failureWindow)-b.cfg.WindowSize:]
			}
		}
		if len(b.failureWindow) >= b.cfg.MinRequests && b.failureRate() >= b.cfg.FailureRate {
			b.open()
			return breakerRecordOpenedByFailure
		}
		if len(b.window) >= b.cfg.MinRequests && b.slowRate() >= b.cfg.SlowRate {
			b.open()
			return breakerRecordOpenedBySlow
		}
		return breakerRecordClosed
	}
}

// probePassed 判断探测结果是否健康：无失败、无慢调用、未被限流；慢调用和 429 视为不健康
func probePassed(result attemptResult) bool {
	return !result.failure && !result.slow && !result.throttled
}

// onNeutral 处理中性结果：保持熔断状态，释放探测位，下一次调用可重新探测
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

// open 打开熔断、重置打开计时并清空窗口
func (b *circuitBreaker) open() {
	b.state = breakerOpen
	b.openUntil = time.Now().Add(b.cfg.OpenDuration)
	b.probeInFlight = false
	b.window = nil
	b.failureWindow = nil
}

// close 关闭熔断并清空窗口
func (b *circuitBreaker) close() {
	b.state = breakerClosed
	b.probeInFlight = false
	b.window = nil
	b.failureWindow = nil
}

// failureRate 返回失败率样本窗口内的失败率
func (b *circuitBreaker) failureRate() float64 {
	return rate(b.failureWindow, func(result attemptResult) bool { return result.failure })
}

// slowRate 返回全部结果窗口内的慢调用率
func (b *circuitBreaker) slowRate() float64 {
	return rate(b.window, func(result attemptResult) bool { return result.slow })
}

// rate 返回窗口内满足条件的比例
func rate(window []attemptResult, match func(attemptResult) bool) float64 {
	total := len(window)
	if total == 0 {
		return 0
	}
	count := 0
	for _, result := range window {
		if match(result) {
			count++
		}
	}
	return float64(count) / float64(total)
}
