// Package repo 提供消息队列事件表仓储
//
// @author zwei
package repo

import (
	"context"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/db/tx"
	"github.com/hwwadmin/yomi-boot/component/common/event/model"
	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/shared/enums"

	"gorm.io/gorm"
)

// outboxUpdateBatchSize 是批量全量更新的单批行数，避免单条 SQL 超过 max_allowed_packet
const outboxUpdateBatchSize = 200

// listSentSuccessBeforeSQL 查询已过保留期、且没有被死信引用的发送成功记录
const listSentSuccessBeforeSQL = `
SELECT o.id
FROM t_sys_event_outbox o
WHERE o.service_name = ?
  AND o.status = ?
  AND o.sent_time IS NOT NULL
  AND o.sent_time <= ?
  AND o.delete_time IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM t_sys_event_outbox_dlq d
      WHERE d.event_outbox_id = o.id
        AND d.delete_time IS NULL
  )
ORDER BY o.sent_time ASC
LIMIT ?`

// resetFetchedTimeoutMySQL 在库内把已超时的 FETCHED 改回 PENDING。按取走时间从早到晚，单轮不超过上限。
const resetFetchedTimeoutMySQL = `
UPDATE t_sys_event_outbox
SET status = ?, fetch_time = NULL
WHERE service_name = ?
  AND status = ?
  AND fetch_time < ?
  AND delete_time IS NULL
ORDER BY fetch_time ASC
LIMIT ?`

// resetFetchedTimeoutPostgres 与 MySQL 同一语义。PostgreSQL 没有 UPDATE ... LIMIT，上限放在子查询里，主键不回到程序。
const resetFetchedTimeoutPostgres = `
UPDATE t_sys_event_outbox
SET status = ?, fetch_time = NULL
WHERE id IN (
    SELECT id FROM (
        SELECT id
        FROM t_sys_event_outbox
        WHERE service_name = ?
          AND status = ?
          AND fetch_time < ?
          AND delete_time IS NULL
        ORDER BY fetch_time ASC
        LIMIT ?
    ) AS picked
)`

// OutboxRepo 发件箱仓储
type OutboxRepo struct {
	DB *gorm.DB
}

// NewOutboxRepo 创建发件箱仓储
func NewOutboxRepo(dbConn *gorm.DB) *OutboxRepo {
	return &OutboxRepo{DB: dbConn}
}

// Insert 插入发件箱记录
func (r *OutboxRepo) Insert(ctx context.Context, row *model.SysEventOutbox) error {
	if err := tx.Exec(ctx, r.DB).Create(row).Error; err != nil {
		return errors.Wrap(err, "插入发件箱记录失败")
	}
	return nil
}

// FetchPending 获取待处理事件
func (r *OutboxRepo) FetchPending(ctx context.Context, serviceName string, size int) ([]model.SysEventOutbox, error) {
	var rows []model.SysEventOutbox
	err := tx.Exec(ctx, r.DB).
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

// ResetFetchedTimeout 在库内把本服务已超时的 FETCHED 记录改回 PENDING，返回实际更新行数。
// 主键不读回程序。limit 是这一轮的上限，必须大于 0。按 fetch_time 从早到晚处理。
func (r *OutboxRepo) ResetFetchedTimeout(ctx context.Context, serviceName string, timeoutBefore time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, errors.New("重置条数必须大于 0")
	}
	statement, err := resetFetchedTimeoutSQL(r.DB.Name())
	if err != nil {
		return 0, err
	}
	result := tx.Exec(ctx, r.DB).Exec(
		statement,
		model.OutboxStatusPending,
		serviceName,
		model.OutboxStatusFetched,
		timeoutBefore,
		limit,
	)
	if result.Error != nil {
		return 0, errors.Wrap(result.Error, "重置 FETCHED 超时发件箱事件失败")
	}
	return result.RowsAffected, nil
}

