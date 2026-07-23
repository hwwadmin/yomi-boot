// Package idempotency 提供 sys_biz 幂等表仓储
//
// @author zwei
package idempotency

import (
	"context"
	"time"
	"yt-boot/db"
	"yt-boot/errors"

	"gorm.io/gorm"
)

// Repository 提供 sys_biz 表访问
type Repository struct {
	DB *gorm.DB
}

// NewRepository 创建仓储
func NewRepository(dbConn *gorm.DB) *Repository {
	return &Repository{DB: dbConn}
}

// Insert 插入幂等记录
func (r *Repository) Insert(ctx context.Context, row *SysBiz) error {
	if err := db.Exec(ctx, r.DB).Create(row).Error; err != nil {
		return errors.Wrap(err, "插入幂等记录失败")
	}
	return nil
}

// FindByKey 按幂等键查询，记录不存在时返回 nil, nil
func (r *Repository) FindByKey(ctx context.Context, bizID, bizSubID string) (*SysBiz, error) {
	var row SysBiz
	err := db.Exec(ctx, r.DB).
		Where("biz_id = ? AND biz_sub_id = ?", bizID, bizSubID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "查询幂等记录失败")
	}
	return &row, nil
}

// MarkCompleted 标记完成并缓存响应
func (r *Repository) MarkCompleted(ctx context.Context, id int64, responseData []byte) error {
	result := db.Exec(ctx, r.DB).Model(&SysBiz{}).
		Where("id = ? AND status = ?", id, StatusProcessing).
		Updates(map[string]any{
			"status":        StatusCompleted,
			"response_data": responseData,
		})
	if result.Error != nil {
		return errors.Wrap(result.Error, "更新幂等记录状态失败")
	}
	if result.RowsAffected == 0 {
		return errors.New("幂等记录状态更新未生效")
	}
	return nil
}

// DeleteStaleProcessing 批量删除超时仍处于 Processing 的幂等记录
func (r *Repository) DeleteStaleProcessing(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, errors.New("清理批量大小必须大于 0")
	}
	result := db.Exec(ctx, r.DB).Unscoped().
		Where("status = ? AND update_time < ?", StatusProcessing, before).
		Limit(limit).
		Delete(&SysBiz{})
	if result.Error != nil {
		return 0, errors.Wrap(result.Error, "清理超时幂等记录失败")
	}
	return result.RowsAffected, nil
}
