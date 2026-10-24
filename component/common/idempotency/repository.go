// Package idempotency 提供 sys_biz 幂等表仓储
//
// @author zwei
package idempotency

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/tx"
	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 提供 sys_biz 表访问
type Repository struct {
	DB *gorm.DB
}

// NewRepository 创建仓储
func NewRepository(dbConn *gorm.DB) *Repository {
	return &Repository{DB: dbConn}
}

// TryInsert 以「冲突即空操作」语义插入幂等记录以抢占处理权
//
// 抢占成功返回 claimed=true；唯一键 uk_biz(biz_id, biz_sub_id) 冲突时返回 claimed=false 且
// err 为 nil。冲突不报错、事务不中止——PG 的语句级中止会让同事务的后续兜底查询必然失败
// （25P02 transaction aborted），因此统一改用：PG 为 ON CONFLICT DO NOTHING，
// MySQL 为 ON DUPLICATE KEY UPDATE 主键自赋值的空更新，两个方言行为一致。
// 注意：MySQL DSN 若开启 clientFoundRows，冲突的 RowsAffected 为 1 会被误判为抢占成功，
// 后续 MarkCompleted 按 id 校验 RowsAffected 会兜底报错回滚，业务不会重复执行。
func (r *Repository) TryInsert(ctx context.Context, row *SysBiz) (bool, error) {
	result := tx.Exec(ctx, r.DB).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "biz_id"}, {Name: "biz_sub_id"}},
		DoNothing: true,
	}).Create(row)
	if result.Error != nil {
		return false, errors.Wrap(result.Error, "插入幂等记录失败")
	}
	return result.RowsAffected > 0, nil
}

// FindByKey 按幂等键查询，记录不存在时返回 nil, nil
func (r *Repository) FindByKey(ctx context.Context, bizID, bizSubID string) (*SysBiz, error) {
	var row SysBiz
	err := tx.Exec(ctx, r.DB).
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
	result := tx.Exec(ctx, r.DB).Model(&SysBiz{}).
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

// DeleteStaleProcessing 批量删除超时仍处于 Processing 的幂等记录，返回实际删除条数
//
// 先查 ID 再按 ID 删除：DELETE ... LIMIT 是 MySQL 方言语法（PG 不支持），
// DELETE 同表子查询则相反（MySQL 拒绝、PG 允许），两步写法对两个方言都成立。
func (r *Repository) DeleteStaleProcessing(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, errors.New("清理批量大小必须大于 0")
	}
	db := tx.Exec(ctx, r.DB)
	var ids []int64
	if err := db.Unscoped().Model(&SysBiz{}).
		Where("status = ? AND update_time < ?", StatusProcessing, before).
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return 0, errors.Wrap(err, "查询超时幂等记录失败")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := db.Unscoped().Where("id IN ?", ids).Delete(&SysBiz{})
	if result.Error != nil {
		return 0, errors.Wrap(result.Error, "清理超时幂等记录失败")
	}
	return result.RowsAffected, nil
}
