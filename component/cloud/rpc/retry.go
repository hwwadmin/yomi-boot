// @author zwei

package rpc

import (
	"context"
	"math/rand/v2"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/core/errors"
)

// shouldRetry 判断是否继续重试：未超次数、调用方上下文未取消且错误可重试
// ctx 是调用方上下文，不是单次尝试的上下文；调用方还活着时，
// DeadlineExceeded 视为单次尝试/发现超时，按可重试传输错误处理
func shouldRetry(ctx context.Context, err error, attempt, maxAttempts int) bool {
	if attempt >= maxAttempts {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) && !isLocalOrProtocolError(err) {
		return true
	}
	return isRetryable(err)
}

// isLocalOrProtocolError 判断错误是否为本地请求错误或响应协议错误
// 这两类错误即使包装了 DeadlineExceeded 也不重试
func isLocalOrProtocolError(err error) bool {
	var protocolErr *ProtocolError
	if errors.As(err, &protocolErr) {
		return true
	}
	var localErr *requestError
	return errors.As(err, &localErr)
}

// isRetryable 判断错误是否属于可重试的传输错误
// 只做错误类型判定；单次尝试超时与调用方到期的区分在 shouldRetry
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

// backoffDuration 返回第 attempt 次失败后的退避：指数增长后在上半区间 [d/2, d] 抖动，结果不超过 max
func backoffDuration(attempt int, base, max time.Duration) time.Duration {
	return jitterBackoff(backoffBase(attempt, base, max))
}

// backoffBase 返回不含抖动的指数退避，结果不超过 max
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

// jitterBackoff 返回 [d/2, d] 内的随机抖动，结果不超过 d
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
