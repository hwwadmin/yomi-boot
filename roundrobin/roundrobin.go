// Package roundrobin 分布式定时轮询调度
//
// @author zwei
package roundrobin

import (
	"context"
	"fmt"
	"gitee.com/hq_project/yomi-boot/appctx"
	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"
	"math/rand/v2"
	"time"
)

const (
	maxConsecutiveErrors = 10               // maxConsecutiveErrors 连续异常达到该阈值后输出 Warn 日志
	maxBackoff           = 60 * time.Second // maxBackoff 是指数退避的最大休眠时间
	defaultExecLockTTL   = 30 * time.Second // defaultExecLockTTL 是执行锁默认过期时间

	frequencyLockKeyPrefix = "roundrobin:frequency:" // frequencyLockKeyPrefix 是频率锁 Key 前缀
	execLockKeyPrefix      = "roundrobin:lock:"      // execLockKeyPrefix 是执行锁 Key 前缀
)

// testExecLockOpts 非空时覆盖 tick 的执行锁选项，仅供测试注入短 TTL / MaxHoldDuration
var testExecLockOpts []distlock.LockOption

// Start 校验参数后在后台 goroutine 按轮询策略执行任务；ctx 取消时退出
func Start(ctx context.Context, cfg Config, task Task, factory distlock.DistLockFactory) {
	if err := validateStartArgs(cfg, task, factory); err != nil {
		logger.ErrorWithoutCtx("roundrobin.start.invalid", "error", err)
		return
	}
	ctx = appctx.OrBackground(ctx)
	go func() {
		_ = runLoop(ctx, cfg, task, factory)
	}()
}

// Run 在当前 goroutine 按轮询策略执行任务，直至 ctx 取消
func Run(ctx context.Context, cfg Config, task Task, factory distlock.DistLockFactory) error {
	if err := validateStartArgs(cfg, task, factory); err != nil {
		return err
	}
	ctx = appctx.OrBackground(ctx)
	return runLoop(ctx, cfg, task, factory)
}

// validateStartArgs 校验 Start / Run 的入参合法性
func validateStartArgs(cfg Config, task Task, factory distlock.DistLockFactory) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	if task == nil {
		return errors.New("roundrobin.task 不能为空")
	}
	if factory == nil {
		return errors.New("roundrobin.distlock_factory 不能为空")
	}
	return nil
}

// runLoop 轮询主循环：抢锁、执行任务、jitter 或退避，直至 ctx 取消
func runLoop(ctx context.Context, cfg Config, task Task, factory distlock.DistLockFactory) error {
	logger.Info(ctx, fmt.Sprintf("RoundRobin[%s] task started.", cfg.Name))

	consecutiveErrors := 0
	for {
		if err := ctx.Err(); err != nil {
			logger.Info(ctx, fmt.Sprintf("RoundRobin[%s] task stopped.", cfg.Name))
			return err
		}

		executed, err := tick(ctx, cfg, task, factory)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Info(ctx, fmt.Sprintf("RoundRobin[%s] task stopped.", cfg.Name))
				return err
			}
			consecutiveErrors++
			backoff := BackoffDuration(consecutiveErrors)
			if consecutiveErrors >= maxConsecutiveErrors {
				logger.Warn(
					ctx,
					fmt.Sprintf("RoundRobin[%s] 连续异常次数已达 %d，当前退避 %s，请检查系统状态", cfg.Name, consecutiveErrors, backoff),
					"error", err,
				)
			} else {
				logger.Error(
					ctx,
					fmt.Sprintf("RoundRobin[%s] task error，consecutiveErrors=%d，退避 %s", cfg.Name, consecutiveErrors, backoff),
					"error", err,
				)
			}
			if sleepErr := sleep(ctx, backoff); sleepErr != nil {
				logger.Info(ctx, fmt.Sprintf("RoundRobin[%s] task stopped.", cfg.Name))
				return sleepErr
			}
			continue
		}

		if executed {
			consecutiveErrors = 0
			if sleepErr := sleep(ctx, jitterDuration(cfg)); sleepErr != nil {
				logger.Info(ctx, fmt.Sprintf("RoundRobin[%s] task stopped.", cfg.Name))
				return sleepErr
			}
		}
	}
}

