// Package repo 提供消息队列事件表仓储
//
// @author zwei
package repo

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/db"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/event/model"

	"gorm.io/gorm"
)

// OutboxDlqRepo 发件箱死信仓储
type OutboxDlqRepo struct {
	db.BaseRepository
}

// NewOutboxDlqRepo 创建发件箱死信仓储
func NewOutboxDlqRepo(dbConn *gorm.DB) *OutboxDlqRepo {
	return &OutboxDlqRepo{BaseRepository: db.BaseRepository{DB: dbConn}}
}

// Insert 插入死信记录
func (r *OutboxDlqRepo) Insert(ctx context.Context, row *model.SysEventOutboxDlq) error {
	if err := r.Create(ctx, row); err != nil {
		return errors.Wrap(err, "插入发件箱死信记录失败")
	}
	return nil
}

// Fetch 获取待重试死信（仅返回未达最大重试次数且已过退避窗口的记录）
func (r *OutboxDlqRepo) Fetch(ctx context.Context, serviceName string, size int, maxRetry int, now time.Time) ([]model.SysEventOutboxDlq, error) {
	var rows []model.SysEventOutboxDlq
	query := db.Exec(ctx, r.DB).
		Model(&model.SysEventOutboxDlq{}).
		Where("service_name = ?", serviceName)
	if maxRetry > 0 {
		query = query.Where("retry_count < ?", maxRetry)
	} else {
		query = query.Where("1 = 0")
	}
	query = query.Where("retry_after_time IS NULL OR retry_after_time <= ?", now)
	err := query.
		Order("COALESCE(retry_after_time, create_time) ASC, create_time ASC").
		Limit(size).
		Find(&rows).Error
	if err != nil {
		return nil, errors.Wrap(err, "查询发件箱死信记录失败")
	}
	return rows, nil
}

// Update 全量更新死信记录
func (r *OutboxDlqRepo) Update(ctx context.Context, row *model.SysEventOutboxDlq) error {
	if err := r.UpdateAll(ctx, row); err != nil {
		return errors.Wrap(err, "更新发件箱死信记录失败")
	}
	return nil
}

// Delete 硬删除死信记录
func (r *OutboxDlqRepo) Delete(ctx context.Context, id int64) error {
	if err := r.DeleteByID(ctx, id, &model.SysEventOutboxDlq{}); err != nil {
		return errors.Wrap(err, "删除发件箱死信记录失败")
	}
	return nil
}

// DeleteWithOutboxSuccess 事务内更新发件箱成功并删除死信
func (r *OutboxDlqRepo) DeleteWithOutboxSuccess(
	ctx context.Context,
	outbox *model.SysEventOutbox,
	dlq *model.SysEventOutboxDlq,
	sentTime time.Time,
	outboxRepo *OutboxRepo,
) error {
	outbox.Status = model.OutboxStatusSentSuccess
	outbox.SentTime = &sentTime
	if _, err := outboxRepo.MarkSentSuccessFromDlq(ctx, outbox.ID, sentTime); err != nil {
		return err
	}
	return r.Delete(ctx, dlq.ID)
}

// ResetRetryCountInTimeRange 将指定创建时间范围内的死信重试计数清零（供人工恢复）
func (r *OutboxDlqRepo) ResetRetryCountInTimeRange(
	ctx context.Context,
	serviceName string,
	from time.Time,
	to time.Time,
) (int64, error) {
	if to.Before(from) {
		return 0, errors.New("重置 DLQ 重试计数时结束时间不能早于开始时间")
	}
	result := db.Exec(ctx, r.DB).
		Model(&model.SysEventOutboxDlq{}).
		Where("service_name = ? AND create_time >= ? AND create_time <= ?", serviceName, from, to).
		Updates(map[string]any{
			"retry_count":      0,
			"retry_last_time":  nil,
			"retry_after_time": nil,
		})
	if result.Error != nil {
		return 0, errors.Wrap(result.Error, "重置发件箱死信重试计数失败")
	}
	return result.RowsAffected, nil
}
