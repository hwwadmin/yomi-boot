// Package rpc 提供读路径重试判定与退避
//
// @author zwei
package rpc

import (
	"context"
	"gitee.com/hq_project/yomi-boot/appctx"
	"gitee.com/hq_project/yomi-boot/errors"
	"time"
)

// isRetryableRPCError 仅传输类故障可重试：网络/发现、502/503/504/408
// 业务错误、4xx、解析错误均不重试
func isRetryableRPCError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var biz *errors.BizError
	if errors.As(err, &biz) {
		return false
	}
	var hs *httpStatusError
	if errors.As(err, &hs) {
		return hs.retryable()
	}
	var re *retryableError
	return errors.As(err, &re)
}

// isTransportSuccess 下游已响应且可判定服务存活（业务成功或业务错误信封）
func isTransportSuccess(err error) bool {
	if err == nil {
		return true
	}
	var biz *errors.BizError
	return errors.As(err, &biz)
}

// backoffDuration 计算第 attempt 次失败后的退避（attempt 从 1 起为首次失败）
func backoffDuration(attempt int, base, max time.Duration) time.Duration {
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
