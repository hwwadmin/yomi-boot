package distlock

import (
	"context"
	"testing"
	"time"
)

// TestApplyLockOptions_AcquireRetryDelay 验证 AcquireRetryDelay 合并逻辑
func TestApplyLockOptions_AcquireRetryDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base LockOptions
		opts []LockOption
		want time.Duration
	}{
		{
			name: "未设置时使用默认间隔",
			base: LockOptions{TTL: defaultLockTTL, AcquireTries: defaultAcquireTries},
			want: defaultAcquireDelay,
		},
		{
			name: "显式零间隔",
			base: defaultLockOptions(),
			opts: []LockOption{WithAcquireRetryDelay(0)},
			want: 0,
		},
		{
			name: "显式正值",
			base: defaultLockOptions(),
			opts: []LockOption{WithAcquireRetryDelay(25 * time.Millisecond)},
			want: 25 * time.Millisecond,
		},
		{
			name: "负值回退默认",
			base: defaultLockOptions(),
			opts: []LockOption{WithAcquireRetryDelay(-time.Millisecond)},
			want: defaultAcquireDelay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := applyLockOptions(tt.base, tt.opts)
			if got.AcquireRetryDelay != tt.want {
				t.Fatalf("AcquireRetryDelay = %v, want %v", got.AcquireRetryDelay, tt.want)
			}
		})
	}
}

// TestApplyServiceOptions_PartialDefaultLockOptions 验证部分 DefaultLockOptions 未覆盖项保持默认
func TestApplyServiceOptions_PartialDefaultLockOptions(t *testing.T) {
	t.Parallel()

	factory := applyServiceOptions([]ServiceOption{
		WithDefaultLockOptions(WithTTL(45 * time.Second)),
	})

	if factory.Default.TTL != 45*time.Second {
		t.Fatalf("TTL = %v, want 45s", factory.Default.TTL)
	}
	if factory.Default.AcquireRetryDelay != defaultAcquireDelay {
		t.Fatalf("AcquireRetryDelay = %v, want default %v", factory.Default.AcquireRetryDelay, defaultAcquireDelay)
	}
}

// TestApplyServiceOptions_DefaultLockOptionsAcquireRetryDelay 验证 DefaultLockOptions 中 AcquireRetryDelay
func TestApplyServiceOptions_DefaultLockOptionsAcquireRetryDelay(t *testing.T) {
	t.Parallel()

	factory := applyServiceOptions([]ServiceOption{
		WithDefaultLockOptions(
			WithTTL(10*time.Second),
			WithAcquireRetryDelay(50*time.Millisecond),
			WithAcquireTries(16),
		),
	})

	if factory.Default.TTL != 10*time.Second {
		t.Fatalf("TTL = %v, want 10s", factory.Default.TTL)
	}
	if factory.Default.AcquireRetryDelay != 50*time.Millisecond {
		t.Fatalf("AcquireRetryDelay = %v, want 50ms", factory.Default.AcquireRetryDelay)
	}
	if factory.Default.AcquireTries != 16 {
		t.Fatalf("AcquireTries = %v, want 16", factory.Default.AcquireTries)
	}
}

// TestApplyServiceOptions_DefaultLockOptionsExplicitZeroRetryDelay 验证显式零 AcquireRetryDelay 应保留 0
func TestApplyServiceOptions_DefaultLockOptionsExplicitZeroRetryDelay(t *testing.T) {
	t.Parallel()

	factory := applyServiceOptions([]ServiceOption{
		WithDefaultLockOptions(WithAcquireRetryDelay(0)),
	})

	if factory.Default.AcquireRetryDelay != 0 {
		t.Fatalf("AcquireRetryDelay = %v, want 0", factory.Default.AcquireRetryDelay)
	}
}

// TestWaitRetry 验证 waitRetry 在零间隔与 ctx 取消时的行为
func TestWaitRetry(t *testing.T) {
	t.Parallel()

	t.Run("零间隔立即继续", func(t *testing.T) {
		t.Parallel()
		if !waitRetry(context.Background(), 0) {
			t.Fatal("零间隔且 ctx 有效时期望 true")
		}
	})

	t.Run("零间隔且 ctx 已取消", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if waitRetry(ctx, 0) {
			t.Fatal("零间隔且 ctx 已取消时期望 false")
		}
	})
}
