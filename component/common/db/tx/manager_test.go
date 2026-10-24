// 本地事务管理器的单元测试
//
// @author zwei

package tx

import (
	"context"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestManager_Execute_CommitAndRollback 验证事务提交与回滚行为
func TestManager_Execute_CommitAndRollback(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanup()

	type countRow struct {
		C int64 `gorm:"column:c"`
	}

	manager := NewManager(gormDB)
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

// TestManager_Execute_NestedCommit 验证嵌套 Execute 复用同一事务，内外层写入随外层一并提交
func TestManager_Execute_NestedCommit(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanup()

	manager := NewManager(gormDB)
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

// TestManager_Execute_NestedRollback 验证嵌套 Execute 不新开事务：外层回滚时内层写入一并回滚
func TestManager_Execute_NestedRollback(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanup()

	manager := NewManager(gormDB)
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

	manager := NewManager(gormDB)

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

// TestManager_ExecuteAutonomous_OuterRollbackKeepsWrite 验证自治事务独立提交：外层回滚不撤销自治写入
func TestManager_ExecuteAutonomous_OuterRollbackKeepsWrite(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanup()

	manager := NewManager(gormDB)
	ctx := context.Background()

	err := manager.Execute(ctx, func(txCtx context.Context) error {
		if err := Exec(txCtx, gormDB).Create(&txTestModel{ID: 41, Name: "outer"}).Error; err != nil {
			return err
		}
		if err := manager.ExecuteAutonomous(txCtx, 0, func(actx context.Context) error {
			return Exec(actx, gormDB).Create(&txTestModel{ID: 42, Name: "autonomous"}).Error
		}); err != nil {
			return err
		}
		return errors.New("外层主动回滚")
	})
	if err == nil {
		t.Fatal("外层返回错误时期望 Execute 返回错误")
	}

	var rows []txTestModel
	if err := gormDB.Where("id IN ?", []int64{41, 42}).Find(&rows).Error; err != nil {
		t.Fatalf("查询结果失败: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != 42 {
		t.Fatalf("期望仅保留自治写入 42，实际 %+v", rows)
	}
}

// TestManager_ExecuteAutonomous_IgnoresCanceledCtx 验证自治事务不随调用方取消：取消后仍提交
func TestManager_ExecuteAutonomous_IgnoresCanceledCtx(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanup()

	manager := NewManager(gormDB)

	err := manager.Execute(context.Background(), func(txCtx context.Context) error {
		canceled, cancel := context.WithCancel(txCtx)
		cancel()
		return manager.ExecuteAutonomous(canceled, 0, func(actx context.Context) error {
			return Exec(actx, gormDB).Create(&txTestModel{ID: 51, Name: "autonomous"}).Error
		})
	})
	if err != nil {
		t.Fatalf("自治事务不应受调用方取消影响: %v", err)
	}

	var count int64
	if err := gormDB.Model(&txTestModel{}).Where("id = ?", 51).Count(&count).Error; err != nil {
		t.Fatalf("查询结果失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("期望自治写入落库，实际 %d 条", count)
	}
}

// TestManager_ExecuteAutonomous_RejectsTransactionRoot 验证用事务对象构造的 Manager 拒绝自治事务
func TestManager_ExecuteAutonomous_RejectsTransactionRoot(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanup()

	manager := NewManager(gormDB)

	err := manager.Execute(context.Background(), func(txCtx context.Context) error {
		gtx, ok := FromContext(txCtx, gormDB)
		if !ok {
			t.Fatal("期望上下文中存在事务")
		}
		return NewManager(gtx).ExecuteAutonomous(txCtx, 0, func(context.Context) error {
			return nil
		})
	})
	if err == nil {
		t.Fatal("用事务对象构造的 Manager 应拒绝自治事务")
	}
}

// TestExec_ScopesTransactionByRoot 验证事务按数据源根句柄隔离：A 库事务不会被 B 库读取
func TestExec_ScopesTransactionByRoot(t *testing.T) {
	gormA, cleanupA := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanupA()
	gormB, cleanupB := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanupB()

	managerA := NewManager(gormA)

	err := managerA.Execute(context.Background(), func(txCtx context.Context) error {
		if _, ok := FromContext(txCtx, gormB); ok {
			t.Fatal("B 库不应读到 A 库的事务")
		}
		if _, ok := FromContext(txCtx, gormA); !ok {
			t.Fatal("A 库应读到自己的事务")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("事务执行失败: %v", err)
	}
}

// TestExec_OutsideTxWritesOnRoot 验证不在事务中时 Exec 直接使用数据源根句柄
func TestExec_OutsideTxWritesOnRoot(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(txTestModel{}))
	defer cleanup()

	if err := Exec(context.Background(), gormDB).Create(&txTestModel{ID: 61, Name: "outside"}).Error; err != nil {
		t.Fatalf("事务外写入失败: %v", err)
	}

	var count int64
	if err := gormDB.Model(&txTestModel{}).Where("id = ?", 61).Count(&count).Error; err != nil {
		t.Fatalf("查询写入结果失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("期望事务外写入 1 条，实际 %d", count)
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
