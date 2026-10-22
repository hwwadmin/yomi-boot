// Package event 提供发件箱/收件箱过期数据清理能力
//
// @author zwei
package event

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/component/distlock"
	"gitee.com/hq_project/yomi-boot/component/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/event/model"
	"gitee.com/hq_project/yomi-boot/component/event/repo"
	"gitee.com/hq_project/yomi-boot/component/roundrobin"
	"gitee.com/hq_project/yomi-boot/core/errors"
	"gitee.com/hq_project/yomi-boot/core/logger"
)

// RetentionCleaner 按配置清理发件箱/收件箱过期数据
type RetentionCleaner struct {
	cfg        cfg.Runtime
	outboxRepo *repo.OutboxRepo
	inboxRepo  *repo.InboxRepo
}

// NewRetentionCleaner 创建过期数据清理器
func NewRetentionCleaner(
	runtime cfg.Runtime,
	outboxRepo *repo.OutboxRepo,
	inboxRepo *repo.InboxRepo,
) *RetentionCleaner {
	return &RetentionCleaner{
		cfg:        runtime,
		outboxRepo: outboxRepo,
		inboxRepo:  inboxRepo,
	}
}

// Start 启动已启用的清理任务；启动失败时返回错误
func (c *RetentionCleaner) Start(ctx context.Context, factory distlock.DistLockFactory) error {
	if c == nil {
		return nil
	}
	if factory == nil {
		return errors.New("event: 分布式锁工厂未初始化")
	}

	started := false
	if c.cfg.OutboxCleanup.Enabled {
		if c.outboxRepo == nil {
			return errors.New("event: 发件箱仓储未初始化")
		}
		if err := validateCleanupRuntime("outbox", c.cfg.OutboxCleanup); err != nil {
			return err
		}
		if err := startCleanupTask(
			ctx, factory, c.cfg.RoundRobinName("MqEventOutboxCleanup"), c.cfg.OutboxCleanup, c.cleanOutbox,
		); err != nil {
			return err
		}
		started = true
	}

	if c.cfg.InboxCleanup.Enabled {
		if c.inboxRepo == nil {
			return errors.New("event: 收件箱仓储未初始化")
		}
		if err := validateCleanupRuntime("inbox", c.cfg.InboxCleanup); err != nil {
			return err
		}
		if err := startCleanupTask(
			ctx, factory, c.cfg.RoundRobinName("MqEventInboxCleanup"), c.cfg.InboxCleanup, c.cleanInbox,
		); err != nil {
			return err
		}
		started = true
	}

	if !started {
		logger.InfoWithoutCtx("[消息队列事件]发件箱/收件箱清理未启用")
	}
	return nil
}

// startCleanupTask 启动单个过期数据清理任务
func startCleanupTask(
	ctx context.Context,
	factory distlock.DistLockFactory,
	name string,
	cleanup cfg.CleanupRuntime,
	task roundrobin.Task,
) error {
	if err := roundrobin.Start(ctx, roundrobin.Simple(name, cleanup.Frequency), task, factory); err != nil {
		return errors.Wrap(err, "event: 启动过期数据清理失败(%s)", name)
	}
	logger.InfoWithoutCtx("[消息队列事件]过期数据清理已启动",
		"name", name,
		"retentionDays", cleanupRetentionDays(cleanup.Retention),
		"frequency", cleanup.Frequency.String(),
		"batchSize", cleanup.BatchSize,
		"maxBatches", cleanup.MaxBatches,
	)
	return nil
}

// cleanOutbox 执行一轮发件箱过期数据清理
func (c *RetentionCleaner) cleanOutbox(ctx context.Context) error {
	return c.clean(ctx, "outbox", c.cfg.OutboxCleanup,
		func(ctx context.Context, cutoff time.Time, limit int) ([]int64, error) {
			return c.outboxRepo.ListSentSuccessBefore(ctx, c.cfg.ServiceName, cutoff, limit)
		},
		func(ctx context.Context, ids []int64) error {
			return c.outboxRepo.DeleteBatch(ctx, ids, &model.SysEventOutbox{})
		},
	)
}

// cleanInbox 执行一轮收件箱过期数据清理
func (c *RetentionCleaner) cleanInbox(ctx context.Context) error {
	return c.clean(ctx, "inbox", c.cfg.InboxCleanup,
		func(ctx context.Context, cutoff time.Time, limit int) ([]int64, error) {
			return c.inboxRepo.ListConsumedBefore(ctx, c.cfg.ServiceName, cutoff, limit)
		},
		func(ctx context.Context, ids []int64) error {
			return c.inboxRepo.DeleteBatch(ctx, ids, &model.SysEventInbox{})
		},
	)
}

// clean 按批查询并硬删除过期数据
func (c *RetentionCleaner) clean(
	ctx context.Context,
	name string,
	cleanup cfg.CleanupRuntime,
	list func(context.Context, time.Time, int) ([]int64, error),
	deleteRows func(context.Context, []int64) error,
) error {
	cutoff := time.Now().Add(-cleanup.Retention)
	deleted := 0
	for batch := 0; batch < cleanup.MaxBatches; batch++ {
		ids, err := list(ctx, cutoff, cleanup.BatchSize)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			break
		}
		if err := deleteRows(ctx, ids); err != nil {
			return err
		}
		deleted += len(ids)
		if len(ids) < cleanup.BatchSize {
			break
		}
	}
	if deleted > 0 {
		logger.Info(ctx, "[消息队列事件]过期数据清理完成",
			"table", name,
			"deleted", deleted,
			"retentionDays", cleanupRetentionDays(cleanup.Retention),
		)
	}
	if deleted == cleanup.MaxBatches*cleanup.BatchSize {
		logger.Warn(ctx, "[消息队列事件]本轮清理达到最大批数，可能仍有数据待下一轮处理",
			"table", name, "deleted", deleted)
	}
	return nil
}

// validateCleanupRuntime 校验单表清理运行时配置
func validateCleanupRuntime(name string, cleanup cfg.CleanupRuntime) error {
	if cleanup.Retention <= 0 {
		return errors.Newf("event: %s cleanup retention 必须大于 0", name)
	}
	if cleanup.Frequency < roundrobin.MinInterval {
		return errors.Newf("event: %s cleanup frequency 必须大于等于 %s", name, roundrobin.MinInterval.String())
	}
	if cleanup.BatchSize <= 0 {
		return errors.Newf("event: %s cleanup batch_size 必须大于 0", name)
	}
	if cleanup.MaxBatches <= 0 {
		return errors.Newf("event: %s cleanup max_batches 必须大于 0", name)
	}
	return nil
}

// cleanupRetentionDays 将保留时长转换为天数，仅用于日志
func cleanupRetentionDays(retention time.Duration) int {
	if retention <= 0 {
		return 0
	}
	return int(retention / (24 * time.Hour))
}
