package appctx

import (
	"context"
	"time"
)

// OrBackground 将 nil context 归一化为 Background，避免 WithCancel / WithTimeout 等派生函数 panic
func OrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// WithCancel 从 parent 派生可取消 context，nil parent 时等价于 Background
func WithCancel(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(OrBackground(parent))
}

// WithTimeout 从 parent 派生带超时的 context，nil parent 时等价于 Background
func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(OrBackground(parent), timeout)
}

// BackgroundWithCancel 基于 Background 创建可取消 context
func BackgroundWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// BackgroundWithTimeout 基于 Background 创建带超时的 context
func BackgroundWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
