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
func (r *OutboxRepo) Update(ctx context.Context, row *model.SysEventOutbox) error {
	if err := r.UpdateAll(ctx, row); err != nil {
		return errors.Wrap(err, "更新发件箱记录失败")
	}
	return nil
}

// UpdateBatch 批量全量更新发件箱记录（按固定分片提交，避免单条 SQL 过大）
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

// FindByIDs 按主键批量查询
func (r *OutboxRepo) FindByIDs(ctx context.Context, ids []int64) ([]model.SysEventOutbox, error) {
	var rows []model.SysEventOutbox
	if err := r.BaseRepository.FindByIDs(ctx, ids, &model.SysEventOutbox{}, &rows); err != nil {
		return nil, errors.Wrap(err, "批量查询发件箱记录失败")
	}
	return rows, nil
}
