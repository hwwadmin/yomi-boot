// 通用仓储基类 CRUD 方法的单元测试
//
// @author zwei

package orm

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/core/shared"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newDeadDB 构造一个必然连接失败的 gorm 客户端，用于触发数据库错误路径
func newDeadDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "root@tcp(127.0.0.1:1)/dead",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("构造失效数据库失败: %v", err)
	}
	return db
}

// TestBaseRepository_FirstByID 验证主键查询的命中与未命中
func TestBaseRepository_FirstByID(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	if err := repo.Create(ctx, &batchTestModel{ID: 1, Name: "a"}); err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}

	found, err := repo.FirstByID(ctx, 1, &batchTestModel{})
	if err != nil || !found {
		t.Fatalf("应查到记录: found=%v err=%v", found, err)
	}
	found, err = repo.FirstByID(ctx, 999, &batchTestModel{})
	if err != nil || found {
		t.Fatalf("未命中应返回 false,nil: found=%v err=%v", found, err)
	}
}

// TestBaseRepository_FirstByField 验证字段查询、非法字段名与未命中
func TestBaseRepository_FirstByField(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	if err := repo.Create(ctx, &batchTestModel{ID: 1, Name: "a"}); err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}

	found, err := repo.FirstByField(ctx, "name", "a", &batchTestModel{})
	if err != nil || !found {
		t.Fatalf("应查到记录: found=%v err=%v", found, err)
	}
	found, err = repo.FirstByField(ctx, "name", "missing", &batchTestModel{})
	if err != nil || found {
		t.Fatalf("未命中应返回 false,nil: found=%v err=%v", found, err)
	}
	if _, err := repo.FirstByField(ctx, "name; drop table x", "a", &batchTestModel{}); err == nil {
		t.Fatal("非法字段名应返回错误")
	}
}

// TestBaseRepository_FindAll 验证全表查询
func TestBaseRepository_FindAll(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	if err := repo.CreateInBatches(ctx, []batchTestModel{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}, 10); err != nil {
		t.Fatalf("批量创建失败: %v", err)
	}

	var rows []batchTestModel
	if err := repo.FindAll(ctx, &batchTestModel{}, &rows); err != nil {
		t.Fatalf("全表查询失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("期望 2 条，实际 %d 条", len(rows))
	}
}

// TestBaseRepository_ExistsByField 验证字段存在性判断与非法字段名
func TestBaseRepository_ExistsByField(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	if err := repo.Create(ctx, &batchTestModel{ID: 1, Name: "a"}); err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}

	if exists, err := repo.ExistsByField(ctx, "name", "a", &batchTestModel{}); err != nil || !exists {
		t.Fatalf("应存在: exists=%v err=%v", exists, err)
	}
	if exists, err := repo.ExistsByField(ctx, "name", "missing", &batchTestModel{}); err != nil || exists {
		t.Fatalf("不应存在: exists=%v err=%v", exists, err)
	}
	if _, err := repo.ExistsByField(ctx, "name; drop table x", "a", &batchTestModel{}); err == nil {
		t.Fatal("非法字段名应返回错误")
	}
}

// TestBaseRepository_UpdateAll 验证全量更新按主键覆盖
func TestBaseRepository_UpdateAll(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	row := batchTestModel{ID: 1, Name: "a"}
	if err := repo.Create(ctx, &row); err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}

	row.Name = "b"
	if err := repo.UpdateAll(ctx, &row); err != nil {
		t.Fatalf("全量更新失败: %v", err)
	}
	var got batchTestModel
	if err := gormDB.First(&got, 1).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if got.Name != "b" {
		t.Fatalf("全量更新未生效: %+v", got)
	}
}

// TestBaseRepository_UpdateAllBatch 验证批量全量更新
func TestBaseRepository_UpdateAllBatch(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	rows := []batchTestModel{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}
	if err := repo.CreateInBatches(ctx, rows, 10); err != nil {
		t.Fatalf("批量创建失败: %v", err)
	}

	rows[0].Name = "a2"
	rows[1].Name = "b2"
	if err := repo.UpdateAllBatch(ctx, rows); err != nil {
		t.Fatalf("批量全量更新失败: %v", err)
	}
	var got []batchTestModel
	if err := gormDB.Order("id ASC").Find(&got).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if got[0].Name != "a2" || got[1].Name != "b2" {
		t.Fatalf("批量全量更新未生效: %+v %+v", got[0], got[1])
	}
}

// TestBaseRepository_UpdateByID 验证按主键更新多字段
func TestBaseRepository_UpdateByID(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	if err := repo.Create(ctx, &batchTestModel{ID: 1, Name: "a"}); err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}

	if err := repo.UpdateByID(ctx, 1, &batchTestModel{}, map[string]any{"name": "b"}); err != nil {
		t.Fatalf("按主键更新失败: %v", err)
	}
	var got batchTestModel
	if err := gormDB.First(&got, 1).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if got.Name != "b" {
		t.Fatalf("更新未生效: %+v", got)
	}
}

