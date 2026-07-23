// Package cronjob 提供业务日历定时任务：cron 表达式触发 + distlock 集群互斥。
//
// 内部固定间隔轮询请使用 roundrobin（Outbox 等）；本包面向业务日历任务。
// 业务代码禁止直接依赖 robfig/cron，须经本包 Start 注册。
//
// # 基本用法
//
//	cronjob.Start(runCtx, cronjob.Job{
//	    Name: "order-service:settle.daily",
//	    Spec: "0 0 2 * * *", // 秒 分 时 日 月 周
//	}, settleDaily, distLockFactory)
//	// 停止：cancel(runCtx)
//
// # 语义
//
//   - Spec 为 6 段（含秒），时区 time.Local
//   - 每次触发 TryLock(cronjob:lock:{Name})；未抢到则跳过
//   - Handler error / panic 仅记日志，不拖死进程
//   - 生命周期仅认 ctx；挂 plugin / Cleanup 由调用方拼装
//
// @author zwei
package cronjob
