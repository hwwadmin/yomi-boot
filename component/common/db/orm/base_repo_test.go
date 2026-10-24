// 通用仓储基类的单元测试
//
// @author zwei

package orm

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestCreateInBatches_RejectsNonPositiveBatchSize 验证分片大小非正时返回明确错误（负数不得 panic）
func TestCreateInBatches_RejectsNonPositiveBatchSize(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	for _, size := range []int{0, -1} {
		err := repo.CreateInBatches(context.Background(), []batchTestModel{{ID: 1}}, size)
		if err == nil {
			t.Fatalf("batchSize=%d 期望报错", size)
		}
		if !strings.Contains(err.Error(), "batchSize 必须大于 0") {
			t.Fatalf("batchSize=%d 错误信息不明确: %v", size, err)
		}
	}
}

// TestCreateInBatches_ValidBatchSize 验证合法分片大小可正常写入
func TestCreateInBatches_ValidBatchSize(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	rows := []batchTestModel{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}
	if err := repo.CreateInBatches(context.Background(), rows, 2); err != nil {
		t.Fatalf("批量创建失败: %v", err)
	}

	var count int64
	if err := gormDB.Model(&batchTestModel{}).Count(&count).Error; err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("期望 2 条，实际 %d", count)
	}
}

// batchTestModel 批量创建单测用 GORM 模型
type batchTestModel struct {
	ID         int64          `gorm:"column:id;primaryKey"`
	Name       string         `gorm:"column:name"`
	DeleteTime gorm.DeletedAt `gorm:"column:delete_time;index"`
}

// TableName 返回批量创建单测表名
func (batchTestModel) TableName() string { return "batch_test_model" }

// TestBaseRepository_ChunksIDBatches 验证批量 ID 方法按 idChunkSize 自动分批
func TestBaseRepository_ChunksIDBatches(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()

	total := idChunkSize + 1
	rows := make([]batchTestModel, 0, total)
	ids := make([]int64, 0, total)
	for i := 1; i <= total; i++ {
		rows = append(rows, batchTestModel{ID: int64(i), Name: "init"})
		ids = append(ids, int64(i))
	}
	if err := repo.CreateInBatches(ctx, rows, 200); err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}

	counter := &traceCounter{}
	gormDB.Logger = counter

	t.Run("FindByIDs", func(t *testing.T) {
		counter.reset()
		var got []batchTestModel
		if err := repo.FindByIDs(ctx, ids, &batchTestModel{}, &got); err != nil {
			t.Fatalf("批量查询失败: %v", err)
		}
		if len(got) != total {
			t.Fatalf("期望 %d 条，实际 %d", total, len(got))
		}
		if n := counter.count(); n != 2 {
			t.Fatalf("期望分 2 片查询，实际 %d 次", n)
		}
	})

	t.Run("UpdateBatch", func(t *testing.T) {
		if err := repo.UpdateBatch(ctx, ids, &batchTestModel{}, map[string]any{"name": "updated"}); err != nil {
			t.Fatalf("批量更新失败: %v", err)
		}
		var n int64
		if err := gormDB.Model(&batchTestModel{}).Where("name = ?", "updated").Count(&n).Error; err != nil {
			t.Fatalf("查询更新结果失败: %v", err)
		}
		if n != int64(total) {
			t.Fatalf("期望更新 %d 条，实际 %d", total, n)
		}
	})

	t.Run("SoftDeleteBatch", func(t *testing.T) {
		if err := repo.SoftDeleteBatch(ctx, ids, &batchTestModel{}); err != nil {
			t.Fatalf("批量软删除失败: %v", err)
		}
		var live, all int64
		if err := gormDB.Model(&batchTestModel{}).Count(&live).Error; err != nil {
			t.Fatalf("查询软删除结果失败: %v", err)
		}
		if err := gormDB.Unscoped().Model(&batchTestModel{}).Count(&all).Error; err != nil {
			t.Fatalf("查询全量结果失败: %v", err)
		}
		if live != 0 || all != int64(total) {
			t.Fatalf("期望全部软删除，live=%d all=%d", live, all)
		}
	})

	t.Run("DeleteBatch", func(t *testing.T) {
		if err := repo.DeleteBatch(ctx, ids, &batchTestModel{}); err != nil {
			t.Fatalf("批量硬删除失败: %v", err)
		}
		var all int64
		if err := gormDB.Unscoped().Model(&batchTestModel{}).Count(&all).Error; err != nil {
			t.Fatalf("查询硬删除结果失败: %v", err)
		}
		if all != 0 {
			t.Fatalf("期望全部硬删除，实际残留 %d", all)
		}
	})
}

// traceCounter 统计 GORM 实际执行的语句数
type traceCounter struct {
	gormlogger.Interface
	n atomic.Int64
}

// Trace 统计一次 SQL 执行
func (c *traceCounter) Trace(context.Context, time.Time, func() (string, int64), error) {
	c.n.Add(1)
}

// LogMode 返回自身，避免 GORM 派生新实例
func (c *traceCounter) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }

// Info 忽略普通日志
func (c *traceCounter) Info(context.Context, string, ...any) {}

// Warn 忽略告警日志
func (c *traceCounter) Warn(context.Context, string, ...any) {}

// Error 忽略错误日志
func (c *traceCounter) Error(context.Context, string, ...any) {}

// reset 清零计数
func (c *traceCounter) reset() { c.n.Store(0) }

// count 返回累计语句数
func (c *traceCounter) count() int64 { return c.n.Load() }