// TestBaseRepository_UpdateFieldByID 验证按主键更新单字段与非法字段名
func TestBaseRepository_UpdateFieldByID(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	if err := repo.Create(ctx, &batchTestModel{ID: 1, Name: "a"}); err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}

	if err := repo.UpdateFieldByID(ctx, 1, &batchTestModel{}, "name", "b"); err != nil {
		t.Fatalf("按主键更新字段失败: %v", err)
	}
	var got batchTestModel
	if err := gormDB.First(&got, 1).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if got.Name != "b" {
		t.Fatalf("更新未生效: %+v", got)
	}
	if err := repo.UpdateFieldByID(ctx, 1, &batchTestModel{}, "name; drop table x", "b"); err == nil {
		t.Fatal("非法字段名应返回错误")
	}
}

// TestBaseRepository_UpdateBatch 验证批量更新、空列表与数据库故障
func TestBaseRepository_UpdateBatch(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	if err := repo.CreateInBatches(ctx, []batchTestModel{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}, 10); err != nil {
		t.Fatalf("批量创建失败: %v", err)
	}

	if err := repo.UpdateBatch(ctx, nil, &batchTestModel{}, map[string]any{"name": "x"}); err != nil {
		t.Fatalf("空列表应直接返回: %v", err)
	}
	if err := repo.UpdateBatch(ctx, []int64{1, 2}, &batchTestModel{}, map[string]any{"name": "c"}); err != nil {
		t.Fatalf("批量更新失败: %v", err)
	}
	var got []batchTestModel
	if err := gormDB.Order("id ASC").Find(&got).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if got[0].Name != "c" || got[1].Name != "c" {
		t.Fatalf("批量更新未生效: %+v %+v", got[0], got[1])
	}

	deadRepo := &BaseRepository{DB: newDeadDB(t)}
	if err := deadRepo.UpdateBatch(ctx, []int64{1}, &batchTestModel{}, map[string]any{"name": "x"}); err == nil {
		t.Fatal("数据库不可用时应返回错误")
	}
}

// TestBaseRepository_DeleteByID 验证硬删除跳过软删除
func TestBaseRepository_DeleteByID(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	if err := repo.Create(ctx, &batchTestModel{ID: 1, Name: "a"}); err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}

	if err := repo.DeleteByID(ctx, 1, &batchTestModel{}); err != nil {
		t.Fatalf("硬删除失败: %v", err)
	}
	var count int64
	if err := gormDB.Unscoped().Model(&batchTestModel{}).Count(&count).Error; err != nil {
		t.Fatalf("统计记录失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("硬删除后不应有残留，实际 %d 条", count)
	}
}

// TestBaseRepository_SoftDeleteByID 验证软删除后普通查询不可见、Unscoped 可见
func TestBaseRepository_SoftDeleteByID(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	if err := repo.Create(ctx, &batchTestModel{ID: 1, Name: "a"}); err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}

	if err := repo.SoftDeleteByID(ctx, 1, &batchTestModel{}); err != nil {
		t.Fatalf("软删除失败: %v", err)
	}
	found, err := repo.FirstByID(ctx, 1, &batchTestModel{})
	if err != nil || found {
		t.Fatalf("软删除后普通查询不应命中: found=%v err=%v", found, err)
	}
	var count int64
	if err := gormDB.Unscoped().Model(&batchTestModel{}).Count(&count).Error; err != nil {
		t.Fatalf("统计记录失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("软删除记录应保留，实际 %d 条", count)
	}
}

