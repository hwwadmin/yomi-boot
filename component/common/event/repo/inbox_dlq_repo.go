// Package repo 提供消息队列事件表仓储
//
// @author zwei
package repo

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/core/errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// InboxDlqRepo 收件箱死信仓储
type InboxDlqRepo struct {
	db.BaseRepository
}

// NewInboxDlqRepo 创建收件箱死信仓储
func NewInboxDlqRepo(dbConn *gorm.DB) *InboxDlqRepo {
	return &InboxDlqRepo{BaseRepository: db.BaseRepository{DB: dbConn}}
}

// Insert 插入收件箱死信记录；同一服务下 eventId 已存在时保持原记录不变
func (r *InboxDlqRepo) Insert(ctx context.Context, row *model.SysEventInboxDlq) error {
	if err := db.Exec(ctx, r.DB).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(row).Error; err != nil {
		return errors.Wrap(err, "插入收件箱死信记录失败")
	}
	return nil
}

// Fetch 获取已到重投时间的收件箱死信记录
func (r *InboxDlqRepo) Fetch(ctx context.Context, serviceName string, size int, now time.Time) ([]model.SysEventInboxDlq, error) {
	if size <= 0 {
		return nil, nil
	}
	var rows []model.SysEventInboxDlq
	err := db.Exec(ctx, r.DB).
		Model(&model.SysEventInboxDlq{}).
		Where("service_name = ? AND (retry_after_time IS NULL OR retry_after_time <= ?)", serviceName, now).
		Order("create_time ASC").
		Limit(size).
		Find(&rows).Error
	if err != nil {
		return nil, errors.Wrap(err, "查询收件箱死信记录失败")
	}
	return rows, nil
}

// UpdateRetry 更新收件箱死信的重投次数与下次可重投时间
func (r *InboxDlqRepo) UpdateRetry(
	ctx context.Context,
	id int64,
	retryCount int,
	lastTime time.Time,
	afterTime time.Time,
) error {
	err := r.UpdateByID(ctx, id, &model.SysEventInboxDlq{}, map[string]any{
		"retry_count":      retryCount,
		"retry_last_time":  lastTime,
		"retry_after_time": afterTime,
	})
	if err != nil {
		return errors.Wrap(err, "更新收件箱死信重投记录失败")
	}
	return nil
}

// Delete 硬删除收件箱死信记录
func (r *InboxDlqRepo) Delete(ctx context.Context, id int64) error {
	if err := r.DeleteByID(ctx, id, &model.SysEventInboxDlq{}); err != nil {
		return errors.Wrap(err, "删除收件箱死信记录失败")
	}
	return nil
}
