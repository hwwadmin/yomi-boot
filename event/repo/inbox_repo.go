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

// listConsumedBeforeSQL 查询已过保留期、且没有被死信引用的已消费记录
const listConsumedBeforeSQL = `
SELECT i.id
FROM sys_event_inbox i
WHERE i.service_name = ?
  AND i.create_time <= ?
  AND i.delete_time IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM sys_event_inbox_dlq d
      WHERE d.service_name = i.service_name
        AND d.event_id = i.event_id
        AND d.delete_time IS NULL
  )
ORDER BY i.create_time ASC
LIMIT ?`

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

// ListConsumedBefore 查询指定服务下已消费且已过保留期的收件箱记录 ID
func (r *InboxRepo) ListConsumedBefore(
	ctx context.Context,
	serviceName string,
	createdBefore time.Time,
	limit int,
) ([]int64, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []struct {
		ID int64
	}
	err := db.Exec(ctx, r.DB).
		Raw(listConsumedBeforeSQL, serviceName, createdBefore, limit).
		Scan(&rows).Error
	if err != nil {
		return nil, errors.Wrap(err, "查询可清理收件箱记录失败")
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	return ids, nil
}
