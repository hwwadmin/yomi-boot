// 对 Milvus 服务端 gRPC 限流做退避重试。
//
// @author zwei

package vectordb

import (
	"context"
	"strings"
	"time"
)

const (
	maxRateLimitAttempts = 6
	rateLimitMaxWait     = 12 * time.Second
)

// isRateLimitErr 判断是否为服务端限流（含 proxy RateLimiter 中间件）
func isRateLimitErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "rate limit") || strings.Contains(msg, "ratelimit")
}

// rateLimitBackoff 第 failedAttempt 次失败后的等待；上限 12s 以覆盖 rate=0.1
func rateLimitBackoff(failedAttempt int) time.Duration {
	w := time.Duration(1<<failedAttempt) * time.Second
	if w > rateLimitMaxWait {
		w = rateLimitMaxWait
	}
	return w
}

// retryOnRateLimit 在 ctx 内对限流错误退避重试
func retryOnRateLimit(ctx context.Context, fn func() error) error {
	_, err := retryOnRateLimitVal(ctx, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// retryOnRateLimitVal 带返回值的限流重试
func retryOnRateLimitVal[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var last T
	var err error
	for attempt := 0; attempt < maxRateLimitAttempts; attempt++ {
		last, err = fn()
		if err == nil || !isRateLimitErr(err) {
			return last, err
		}
		// 最后一次失败后不再等待，避免无意义地多睡一个退避周期
		if attempt == maxRateLimitAttempts-1 {
			select {
			case <-ctx.Done():
				var zero T
				return zero, wrapCtxErr(ctx.Err())
			default:
				return last, err
			}
		}
		select {
		case <-ctx.Done():
			var zero T
			return zero, wrapCtxErr(ctx.Err())
		case <-time.After(rateLimitBackoff(attempt)):
		}
	}
	return last, err
}
