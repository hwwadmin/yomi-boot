// Package repo 提供消息队列事件表仓储
//
// @author zwei
package repo

import (
	"context"
	"gitee.com/hq_project/yomi-boot/db"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/event/model"

	"gorm.io/gorm"
)

// InboxRepo 收件箱仓储
type InboxRepo struct {
	db.BaseRepository
}

// NewInboxRepo 创建收件箱仓储
func NewInboxRepo(dbConn *gorm.DB) *InboxRepo {
	return &InboxRepo{BaseRepository: db.BaseRepository{DB: dbConn}}
}

// Insert 插入收件箱记录
func (r *InboxRepo) Insert(ctx context.Context, row *model.SysEventInbox) error {
	if err := r.Create(ctx, row); err != nil {
		return errors.Wrap(err, "插入收件箱记录失败")
	}
	return nil
}

// Exists 判断指定服务下事件 ID 是否已存在
func (r *InboxRepo) Exists(ctx context.Context, serviceName, eventID string) (bool, error) {
	if serviceName == "" || eventID == "" {
		return false, nil
	}
	var count int64
	err := db.Exec(ctx, r.DB).Model(&model.SysEventInbox{}).
		Where("service_name = ? AND event_id = ?", serviceName, eventID).
		Count(&count).Error
	if err != nil {
		return false, errors.Wrap(err, "查询收件箱事件是否存在失败")
	}
	return count > 0, nil
}
