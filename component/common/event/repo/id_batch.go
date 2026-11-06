// 主键列表分批，避免单条 SQL 占位符超过数据库上限
//
// @author zwei

package repo

import (
	"context"

	"github.com/hwwadmin/yomi-boot/component/common/db/tx"

	"gorm.io/gorm"
)

// idChunkSize 是批量 ID 操作的单片上限，避免触发数据库占位符上限（MySQL/PostgreSQL 均为 65535）
const idChunkSize = 500

// forEachIDChunk 按 idChunkSize 切分主键列表逐片执行，任一分片失败立即返回
func forEachIDChunk(ids []int64, fn func(chunk []int64) error) error {
	for start := 0; start < len(ids); start += idChunkSize {
		end := start + idChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := fn(ids[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// uniqueIDs 按首次出现顺序去重，避免跨分片重复返回同一行
func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// hardDeleteByIDs 按主键分批硬删除，跳过 GORM 软删除。调用方负责包装错误。
// 未在事务中时，任一分片失败后已执行的分片不会回滚。
func hardDeleteByIDs(ctx context.Context, db *gorm.DB, ids []int64, row any) error {
	if len(ids) == 0 {
		return nil
	}
	return forEachIDChunk(ids, func(chunk []int64) error {
		return tx.Exec(ctx, db).Unscoped().Where("id IN ?", chunk).Delete(row).Error
	})
}
