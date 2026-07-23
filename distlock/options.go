// Package distlock 锁级别（LockOptions）与工厂级别（ServiceOptions）的配置选项。
//
// @author zwei
package distlock

import (
	"time"
)

const (
	defaultLockTTL                = 30 * time.Second       // defaultLockTTL 是默认锁过期时间
	defaultAcquireTries           = 32                     // defaultAcquireTries 是默认单次获取尝试次数
	defaultAcquireDelay           = 100 * time.Millisecond // defaultAcquireDelay 是默认获取重试间隔
	defaultWatchdogExtendFailures = 2                      // defaultWatchdogExtendFailures 是看门狗连续续期失败次数上限
)

// LockOptions 单次加锁参数。
//
// 各配置项对不同加锁方法的作用（有意区分，非实现遗漏）：
//   - Lock：AcquireTries 为内层 redsync 重试次数，AcquireRetryDelay 同时用于内层与外层循环
//   - TryLockWithTimeout：AcquireTries 无效（内层固定 1 次）；总时长由 timeout 约束，节奏由 AcquireRetryDelay 控制
//   - TryLock：不使用 AcquireTries / AcquireRetryDelay
type LockOptions struct {
	TTL time.Duration // TTL 是锁过期时间
	// AcquireRetryDelay 是 Lock / TryLockWithTimeout 在锁被占用时的外层重试间隔；
	// 同时作为 Lock 内层 redsync 重试间隔（TryLockWithTimeout 内层 tries 固定为 1，仅外层使用）。
	// 设为 0 时内外层均不等待、立即重试，单次 Lock 可能连续发起 AcquireTries 次 Redis 请求；
	// ctx 取消后亦须等当前内层尝试轮次结束方可退出，高并发场景慎用。
	AcquireRetryDelay time.Duration
	// AcquireTries 仅作用于 Lock：单次 LockContext 内 redsync 的最大尝试次数。
	// TryLockWithTimeout 不使用此配置（固定 tries=1），请勿期望调大它能延长限时等待。
	AcquireTries int

	// Watchdog 是否在持锁期间自动续期；默认 false
	Watchdog bool
	// WatchdogInterval 是看门狗续期间隔；为 0 且 Watchdog 为 true 时使用 TTL/3
	WatchdogInterval time.Duration
	// MaxHoldDuration 是持锁最长时间，到期后停止续期并 cancel HeldContext；0 表示不限制
	MaxHoldDuration time.Duration

	acquireRetryDelaySet bool // 是否通过 Option 显式设置过 AcquireRetryDelay
}

// LockOption 自定义单次加锁参数
type LockOption func(*LockOptions)

// WithTTL 设置锁过期时间
func WithTTL(d time.Duration) LockOption {
	return func(o *LockOptions) {
		o.TTL = d
	}
}

// WithAcquireTries 设置 Lock 单次 LockContext 内 redsync 的最大尝试次数。
// 不影响 TryLock（固定 1 次）与 TryLockWithTimeout（固定 1 次，由 timeout 约束总时长）。
func WithAcquireTries(n int) LockOption {
	return func(o *LockOptions) {
		o.AcquireTries = n
	}
}

// WithAcquireRetryDelay 设置锁被占用时的重试间隔。
// Lock 与 TryLockWithTimeout 的外层循环均使用此间隔；Lock 的内层 redsync 重试也使用同一间隔。
// 传入 0 表示获取失败后立即重试、不等待（可能高频访问 Redis，见 LockOptions.AcquireRetryDelay 说明）；负值将回退为默认间隔。
func WithAcquireRetryDelay(d time.Duration) LockOption {
	return func(o *LockOptions) {
		o.AcquireRetryDelay = d
		o.acquireRetryDelaySet = true
	}
}

// WithWatchdog 启用持锁期间自动续期看门狗
func WithWatchdog(enabled bool) LockOption {
	return func(o *LockOptions) {
		o.Watchdog = enabled
	}
}

// WithWatchdogInterval 设置看门狗续期间隔；0 表示使用 TTL/3
func WithWatchdogInterval(d time.Duration) LockOption {
	return func(o *LockOptions) {
		o.WatchdogInterval = d
	}
}

// WithMaxHoldDuration 设置持锁最长时间，到期后停止续期；0 表示不限制
func WithMaxHoldDuration(d time.Duration) LockOption {
	return func(o *LockOptions) {
		o.MaxHoldDuration = d
	}
}

// ServiceOptions 工厂级默认配置
type ServiceOptions struct {
	Default LockOptions // Default 是 GetLock 未覆盖时的默认加锁参数
}

// ServiceOption 自定义工厂级配置
type ServiceOption func(*ServiceOptions)

// WithDefaultLockOptions 设置工厂默认加锁参数
func WithDefaultLockOptions(opts ...LockOption) ServiceOption {
	return func(o *ServiceOptions) {
		o.Default = applyLockOptions(defaultLockOptions(), opts)
	}
}

// defaultLockOptions 返回内置默认加锁参数
func defaultLockOptions() LockOptions {
	return LockOptions{
		TTL:                  defaultLockTTL,
		AcquireRetryDelay:    defaultAcquireDelay,
		AcquireTries:         defaultAcquireTries,
		acquireRetryDelaySet: true,
	}
}

// applyLockOptions 合并基础参数与 Option 并回填默认值
func applyLockOptions(base LockOptions, opts []LockOption) LockOptions {
	merged := base
	for _, opt := range opts {
		if opt != nil {
			opt(&merged)
		}
	}
	if merged.TTL <= 0 {
		merged.TTL = defaultLockTTL
	}
	if !merged.acquireRetryDelaySet {
		merged.AcquireRetryDelay = defaultAcquireDelay
		merged.acquireRetryDelaySet = true
	} else if merged.AcquireRetryDelay < 0 {
		merged.AcquireRetryDelay = defaultAcquireDelay
	}
	if merged.AcquireTries <= 0 {
		merged.AcquireTries = defaultAcquireTries
	}
	if merged.MaxHoldDuration < 0 {
		merged.MaxHoldDuration = 0
	}
	if merged.Watchdog {
		normalizeWatchdogInterval(&merged)
	}
	return merged
}

// normalizeWatchdogInterval 确保续期间隔小于 TTL，避免首个 tick 前锁已过期
func normalizeWatchdogInterval(opts *LockOptions) {
	interval := opts.WatchdogInterval
	if interval <= 0 {
		interval = opts.TTL / 3
	}
	if interval <= 0 {
		interval = time.Second
	}
	if interval >= opts.TTL {
		interval = opts.TTL / 3
		if interval <= 0 {
			interval = time.Millisecond
		}
	}
	opts.WatchdogInterval = interval
}

// applyServiceOptions 合并工厂级 Option 并规范化默认加锁参数
func applyServiceOptions(opts []ServiceOption) ServiceOptions {
	merged := ServiceOptions{
		Default: defaultLockOptions(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&merged)
		}
	}
	merged.Default = applyLockOptions(merged.Default, nil)
	return merged
}
