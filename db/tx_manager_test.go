// Package db 提供本地事务管理的单元测试
package db

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/db/dbtest"
	"gitee.com/hq_project/yomi-boot/errors"
)

// TestTxManager_Execute_CommitAndRollback 验证事务提交与回滚行为
func TestTxManager_Execute_CommitAndRollback(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanup()

	type countRow struct {
		C int64 `gorm:"column:c"`
	}

	manager := NewTxManager(gormDB)
	ctx := context.Background()

	err := manager.Execute(ctx, func(txCtx context.Context) error {
		return Exec(txCtx, gormDB).Create(&txTestModel{ID: 1, Name: "committed"}).Error
	})
	if err != nil {
		t.Fatalf("提交事务失败: %v", err)
	}

	var committed countRow
	if err := gormDB.Raw("SELECT COUNT(*) AS c FROM tx_test_model WHERE id = ?", 1).Scan(&committed).Error; err != nil {
		t.Fatalf("查询提交结果失败: %v", err)
	}
	if committed.C != 1 {
		t.Fatalf("提交后期望 1 条记录，实际 %d", committed.C)
	}

	rollbackErr := manager.Execute(ctx, func(txCtx context.Context) error {
		if err := Exec(txCtx, gormDB).Create(&txTestModel{ID: 2, Name: "rolled-back"}).Error; err != nil {
			return err
		}
		return context.Canceled
	})
	if rollbackErr == nil {
		t.Fatal("期望事务回滚返回错误")
	}

	var rolledBack countRow
	if err := gormDB.Raw("SELECT COUNT(*) AS c FROM tx_test_model WHERE id = ?", 2).Scan(&rolledBack).Error; err != nil {
		t.Fatalf("查询回滚结果失败: %v", err)
	}
	if rolledBack.C != 0 {
		t.Fatalf("回滚后期望 0 条记录，实际 %d", rolledBack.C)
	}
}

// TestTxManager_Execute_NestedCommit 验证嵌套 Execute 复用同一事务，内外层写入随外层一并提交
func TestTxManager_Execute_NestedCommit(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanup()

	manager := NewTxManager(gormDB)
	ctx := context.Background()

	err := manager.Execute(ctx, func(outerCtx context.Context) error {
		if err := Exec(outerCtx, gormDB).Create(&txTestModel{ID: 11, Name: "outer"}).Error; err != nil {
			return err
		}
		return manager.Execute(outerCtx, func(innerCtx context.Context) error {
			return Exec(innerCtx, gormDB).Create(&txTestModel{ID: 12, Name: "inner"}).Error
		})
	})
	if err != nil {
		t.Fatalf("嵌套事务提交失败: %v", err)
	}

	var count int64
	if err := gormDB.Model(&txTestModel{}).Where("id IN ?", []int64{11, 12}).Count(&count).Error; err != nil {
		t.Fatalf("查询提交结果失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("期望内外层各 1 条，实际 %d 条", count)
	}
}

// TestTxManager_Execute_NestedRollback 验证嵌套 Execute 不新开事务：外层回滚时内层写入一并回滚
func TestTxManager_Execute_NestedRollback(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanup()

	manager := NewTxManager(gormDB)
	ctx := context.Background()

	err := manager.Execute(ctx, func(outerCtx context.Context) error {
		if err := Exec(outerCtx, gormDB).Create(&txTestModel{ID: 21, Name: "outer"}).Error; err != nil {
			return err
		}
		if err := manager.Execute(outerCtx, func(innerCtx context.Context) error {
			return Exec(innerCtx, gormDB).Create(&txTestModel{ID: 22, Name: "inner"}).Error
		}); err != nil {
			return err
		}
		return errors.New("外层主动回滚")
	})
	if err == nil {
		t.Fatal("外层返回错误时期望 Execute 返回错误")
	}

	var count int64
	if err := gormDB.Model(&txTestModel{}).Where("id IN ?", []int64{21, 22}).Count(&count).Error; err != nil {
		t.Fatalf("查询回滚结果失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("嵌套事务应随外层回滚，实际残留 %d 条", count)
	}
}

// TestExec_AppliesCallerCtxInsideTx 验证事务内 Exec 使用调用方 ctx：已取消的 ctx 不得写库
func TestExec_AppliesCallerCtxInsideTx(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanup()

	manager := NewTxManager(gormDB)

	err := manager.Execute(context.Background(), func(txCtx context.Context) error {
		canceled, cancel := context.WithCancel(txCtx)
		cancel()
		return Exec(canceled, gormDB).Create(&txTestModel{ID: 31, Name: "canceled"}).Error
	})
	if err == nil {
		t.Fatal("事务内使用已取消的 ctx 仍写入成功")
	}

	var count int64
	if err := gormDB.Model(&txTestModel{}).Where("id = ?", 31).Count(&count).Error; err != nil {
		t.Fatalf("查询回滚结果失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("已取消 ctx 不应落库，实际 %d 条", count)
	}
}

// txTestModel 事务管理器单测用 GORM 模型
type txTestModel struct {
	ID   int64  `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

// TableName 返回事务单测表名
func (txTestModel) TableName() string {
	return "tx_test_model"
}