// tick 执行一轮抢锁与任务调度；executed 表示是否成功持执行锁完成任务
func tick(ctx context.Context, cfg Config, task Task, factory distlock.DistLockFactory) (executed bool, err error) {
	// 抢频率锁，控制轮询频率
	freqLock := factory.GetLock(
		frequencyLockKey(cfg.Name),
		distlock.WithTTL(cfg.Frequency),
		distlock.WithWatchdog(false),
	)
	freqHeld, err := freqLock.TryLock(ctx)
	if err != nil {
		return false, err
	}
	if !freqHeld {
		if sleepErr := sleep(ctx, cfg.WakeUpInterval); sleepErr != nil {
			return false, sleepErr
		}
		return false, nil
	}

	// 抢执行锁，保证集群互斥
	execOpts := []distlock.LockOption{
		distlock.WithTTL(defaultExecLockTTL),
		distlock.WithWatchdog(true),
	}
	if testExecLockOpts != nil {
		execOpts = testExecLockOpts
	}
	execLock := factory.GetLock(execLockKey(cfg.Name), execOpts...)
	execHeld, err := execLock.TryLock(ctx)
	if err != nil {
		return false, err
	}
	if !execHeld {
		if sleepErr := sleep(ctx, cfg.WakeUpInterval); sleepErr != nil {
			return false, sleepErr
		}
		return false, nil
	}

	// panic 时仍须释放执行锁并停止看门狗，否则 goroutine 崩溃且锁无法释放
	defer func() {
		unlockCtx, cancel := appctx.BackgroundWithTimeout(5 * time.Second)
		defer cancel()
		unlockErr := execLock.Unlock(unlockCtx)
		if unlockErr != nil && !errors.Is(unlockErr, distlock.ErrLockNotHeld) && err == nil {
			err = unlockErr
		}
	}()

	// 锁丢失时 cancel runCtx，让 Task 可感知并尽快退出，避免多实例并发跑同一任务
	runCtx, stop := distlock.BindHeldContext(ctx, execLock)
	defer stop()
	runTaskSafely(runCtx, cfg.Name, task)
	return true, err
}

// runTaskSafely 执行任务并 recover panic，避免未捕获 panic 导致进程退出
func runTaskSafely(ctx context.Context, name string, task Task) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error(ctx, fmt.Sprintf("RoundRobin[%s] task panic.", name), "panic", r)
		}
	}()
	if taskErr := task(ctx); taskErr != nil {
		logger.Error(ctx, "轮询具体任务执行失败.", "name", name, "error", taskErr)
	}
}

// frequencyLockKey 构造频率锁 Redis Key
func frequencyLockKey(name string) string {
	return frequencyLockKeyPrefix + name
}

// execLockKey 构造执行锁 Redis Key
func execLockKey(name string) string {
	return execLockKeyPrefix + name
}

// jitterDuration 计算成功执行后的随机休眠时长，区间为 [Frequency+Min, Frequency+Max)
func jitterDuration(cfg Config) time.Duration {
	low := cfg.Frequency + cfg.RandomDelayMin
	high := cfg.Frequency + cfg.RandomDelayMax
	if high <= low {
		return low
	}
	return low + time.Duration(rand.Int64N(int64(high-low)))
}

// BackoffDuration 根据连续异常次数计算指数退避时长，最大不超过 maxBackoff
func BackoffDuration(consecutiveErrors int) time.Duration {
	if consecutiveErrors <= 0 {
		return time.Second
	}
	shift := consecutiveErrors - 1
	if shift > 6 {
		shift = 6
	}
	backoff := time.Duration(1<<shift) * time.Second
	if backoff > maxBackoff {
		return maxBackoff
	}
	return backoff
}

// sleep 休眠指定时长，ctx 取消时返回 ctx.Err()
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer stopTimer(timer)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// stopTimer 停止 timer 并排空 channel，避免泄漏
func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
