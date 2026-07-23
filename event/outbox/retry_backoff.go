// Package outbox 提供发件箱后台抓取与发送能力
//
// @author zwei
package outbox

import (
	"math/rand/v2"
	"time"
)

// DLQ 重试退避常量，算法与 Kafka Consumer 一致：指数退避 + 抖动 + 上限
const (
	dlqRetryInitialDelay = 1 * time.Second
	dlqRetryMaxDelay     = 60 * time.Second
	dlqRetryJitterMin    = 8
	dlqRetryJitterMax    = 12
	dlqRetryJitterDenom  = 10
)

// dlqRetryDelay 根据当前重试次数计算下一次重试等待时长（含指数退避、上限与抖动）
func dlqRetryDelay(retryCount int) time.Duration {
	base := dlqRetryDelayBase(retryCount)
	if base <= 0 {
		return 0
	}
	return jitterDelay(base)
}

// dlqRetryDelayBase 计算不含抖动的指数退避基础等待时长
func dlqRetryDelayBase(retryCount int) time.Duration {
	if retryCount <= 0 {
		return 0
	}
	shift := retryCount - 1
	if shift > 6 {
		return dlqRetryMaxDelay
	}
	delay := dlqRetryInitialDelay * time.Duration(1<<shift)
	if delay > dlqRetryMaxDelay {
		return dlqRetryMaxDelay
	}
	return delay
}

// jitterDelay 将等待时长抖动到 [delay*80%, delay*120%] 区间，避免大量死信同时重试惊群
func jitterDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return delay
	}

	min := delay * dlqRetryJitterMin / dlqRetryJitterDenom
	max := delay * dlqRetryJitterMax / dlqRetryJitterDenom
	if max <= min {
		return delay
	}
	return min + time.Duration(rand.Int64N(int64(max-min)+1))
}
