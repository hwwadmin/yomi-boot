// 调度启动与触发执行
//
// @author zwei

package cronjob

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/appctx"
	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"

	"github.com/robfig/cron/v3"
)

const (
	defaultExecLockTTL = 30 * time.Second // 执行锁默认过期时间（与 roundrobin 执行锁一致）
	execLockKeyPrefix  = "cronjob:lock:"  // 执行锁 Key 前缀
	skipWarnInterval   = time.Minute      // 抢锁失败 Warn 的最小间隔
)

// jobNameState 记录进程内同名 Job 的活跃数量与跳过告警时间
type jobNameState struct {
	active       int       // active 是活跃调度器数量
	lastSkipWarn time.Time // lastSkipWarn 是最近一次跳过 Warn 的时间
}

// jobNames 是进程内 Job 名状态表，用于同名提示与跳过告警节流
var (
	jobNamesMu sync.Mutex
	jobNames   = make(map[string]*jobNameState)
)

// Start 校验后在后台按 Spec 触发；ctx 取消时停止调度并退出。
// 非法参数或 Spec 无法解析时打 Error 日志并返回已关闭的 done。
// 返回的 done 在调度停止且所有在途 Handler 结束后关闭。
func Start(ctx context.Context, job Job, h Handler, factory distlock.DistLockFactory) <-chan struct{} {
	if err := job.validate(h, factory); err != nil {
		logger.ErrorWithoutCtx("cronjob.start.invalid", "name", job.Name, "spec", job.Spec, "error", err)
		return closedDone()
	}
	return startScheduler(ctx, job, h, factory)
}

// MustStart 与 Start 相同，但启动参数非法时 panic；供启动装配阶段 Fail Fast 使用
func MustStart(ctx context.Context, job Job, h Handler, factory distlock.DistLockFactory) <-chan struct{} {
	if err := job.validate(h, factory); err != nil {
		logger.ErrorWithoutCtx("cronjob.must_start.invalid", "name", job.Name, "spec", job.Spec, "error", err)
		panic(errors.Wrap(err, "cronjob 启动参数非法"))
	}
	return startScheduler(ctx, job, h, factory)
}

// startScheduler 在后台启动调度，返回调度与在途 Handler 全部结束的信号
func startScheduler(ctx context.Context, job Job, h Handler, factory distlock.DistLockFactory) <-chan struct{} {
	ctx = appctx.OrBackground(ctx)
	if n := acquireJobName(job.Name); n > 1 {
		logger.Warn(ctx, "cronjob.start.duplicate_name", "name", job.Name, "active", n)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer releaseJobName(job.Name)
		runScheduler(ctx, job, h, factory)
	}()
	return done
}

// closedDone 返回已关闭的调度结束信号
func closedDone() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

// runScheduler 创建 cron、注册任务，直至 ctx 取消后 Stop
func runScheduler(ctx context.Context, job Job, h Handler, factory distlock.DistLockFactory) {
	c := cron.New(cron.WithSeconds())
	_, err := c.AddFunc(job.Spec, func() {
		fire(ctx, job, h, factory)
	})
	if err != nil {
		// validate 已解析 Spec；此处失败属防御性日志
		logger.Error(ctx, "cronjob.add_func.failed", "name", job.Name, "error", err)
		return
	}

	c.Start()
	logger.Info(ctx, fmt.Sprintf("CronJob[%s] started.", job.Name), "spec", job.Spec)

	<-ctx.Done()
	stopCtx := c.Stop()
	<-stopCtx.Done()
	logger.Info(ctx, fmt.Sprintf("CronJob[%s] stopped.", job.Name))
}