// resetFetchedTimeoutSQL 按数据库方言返回超时回收语句。占位符顺序两边一致。
func resetFetchedTimeoutSQL(dialector string) (string, error) {
	switch enums.DbType(dialector) {
	case enums.MySQL:
		return resetFetchedTimeoutMySQL, nil
	case enums.Postgres:
		return resetFetchedTimeoutPostgres, nil
	default:
		return "", errors.Newf("不支持的数据库类型: %s", dialector)
	}
}

// Update 全量更新发件箱记录
//
// 全列 upsert 会覆盖并发状态；状态迁移请使用 MarkFetchedBatch/RestorePendingBatch/MarkSentSuccess/MarkSentFail。
func (r *OutboxRepo) Update(ctx context.Context, row *model.SysEventOutbox) error {
	if err := tx.Exec(ctx, r.DB).Save(row).Error; err != nil {
		return errors.Wrap(err, "更新发件箱记录失败")
	}
	return nil
}

// UpdateBatch 批量全量更新发件箱记录（按固定分片提交，避免单条 SQL 过大）
//
// 全列 upsert 会覆盖并发状态；状态迁移请使用 MarkFetchedBatch/RestorePendingBatch。
func (r *OutboxRepo) UpdateBatch(ctx context.Context, rows []model.SysEventOutbox) error {
	if len(rows) == 0 {
		return nil
	}
	for start := 0; start < len(rows); start += outboxUpdateBatchSize {
		end := min(start+outboxUpdateBatchSize, len(rows))
		if err := tx.Exec(ctx, r.DB).Save(rows[start:end]).Error; err != nil {
			return errors.Wrap(err, "批量全量更新发件箱记录失败")
		}
	}
	return nil
}

// MarkFetchedBatch 仅将仍为 PENDING 的记录批量标记为 FETCHED，返回实际更新成功的 ID
func (r *OutboxRepo) MarkFetchedBatch(ctx context.Context, ids []int64, fetchTime time.Time) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	result := tx.Exec(ctx, r.DB).
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
	if err := tx.Exec(ctx, r.DB).
		Model(&model.SysEventOutbox{}).
		Where("id IN ? AND status = ?", ids, model.OutboxStatusFetched).
		Pluck("id", &fetchedIDs).Error; err != nil {
		return nil, errors.Wrap(err, "查询已标记发件箱事件失败")
	}
	return fetchedIDs, nil
}

// MarkSentSuccess 仅将仍为 FETCHED 的记录标记为发送成功，返回是否实际更新
func (r *OutboxRepo) MarkSentSuccess(ctx context.Context, id int64, sentTime time.Time) (bool, error) {
	result := tx.Exec(ctx, r.DB).
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
	result := tx.Exec(ctx, r.DB).
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
	result := tx.Exec(ctx, r.DB).
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
	result := tx.Exec(ctx, r.DB).
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
	err := tx.Exec(ctx, r.DB).
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
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > idChunkSize {
		ids = uniqueIDs(ids)
	}
	rows := make([]model.SysEventOutbox, 0, len(ids))
	err := forEachIDChunk(ids, func(chunk []int64) error {
		var part []model.SysEventOutbox
		if err := tx.Exec(ctx, r.DB).Model(&model.SysEventOutbox{}).Where("id IN ?", chunk).Find(&part).Error; err != nil {
			return errors.Wrap(err, "批量查询发件箱记录失败")
		}
		rows = append(rows, part...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// DeleteBatch 按主键批量硬删除。超过 idChunkSize 时分批执行。
// 未在事务中时，任一分片失败后已执行的分片不会回滚。
func (r *OutboxRepo) DeleteBatch(ctx context.Context, ids []int64) error {
	if err := hardDeleteByIDs(ctx, r.DB, ids, &model.SysEventOutbox{}); err != nil {
		return errors.Wrap(err, "批量删除发件箱记录失败")
	}
	return nil
}
