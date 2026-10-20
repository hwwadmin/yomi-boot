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

// outboxUpdateBatchSize 是批量全量更新的单批行数，避免单条 SQL 超过 max_allowed_packet
const outboxUpdateBatchSize = 200

// listSentSuccessBeforeSQL 查询已过保留期、且没有被死信引用的发送成功记录
const listSentSuccessBeforeSQL = `
SELECT o.id
FROM sys_event_outbox o
WHERE o.service_name = ?
  AND o.status = ?
  AND o.sent_time IS NOT NULL
  AND o.sent_time <= ?
  AND o.delete_time IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM sys_event_outbox_dlq d
      WHERE d.event_outbox_id = o.id
        AND d.delete_time IS NULL
  )
ORDER BY o.sent_time ASC
LIMIT ?`

// OutboxRepo 发件箱仓储
type OutboxRepo struct {
	db.BaseRepository
}

// NewOutboxRepo 创建发件箱仓储
func NewOutboxRepo(dbConn *gorm.DB) *OutboxRepo {
	return &OutboxRepo{BaseRepository: db.BaseRepository{DB: dbConn}}
}

// Insert 插入发件箱记录
func (r *OutboxRepo) Insert(ctx context.Context, row *model.SysEventOutbox) error {
	if err := r.Create(ctx, row); err != nil {
		return errors.Wrap(err, "插入发件箱记录失败")
	}
	return nil
}

// FetchPending 获取待处理事件
func (r *OutboxRepo) FetchPending(ctx context.Context, serviceName string, size int) ([]model.SysEventOutbox, error) {
	var rows []model.SysEventOutbox
	err := db.Exec(ctx, r.DB).
		Model(&model.SysEventOutbox{}).
		Where("service_name = ? AND status = ?", serviceName, model.OutboxStatusPending).
		Order("create_time ASC").
		Limit(size).
		Find(&rows).Error
	if err != nil {
		return nil, errors.Wrap(err, "查询待发件箱事件失败")
	}
	return rows, nil
}

// FetchFetchedTimeout 获取抓取超时事件
func (r *OutboxRepo) FetchFetchedTimeout(ctx context.Context, serviceName string, timeoutBefore time.Time) ([]model.SysEventOutbox, error) {
	var rows []model.SysEventOutbox
	err := db.Exec(ctx, r.DB).
		Model(&model.SysEventOutbox{}).
		Where("service_name = ? AND status = ? AND fetch_time < ?",
			serviceName, model.OutboxStatusFetched, timeoutBefore).
		Find(&rows).Error
	if err != nil {
		return nil, errors.Wrap(err, "查询抓取超时发件箱事件失败")
	}
	return rows, nil
}

// Update 全量更新发件箱记录
//
// 全列 upsert 会覆盖并发状态；状态迁移请使用 MarkFetchedBatch/MarkPendingBatch/MarkSentSuccess/MarkSentFail。
func (r *OutboxRepo) Update(ctx context.Context, row *model.SysEventOutbox) error {
	if err := r.UpdateAll(ctx, row); err != nil {
		return errors.Wrap(err, "更新发件箱记录失败")
	}
	return nil
}