// fire 单次触发：TryLock 互斥后按 MaxRunDuration 执行 Handler
func fire(ctx context.Context, job Job, h Handler, factory distlock.DistLockFactory) {
	opts := []distlock.LockOption{
		distlock.WithTTL(defaultExecLockTTL),
		distlock.WithWatchdog(true),
		// MaxRunDuration 只跟随 Job 配置，显式覆盖工厂级 MaxHoldDuration 默认值
		distlock.WithMaxHoldDuration(job.MaxRunDuration),
	}
	lock := factory.GetLock(execLockKey(job.Name), opts...)
	held, err := lock.TryLock(ctx)
	if err != nil {
		logger.Error(ctx, "cronjob.trylock.failed", "name", job.Name, "error", err)
		return
	}
	if !held {
		if shouldWarnSkip(job.Name) {
			logger.Warn(ctx, "cronjob.skip.lock_not_held", "name", job.Name)
		} else {
			logger.Debug(ctx, "cronjob.skip.lock_not_held", "name", job.Name)
		}
		return
	}

	defer func() {
		unlockCtx, cancel := appctx.BackgroundWithTimeout(5 * time.Second)
		defer cancel()
		unlockErr := lock.Unlock(unlockCtx)
		switch {
		case unlockErr == nil:
		case errors.Is(unlockErr, distlock.ErrLockNotHeld):
			// 本调用点必然获取过锁，NotHeld 说明锁已在 Redis 侧丢失
			logger.Warn(ctx, "cronjob.unlock.lock_not_held", "name", job.Name)
		default:
			logger.Error(ctx, "cronjob.unlock.failed", "name", job.Name, "error", unlockErr)
		}
	}()

	// 锁丢失或达到 MaxRunDuration 时 cancel runCtx，让 Handler 感知并尽快退出
	runCtx, stop := distlock.BindHeldContext(ctx, lock)
	defer stop()
	if job.MaxRunDuration > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(runCtx, job.MaxRunDuration)
		defer cancel()
	}
	runHandlerSafely(runCtx, job.Name, h)
	if job.MaxRunDuration > 0 && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		logger.Warn(ctx, "cronjob.handler.timeout", "name", job.Name, "max_run_duration", job.MaxRunDuration.String())
	}
}

// runHandlerSafely 执行 Handler 并 recover panic，避免未捕获 panic 导致进程退出
func runHandlerSafely(ctx context.Context, name string, h Handler) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			logger.Error(ctx, fmt.Sprintf("CronJob[%s] panic.", name),
				"panic", r,
				"stack", string(debug.Stack()),
				"duration", time.Since(start).String(),
			)
		}
	}()
	if err := h(ctx); err != nil {
		logger.Error(ctx, "cronjob.handler.failed",
			"name", name,
			"error", err,
			"duration", time.Since(start).String(),
		)
	}
}

// execLockKey 构造执行锁 Redis Key
func execLockKey(name string) string {
	return execLockKeyPrefix + name
}

// acquireJobName 记录进程内活跃 Job 名并返回同名数量
func acquireJobName(name string) int {
	jobNamesMu.Lock()
	defer jobNamesMu.Unlock()
	st := jobNames[name]
	if st == nil {
		st = &jobNameState{}
		jobNames[name] = st
	}
	st.active++
	return st.active
}

// releaseJobName 释放进程内活跃 Job 名引用
func releaseJobName(name string) {
	jobNamesMu.Lock()
	defer jobNamesMu.Unlock()
	st := jobNames[name]
	if st == nil {
		return
	}
	st.active--
	if st.active <= 0 {
		delete(jobNames, name)
	}
}

// shouldWarnSkip 判断抢锁失败是否输出 Warn；同一 Job 至少间隔 skipWarnInterval。
// 未注册的 Job（直接调用 fire 的测试场景）不节流。
func shouldWarnSkip(name string) bool {
	jobNamesMu.Lock()
	defer jobNamesMu.Unlock()
	st := jobNames[name]
	if st == nil {
		return true
	}
	now := time.Now()
	if !st.lastSkipWarn.IsZero() && now.Sub(st.lastSkipWarn) < skipWarnInterval {
		return false
	}
	st.lastSkipWarn = now
	return true
}
