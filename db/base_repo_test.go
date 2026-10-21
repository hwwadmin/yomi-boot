// 通用仓储基类的单元测试
//
// @author zwei

package db

import (
	"context"
	"strings"
	"testing"

	"gitee.com/hq_project/yomi-boot/db/dbtest"
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
	ID   int64  `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

// TableName 返回批量创建单测表名
func (batchTestModel) TableName() string { return "batch_test_model" }
