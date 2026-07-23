// Package cronjob 调度启动与触发执行
//
// @author zwei
package cronjob

import (
	"context"
	"fmt"
	"gitee.com/hq_project/yomi-boot/appctx"
	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	defaultExecLockTTL = 30 * time.Second // 执行锁默认过期时间（与 roundrobin 执行锁一致）
	execLockKeyPrefix  = "cronjob:lock:"  // 执行锁 Key 前缀
)

// testExecLockOpts 非空时覆盖 fire 的执行锁选项，仅供测试注入短 TTL / MaxHoldDuration
var testExecLockOpts []distlock.LockOption

// Start 校验后在后台按 Spec 触发；ctx 取消时停止调度并退出。
// 非法参数或 Spec 无法解析时打 Error 日志并直接返回。
func Start(ctx context.Context, job Job, h Handler, factory distlock.DistLockFactory) {
	if err := job.validate(h, factory); err != nil {
		logger.ErrorWithoutCtx("cronjob.start.invalid", "error", err)
		return
	}
	ctx = appctx.OrBackground(ctx)
	go runScheduler(ctx, job, h, factory)
}

// runScheduler 创建 cron、注册任务，直至 ctx 取消后 Stop
func runScheduler(ctx context.Context, job Job, h Handler, factory distlock.DistLockFactory) {
	c := cron.New(cron.WithSeconds(), cron.WithLocation(time.Local))
	_, err := c.AddFunc(job.Spec, func() {
		fire(ctx, job.Name, h, factory)
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

// fire 单次触发：TryLock 互斥后执行 Handler
func fire(ctx context.Context, name string, h Handler, factory distlock.DistLockFactory) {
	opts := []distlock.LockOption{
		distlock.WithTTL(defaultExecLockTTL),
		distlock.WithWatchdog(true),
	}
	if testExecLockOpts != nil {
		opts = testExecLockOpts
	}
	lock := factory.GetLock(execLockKey(name), opts...)
	held, err := lock.TryLock(ctx)
	if err != nil {
		logger.Error(ctx, "cronjob.trylock.failed", "name", name, "error", err)
		return
	}
	if !held {
		logger.Debug(ctx, "cronjob.skip.lock_not_held", "name", name)
		return
	}

	defer func() {
		unlockCtx, cancel := appctx.BackgroundWithTimeout(5 * time.Second)
		defer cancel()
		unlockErr := lock.Unlock(unlockCtx)
		if unlockErr != nil && !errors.Is(unlockErr, distlock.ErrLockNotHeld) {
			logger.Error(ctx, "cronjob.unlock.failed", "name", name, "error", unlockErr)
		}
	}()

	// 锁丢失时 cancel runCtx，让 Handler 可感知并尽快退出，避免多实例并发跑同一 Job
	runCtx, stop := distlock.BindHeldContext(ctx, lock)
	defer stop()
	runHandlerSafely(runCtx, name, h)
}

// runHandlerSafely 执行 Handler 并 recover panic，避免未捕获 panic 导致进程退出
func runHandlerSafely(ctx context.Context, name string, h Handler) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			logger.Error(ctx, fmt.Sprintf("CronJob[%s] panic.", name),
				"panic", r,
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
