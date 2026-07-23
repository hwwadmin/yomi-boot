// Package repo 提供消息队列事件表仓储
//
// @author zwei
package repo

import (
	"context"
	"gitee.com/hq_project/yomi-boot/db"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/event/model"
	"time"

	"gorm.io/gorm"
)

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

// UpdateBatch 批量全量更新发件箱记录
func (r *OutboxRepo) UpdateBatch(ctx context.Context, rows []model.SysEventOutbox) error {
	if len(rows) == 0 {
		return nil
	}
	if err := r.UpdateAllBatch(ctx, rows); err != nil {
		return errors.Wrap(err, "批量更新发件箱记录失败")
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
