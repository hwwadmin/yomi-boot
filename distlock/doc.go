// Package distlock 提供基于 redsync 的分布式互斥锁，支持看门狗自动续期、锁丢失检测与优雅释放。
//
// # 基本用法
//
// 使用工厂模式创建锁实例，每个实例绑定一个 Redis Key（由调用方完整构造）：
//
//	factory, err := NewFactory(redisClient)
//	lock := factory.GetLock("my-key", WithTTL(30*time.Second))
//	if err := lock.Lock(ctx); err != nil {
//	    return err
//	}
//	defer lock.Unlock(ctx)
//
// # 看门狗模式
//
// 启用看门狗（WithWatchdog）后，后台会周期性续期，直至 Unlock、续期连续失败或达到 MaxHoldDuration。
// 推荐使用 RunWithLock 作为统一入口，其传入的 ctx 在锁丢失时会被 cancel：
//
//	err := RunWithLock(ctx, lock, func(runCtx context.Context) error {
//	    select {
//	    case <-runCtx.Done():
//	        return runCtx.Err() // 锁已丢失
//	    default:
//	        // 执行业务逻辑
//	        return nil
//	    }
//	})
//	if errors.Is(err, ErrLockLost) {
//	    // 互斥保护已失效，业务结果不可信
//	}
//
// 自行 TryLock / Lock 时，请用 BindHeldContext 把 HeldContext 合并进业务 ctx：
//
//	runCtx, stop := BindHeldContext(ctx, lock)
//	defer stop()
//	// 在 runCtx 上执行业务
//
// # 并发模型
//
// 同一 Lock 实例非并发安全，应由单 goroutine 串行调用。Lock、TryLockWithTimeout
// 在获取阶段会持有内部互斥量直至成功或 ctx 结束，此期间同实例的其他方法将被阻塞。
//
// @author zwei
package distlock
