// @author zwei

package rpc

import (
	"context"
	"math/rand/v2"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/appctx"
	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// shouldRetry 判断是否继续重试：未超次数、上下文未取消且错误可重试
func shouldRetry(ctx context.Context, err error, attempt, maxAttempts int) bool {
	if attempt >= maxAttempts {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	return isRetryable(err)
}

// isRetryable 判断错误是否属于可重试的传输错误
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.Retryable()
	}
	var protocolErr *ProtocolError
	if errors.As(err, &protocolErr) {
		return false
	}
	var localErr *requestError
	if errors.As(err, &localErr) {
		return false
	}
	var discoveryErr *discoveryError
	if errors.As(err, &discoveryErr) {
		return true
	}
	var retryErr *retryableError
	return errors.As(err, &retryErr)
}

// backoffDuration 计算第 attempt 次失败后的退避（attempt 从 1 起为首次失败）
// 指数退避后在上半区间 [d/2, d] 内抖动，避免多实例同时重试；抖动不会超过 max
func backoffDuration(attempt int, base, max time.Duration) time.Duration {
	return jitterBackoff(backoffBase(attempt, base, max))
}

// backoffBase 计算不含抖动的指数退避，结果不超过 max
func backoffBase(attempt int, base, max time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := base
	for i := 1; i < attempt; i++ {
		if d >= max {
			return max
		}
		next := d * 2
		if next > max || next < d {
			return max
		}
		d = next
	}
	if d > max {
		return max
	}
	return d
}

// jitterBackoff 在 [d/2, d] 内随机抖动，向下抖动以保证不超过配置的退避上限
func jitterBackoff(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	half := d / 2
	if half <= 0 {
		return d
	}
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

// canWait 判断 ctx 的剩余预算是否足以完整等待 d；无 deadline 时视为可以等待
func canWait(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(deadline) > d
}

// sleepBackoff 在 ctx 可取消的前提下等待退避时间
func sleepBackoff(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	ctx = appctx.OrBackground(ctx)
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
