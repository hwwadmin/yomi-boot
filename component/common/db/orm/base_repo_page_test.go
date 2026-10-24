// CountPage 分页行为单测
//
// @author zwei

package orm

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
)

// TestCountPage_ZeroTotalClearsDest 验证 total=0 时清空调用方复用的结果切片
func TestCountPage_ZeroTotalClearsDest(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(pageTestModel{}), dbtest.WithData(
		pageTestModel{ID: 1, Name: "alpha"},
	))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()

	// 先查到有数据，故意复用同一切片
	rows := []pageTestModel{{ID: 99, Name: "stale"}}
	total, err := repo.CountPage(ctx, gormDB.Model(&pageTestModel{}), 1, 10, &rows)
	if err != nil {
		t.Fatalf("有数据分页失败: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].Name != "alpha" {
		t.Fatalf("有数据分页期望 1 条 alpha，实际 total=%d rows=%+v", total, rows)
	}

	// 条件无命中时，rows 须被清空而非残留 alpha
	total, err = repo.CountPage(ctx, gormDB.Model(&pageTestModel{}).Where("name = ?", "missing"), 1, 10, &rows)
	if err != nil {
		t.Fatalf("零结果分页失败: %v", err)
	}
	if total != 0 {
		t.Fatalf("零结果期望 total=0，实际 %d", total)
	}
	if len(rows) != 0 {
		t.Fatalf("零结果期望清空 dest，实际 len=%d rows=%+v", len(rows), rows)
	}
}

// TestResetPageDest_RequiresPointer 验证 dest 非指针时返回错误
func TestResetPageDest_RequiresPointer(t *testing.T) {
	t.Parallel()

	var rows []pageTestModel
	if err := resetPageDest(rows); err == nil {
		t.Fatal("非指针 dest 应返回错误")
	}
}

// pageTestModel 分页单测用 GORM 模型
type pageTestModel struct {
	ID   int64  `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

// TableName 返回分页单测表名
func (pageTestModel) TableName() string {
	return "page_test_model"
}
