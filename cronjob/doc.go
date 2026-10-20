// Package cronjob 提供业务日历定时任务：cron 表达式触发 + distlock 集群互斥。
//
// 内部固定间隔轮询请使用 roundrobin（Outbox 等）；本包面向业务日历任务。
// 业务代码禁止直接依赖 robfig/cron，须经本包 Start / MustStart 注册。
//
// # 基本用法
//
//	done := cronjob.Start(runCtx, cronjob.Job{
//	    Name: "order-service:settle.daily",
//	    Spec: "0 0 2 * * *", // 秒 分 时 日 月 周
//	}, settleDaily, distLockFactory)
//	// 停止：cancel(runCtx)；等待调度与在途 Handler 结束：<-done
//
// # 语义
//
//   - Spec 为 6 段（含秒）；默认时区 time.Local，可在 Spec 前加 TZ=/CRON_TZ= 指定
//   - 每次触发 TryLock(cronjob:lock:{Name})；未抢到则跳过（每个进程内每分钟最多一次 Warn，其余 Debug）
//   - MaxRunDuration 为单次 Handler 最长执行时间；0 表示不限制，始终以 Job 配置为准（覆盖工厂默认值），到期 cancel Handler ctx 并停止续锁
//   - 至少一次：崩溃、锁过期或故障恢复后可能重跑，Handler 必须幂等
//   - 锁丢失且 Handler 不响应 ctx 时可能出现短暂重叠执行
//   - Handler error / panic 仅记日志，不拖死进程
//   - 生命周期仅认 ctx；挂 plugin / Cleanup 由调用方拼装
//   - Start / MustStart 返回的 done 在调度停止且在途 Handler 结束后关闭
//
// @author zwei
package cronjob
