package distlock

import (
	"context"
	"io"
	"testing"

	"gitee.com/hq_project/yomi-boot/core/errors"

	"github.com/go-redsync/redsync/v4"
	"github.com/hashicorp/go-multierror"
)

// TestMapLockContextError 验证 mapLockContextError 各类错误映射
func TestMapLockContextError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	redisErr := &redsync.RedisError{Node: 0, Err: io.EOF}
	multiRedisErr := multierror.Append(nil, redisErr)

	tests := []struct {
		name          string
		ctx           context.Context
		err           error
		retryRedisErr bool
		wantNil       bool
	}{
		{name: "nil error", ctx: ctx, err: nil, retryRedisErr: true, wantNil: true},
		{name: "ErrFailed", ctx: ctx, err: redsync.ErrFailed, retryRedisErr: true, wantNil: true},
		{name: "ErrTaken", ctx: ctx, err: &redsync.ErrTaken{Nodes: []int{0}}, retryRedisErr: true, wantNil: true},
		{name: "RedisError retryable", ctx: ctx, err: redisErr, retryRedisErr: true, wantNil: true},
		{name: "multierror RedisError retryable", ctx: ctx, err: multiRedisErr, retryRedisErr: true, wantNil: true},
		{name: "RedisError not retryable", ctx: ctx, err: redisErr, retryRedisErr: false, wantNil: false},
		{
			name:          "ctx canceled",
			ctx:           canceledContext(t),
			err:           redisErr,
			retryRedisErr: true,
			wantNil:       false,
		},
		{
			name:          "unexpected error",
			ctx:           ctx,
			err:           errors.New("boom"),
			retryRedisErr: true,
			wantNil:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mapLockContextError(tt.ctx, tt.err, tt.retryRedisErr)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("mapLockContextError() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("mapLockContextError() = nil, want error")
			}
			if tt.name == "ctx canceled" && !errors.Is(got, context.Canceled) {
				t.Fatalf("mapLockContextError() = %v, want context.Canceled", got)
			}
			if tt.name == "RedisError not retryable" {
				var redisErr *redsync.RedisError
				if !errors.As(got, &redisErr) {
					t.Fatalf("mapLockContextError() = %v, want wrapped RedisError", got)
				}
			}
			if tt.name == "unexpected error" && got.Error() == "" {
				t.Fatal("mapLockContextError() returned empty error")
			}
		})
	}
}

// canceledContext 返回一个已取消的 context，用于测试错误映射逻辑
func canceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ctx.Err() == nil {
		t.Fatal("context not canceled")
	}
	return ctx
}

// TestIsRetryableError 验证 isRetryableError 可重试判定
func TestIsRetryableError(t *testing.T) {
	t.Parallel()

	redisErr := &redsync.RedisError{Node: 1, Err: io.EOF}
	if !isRetryableError(redisErr) {
		t.Fatal("isRetryableError(RedisError) = false, want true")
	}
	if isRetryableError(redsync.ErrFailed) {
		t.Fatal("isRetryableError(ErrFailed) = true, want false")
	}
	if isRetryableError(errors.New("other")) {
		t.Fatal("isRetryableError(other) = true, want false")
	}

	// context 超时/取消应视为可重试，避免单次续期超时就标记锁丢失
	if !isRetryableError(context.DeadlineExceeded) {
		t.Fatal("isRetryableError(DeadlineExceeded) = false, want true")
	}
	if !isRetryableError(context.Canceled) {
		t.Fatal("isRetryableError(Canceled) = false, want true")
	}
}
