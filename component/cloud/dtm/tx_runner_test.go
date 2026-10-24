// TxRunner 的单元测试
//
// @author zwei

package dtm

import (
	"context"
	"net/url"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/db/tx"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"

	"gorm.io/gorm"
)

// TestTxRunner_RunUsesBoundRoot 验证无 barrier 时 Run 在绑定数据源上提交事务
func TestTxRunner_RunUsesBoundRoot(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(dtmTestModel{}))
	defer cleanup()

	runner := NewTxRunner(gormDB, enums.MySQL, "")

	err := runner.Run(context.Background(), func(ctx context.Context) error {
		return tx.Exec(ctx, gormDB).Create(&dtmTestModel{ID: 1, Name: "tx-runner"}).Error
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	var count int64
	if err := gormDB.Model(&dtmTestModel{}).Where("id = ?", 1).Count(&count).Error; err != nil {
		t.Fatalf("查询结果失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("期望写入 1 条，实际 %d", count)
	}
}

// TestTxRunner_RunRollbackOnError 验证回调返回错误时事务回滚
func TestTxRunner_RunRollbackOnError(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(dtmTestModel{}))
	defer cleanup()

	runner := NewTxRunner(gormDB, enums.MySQL, "")

	err := runner.Run(context.Background(), func(ctx context.Context) error {
		if err := tx.Exec(ctx, gormDB).Create(&dtmTestModel{ID: 2, Name: "rolled-back"}).Error; err != nil {
			return err
		}
		return context.Canceled
	})
	if err == nil {
		t.Fatal("期望 Run 返回错误")
	}

	var count int64
	if err := gormDB.Model(&dtmTestModel{}).Where("id = ?", 2).Count(&count).Error; err != nil {
		t.Fatalf("查询结果失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("回滚后期望 0 条，实际 %d", count)
	}
}

// TestNewTxRunner_RejectsInvalidBarrierTable 验证非法 Barrier 表名在构造期被拒绝
func TestNewTxRunner_RejectsInvalidBarrierTable(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("非法 barrier_table 期望 panic")
		}
	}()
	NewTxRunner(&gorm.DB{}, enums.MySQL, "sys_barrier; DROP TABLE x")
}

// dtmTestModel TxRunner 单测用 GORM 模型
type dtmTestModel struct {
	ID   int64  `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

// TableName 返回 TxRunner 单测表名
func (dtmTestModel) TableName() string {
	return "dtm_test_model"
}

// TestTxRunner_InTx 验证 InTx 的绑定判定：Barrier 形态不算绑定，其余按 ctx 中的事务判定
func TestTxRunner_InTx(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(dtmTestModel{}))
	defer cleanup()

	runner := NewTxRunner(gormDB, enums.MySQL, "")
	ctx := context.Background()

	if runner.InTx(ctx) {
		t.Fatal("无事务 ctx 不应判定为绑定")
	}

	// Barrier 形态由 Run 自行开启并提交事务，提交点归 Run 所有，不算绑定
	q := url.Values{}
	q.Set("trans_type", "saga")
	q.Set("gid", "test-gid")
	q.Set("branch_id", "test-branch")
	q.Set("op", "action")
	barrierCtx, err := ContextWithBarrier(ctx, q)
	if err != nil {
		t.Fatalf("构造 barrier ctx 失败: %v", err)
	}
	if runner.InTx(barrierCtx) {
		t.Fatal("Barrier 形态不应判定为绑定")
	}

	// 普通形态：ctx 已绑定事务即为 true
	err = runner.Run(ctx, func(txCtx context.Context) error {
		if !runner.InTx(txCtx) {
			t.Error("事务内 ctx 应判定为绑定")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
}