// UpdateBatch 批量全量更新发件箱记录（按固定分片提交，避免单条 SQL 过大）
//
// 全列 upsert 会覆盖并发状态；状态迁移请使用 MarkFetchedBatch/MarkPendingBatch。
func (r *OutboxRepo) UpdateBatch(ctx context.Context, rows []model.SysEventOutbox) error {
	if len(rows) == 0 {
		return nil
	}
	for start := 0; start < len(rows); start += outboxUpdateBatchSize {
		end := min(start+outboxUpdateBatchSize, len(rows))
		if err := r.UpdateAllBatch(ctx, rows[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// MarkFetchedBatch 仅将仍为 PENDING 的记录批量标记为 FETCHED，返回实际更新成功的 ID
func (r *OutboxRepo) MarkFetchedBatch(ctx context.Context, ids []int64, fetchTime time.Time) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	result := db.Exec(ctx, r.DB).
		Model(&model.SysEventOutbox{}).
		Where("id IN ? AND status = ?", ids, model.OutboxStatusPending).
		Updates(map[string]any{
			"status":     model.OutboxStatusFetched,
			"fetch_time": fetchTime,
		})
	if result.Error != nil {
		return nil, errors.Wrap(result.Error, "批量标记发件箱事件为已抓取失败")
	}
	if result.RowsAffected == int64(len(ids)) {
		return ids, nil
	}

	var fetchedIDs []int64
	if err := db.Exec(ctx, r.DB).
		Model(&model.SysEventOutbox{}).
		Where("id IN ? AND status = ?", ids, model.OutboxStatusFetched).
		Pluck("id", &fetchedIDs).Error; err != nil {
		return nil, errors.Wrap(err, "查询已标记发件箱事件失败")
	}
	return fetchedIDs, nil
}

// MarkPendingBatch 仅将仍为 FETCHED 且已超时的记录批量恢复为 PENDING，返回实际更新行数
func (r *OutboxRepo) MarkPendingBatch(ctx context.Context, ids []int64, fetchTimeBefore time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := db.Exec(ctx, r.DB).
		Model(&model.SysEventOutbox{}).
		Where("id IN ? AND status = ? AND fetch_time < ?",
			ids, model.OutboxStatusFetched, fetchTimeBefore).
		Updates(map[string]any{
			"status":     model.OutboxStatusPending,
			"fetch_time": nil,
		})
	if result.Error != nil {
		return 0, errors.Wrap(result.Error, "批量重置发件箱事件为待处理失败")
	}
	return result.RowsAffected, nil
}

// MarkSentSuccess 仅将仍为 FETCHED 的记录标记为发送成功，返回是否实际更新
func (r *OutboxRepo) MarkSentSuccess(ctx context.Context, id int64, sentTime time.Time) (bool, error) {
	result := db.Exec(ctx, r.DB).
		Model(&model.SysEventOutbox{}).
		Where("id = ? AND status = ?", id, model.OutboxStatusFetched).
		Updates(map[string]any{
			"status":    model.OutboxStatusSentSuccess,
			"sent_time": sentTime,
		})
	if result.Error != nil {
		return false, errors.Wrap(result.Error, "标记发件箱事件发送成功失败")
	}
	return result.RowsAffected > 0, nil
}

// MarkSentFail 仅将仍为 FETCHED 的记录标记为发送失败，返回是否实际更新
func (r *OutboxRepo) MarkSentFail(ctx context.Context, id int64) (bool, error) {
	result := db.Exec(ctx, r.DB).
		Model(&model.SysEventOutbox{}).
		Where("id = ? AND status = ?", id, model.OutboxStatusFetched).
		Updates(map[string]any{
			"status": model.OutboxStatusSentFail,
		})
	if result.Error != nil {
		return false, errors.Wrap(result.Error, "标记发件箱事件发送失败失败")
	}
	return result.RowsAffected > 0, nil
}

// MarkSentSuccessFromDlq 将非发送成功状态的事件标记为发送成功，用于死信重投成功后的回写
func (r *OutboxRepo) MarkSentSuccessFromDlq(ctx context.Context, id int64, sentTime time.Time) (bool, error) {
	result := db.Exec(ctx, r.DB).
		Model(&model.SysEventOutbox{}).
		Where("id = ? AND status <> ?", id, model.OutboxStatusSentSuccess).
		Updates(map[string]any{
			"status":    model.OutboxStatusSentSuccess,
			"sent_time": sentTime,
		})
	if result.Error != nil {
		return false, errors.Wrap(result.Error, "标记死信发件箱事件发送成功失败")
	}
	return result.RowsAffected > 0, nil
}

// RestorePendingBatch 将指定发件箱记录批量恢复为待处理状态（仅处理 FETCHED 记录）
func (r *OutboxRepo) RestorePendingBatch(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	result := db.Exec(ctx, r.DB).
		Model(&model.SysEventOutbox{}).
		Where("id IN ? AND status = ?", ids, model.OutboxStatusFetched).
		Updates(map[string]any{
			"status":     model.OutboxStatusPending,
			"fetch_time": nil,
		})
	if result.Error != nil {
		return errors.Wrap(result.Error, "批量恢复发件箱事件为待处理失败")
	}
	return nil
}

// ListSentSuccessBefore 查询指定服务下发送成功且已过保留期的发件箱记录 ID
func (r *OutboxRepo) ListSentSuccessBefore(
	ctx context.Context,
	serviceName string,
	sentBefore time.Time,
	limit int,
) ([]int64, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []struct {
		ID int64
	}
	err := db.Exec(ctx, r.DB).
		Raw(listSentSuccessBeforeSQL, serviceName, model.OutboxStatusSentSuccess, sentBefore, limit).
		Scan(&rows).Error
	if err != nil {
		return nil, errors.Wrap(err, "查询可清理发件箱记录失败")
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	return ids, nil
}

// FindByIDs 按主键批量查询
func (r *OutboxRepo) FindByIDs(ctx context.Context, ids []int64) ([]model.SysEventOutbox, error) {
	var rows []model.SysEventOutbox
	if err := r.BaseRepository.FindByIDs(ctx, ids, &model.SysEventOutbox{}, &rows); err != nil {
		return nil, errors.Wrap(err, "批量查询发件箱记录失败")
	}
	return rows, nil
}
