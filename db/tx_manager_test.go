// Package db 提供本地事务管理的单元测试
package db

import (
	"context"
	"testing"
	"yomi-boot/db/dbtest"
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

// txTestModel 事务管理器单测用 GORM 模型
type txTestModel struct {
	ID   int64  `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

// TableName 返回事务单测表名
func (txTestModel) TableName() string {
	return "tx_test_model"
}