// TestBaseRepository_DeleteBatch 验证批量硬删除与数据库故障
func TestBaseRepository_DeleteBatch(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	if err := repo.CreateInBatches(ctx, []batchTestModel{{ID: 1}, {ID: 2}}, 10); err != nil {
		t.Fatalf("批量创建失败: %v", err)
	}

	if err := repo.DeleteBatch(ctx, nil, &batchTestModel{}); err != nil {
		t.Fatalf("空列表应直接返回: %v", err)
	}
	if err := repo.DeleteBatch(ctx, []int64{1, 2}, &batchTestModel{}); err != nil {
		t.Fatalf("批量硬删除失败: %v", err)
	}
	var count int64
	if err := gormDB.Unscoped().Model(&batchTestModel{}).Count(&count).Error; err != nil {
		t.Fatalf("统计记录失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("硬删除后不应有残留，实际 %d 条", count)
	}

	deadRepo := &BaseRepository{DB: newDeadDB(t)}
	if err := deadRepo.DeleteBatch(ctx, []int64{1}, &batchTestModel{}); err == nil {
		t.Fatal("数据库不可用时应返回错误")
	}
}

// TestBaseRepository_SoftDeleteBatch 验证批量软删除与数据库故障
func TestBaseRepository_SoftDeleteBatch(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	repo := &BaseRepository{DB: gormDB}
	ctx := context.Background()
	if err := repo.CreateInBatches(ctx, []batchTestModel{{ID: 1}, {ID: 2}}, 10); err != nil {
		t.Fatalf("批量创建失败: %v", err)
	}

	if err := repo.SoftDeleteBatch(ctx, nil, &batchTestModel{}); err != nil {
		t.Fatalf("空列表应直接返回: %v", err)
	}
	if err := repo.SoftDeleteBatch(ctx, []int64{1, 2}, &batchTestModel{}); err != nil {
		t.Fatalf("批量软删除失败: %v", err)
	}
	found, err := repo.FirstByID(ctx, 1, &batchTestModel{})
	if err != nil || found {
		t.Fatalf("软删除后普通查询不应命中: found=%v err=%v", found, err)
	}
	var count int64
	if err := gormDB.Unscoped().Model(&batchTestModel{}).Count(&count).Error; err != nil {
		t.Fatalf("统计记录失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("软删除记录应保留，实际 %d 条", count)
	}

	deadRepo := &BaseRepository{DB: newDeadDB(t)}
	if err := deadRepo.SoftDeleteBatch(ctx, []int64{1}, &batchTestModel{}); err == nil {
		t.Fatal("数据库不可用时应返回错误")
	}
}

// TestApplyPage_DefaultsAndClamping 验证分页参数的默认值与上限截断
func TestApplyPage_DefaultsAndClamping(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()

	rows := make([]batchTestModel, 0, shared.MaxPageSize+5)
	for i := 1; i <= shared.MaxPageSize+5; i++ {
		rows = append(rows, batchTestModel{ID: int64(i), Name: "n"})
	}
	if err := gormDB.CreateInBatches(&rows, 25).Error; err != nil {
		t.Fatalf("初始化数据失败: %v", err)
	}

	// page/pageSize 非法时取默认值：等价于第 1 页每页 10 条
	var first []batchTestModel
	paged, err := ApplyPage(gormDB.Model(&batchTestModel{}), 0, 0, OrderAsc("id"))
	if err != nil {
		t.Fatalf("ApplyPage 失败: %v", err)
	}
	if err := paged.Find(&first).Error; err != nil {
		t.Fatalf("分页查询失败: %v", err)
	}
	if len(first) != 10 || first[0].ID != 1 {
		t.Fatalf("非法参数应取默认第 1 页 10 条，实际 %d 条首条 ID %d", len(first), first[0].ID)
	}

	// pageSize 超过上限时截断为 MaxPageSize
	var clamped []batchTestModel
	paged, err = ApplyPage(gormDB.Model(&batchTestModel{}), 1, 999999, OrderAsc("id"))
	if err != nil {
		t.Fatalf("ApplyPage 失败: %v", err)
	}
	if err := paged.Find(&clamped).Error; err != nil {
		t.Fatalf("分页查询失败: %v", err)
	}
	if len(clamped) != shared.MaxPageSize {
		t.Fatalf("超限 pageSize 应截断为 %d，实际 %d 条", shared.MaxPageSize, len(clamped))
	}

	// 指定第 3 页：偏移 20 条
	var third []batchTestModel
	paged, err = ApplyPage(gormDB.Model(&batchTestModel{}), 3, 10, OrderAsc("id"))
	if err != nil {
		t.Fatalf("ApplyPage 失败: %v", err)
	}
	if err := paged.Find(&third).Error; err != nil {
		t.Fatalf("分页查询失败: %v", err)
	}
	if len(third) != 10 || third[0].ID != 21 {
		t.Fatalf("第 3 页期望偏移 20 条，实际 %d 条首条 ID %d", len(third), third[0].ID)
	}
}

// TestBaseRepository_CountPageErrors 验证统计与排序参数故障时的错误透传
func TestBaseRepository_CountPageErrors(t *testing.T) {
	ctx := context.Background()

	deadRepo := &BaseRepository{DB: newDeadDB(t)}
	query := deadRepo.DB.Model(&batchTestModel{})
	var rows []batchTestModel
	if _, err := deadRepo.CountPage(ctx, query, 1, 10, &rows); err == nil {
		t.Fatal("数据库不可用时统计应返回错误")
	}

	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(batchTestModel{}))
	defer cleanup()
	repo := &BaseRepository{DB: gormDB}
	// 表中须有数据使 Count 通过，才会走到排序校验
	if err := repo.Create(ctx, &batchTestModel{ID: 1, Name: "a"}); err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}
	if _, err := repo.CountPage(ctx, gormDB.Model(&batchTestModel{}), 1, 10, &rows,
		OrderClause{Column: "name; drop table x"}); err == nil {
		t.Fatal("排序字段非法应返回错误")
	}
}
