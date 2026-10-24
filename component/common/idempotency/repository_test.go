// sys_biz 仓储冲突语义与超时清理的集成测试
//
// @author zwei

package idempotency

import (
	"context"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/db/orm"
	"gitee.com/hq_project/yomi-boot/component/common/db/tx"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestRepository_TryInsert 抢占成功与唯一键冲突的判定
func TestRepository_TryInsert(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	repo := NewRepository(env.db)
	ctx := context.Background()

	newRow := func(id int64, bizID, bizSubID string) *SysBiz {
		return &SysBiz{
			BizID: bizID, BizSubID: bizSubID,
			OperationType: "TEST_OP", Status: StatusProcessing,
			BaseModel: orm.BaseModel{ID: id},
		}
	}

	claimed, err := repo.TryInsert(ctx, newRow(1, "b1", "s1"))
	if err != nil || !claimed {
		t.Fatalf("首插应抢占成功, claimed=%v, err=%v", claimed, err)
	}

	claimed, err = repo.TryInsert(ctx, newRow(2, "b1", "s1"))
	if err != nil || claimed {
		t.Fatalf("同键应判定冲突且不报错, claimed=%v, err=%v", claimed, err)
	}

	claimed, err = repo.TryInsert(ctx, newRow(3, "b1", "s2"))
	if err != nil || !claimed {
		t.Fatalf("不同子键应抢占成功, claimed=%v, err=%v", claimed, err)
	}
}

// TestRepository_TryInsert_TxAliveAfterConflict 冲突后事务仍可用——PG 语句级中止修复的核心契约
func TestRepository_TryInsert_TxAliveAfterConflict(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	repo := NewRepository(env.db)
	runner := tx.NewLocalRunner(env.db)

	err := runner.Run(context.Background(), func(txCtx context.Context) error {
		claimed, err := repo.TryInsert(txCtx, &SysBiz{
			BizID: "b2", BizSubID: "s2", OperationType: "TEST_OP", Status: StatusProcessing,
			BaseModel: orm.BaseModel{ID: 10},
		})
		if err != nil || !claimed {
			t.Errorf("事务内首插应抢占成功, claimed=%v, err=%v", claimed, err)
			return nil
		}
		claimed, err = repo.TryInsert(txCtx, &SysBiz{
			BizID: "b2", BizSubID: "s2", OperationType: "TEST_OP", Status: StatusProcessing,
			BaseModel: orm.BaseModel{ID: 11},
		})
		if err != nil || claimed {
			t.Errorf("事务内同键应判定冲突且不报错, claimed=%v, err=%v", claimed, err)
			return nil
		}
		// 冲突后同事务的兜底查询必须可用，否则 PG 下回放会拿到 transaction aborted
		found, err := repo.FindByKey(txCtx, "b2", "s2")
		if err != nil || found == nil || found.ID != 10 {
			t.Errorf("冲突后同事务查询应可用, found=%v, err=%v", found, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("事务执行失败: %v", err)
	}
}

// TestRepository_DeleteStaleProcessing 只清理过期 Processing 记录，limit 生效
func TestRepository_DeleteStaleProcessing(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	repo := NewRepository(env.db)
	ctx := context.Background()

	seed := func(id int64, bizID string) {
		t.Helper()
		claimed, err := repo.TryInsert(ctx, &SysBiz{
			BizID: bizID, BizSubID: "s", OperationType: "TEST_OP", Status: StatusProcessing,
			BaseModel: orm.BaseModel{ID: id},
		})
		if err != nil || !claimed {
			t.Fatalf("造数失败 id=%d, claimed=%v, err=%v", id, claimed, err)
		}
	}
	seed(100, "stale-1")
	seed(101, "stale-2")
	seed(102, "fresh")

	// 回拨过期记录的 update_time（绕过 gorm autoUpdateTime 的自动刷新）
	backdate := func(ids []int64) {
		t.Helper()
		if err := env.db.Exec("UPDATE sys_biz SET update_time = ? WHERE id IN ?",
			time.Now().Add(-2*time.Hour), ids).Error; err != nil {
			t.Fatalf("回拨 update_time 失败: %v", err)
		}
	}
	backdate([]int64{100, 101})

	count, err := repo.DeleteStaleProcessing(ctx, time.Now().Add(-time.Hour), 100)
	if err != nil || count != 2 {
		t.Fatalf("应清理 2 条过期记录, count=%d, err=%v", count, err)
	}
	var remaining int64
	env.db.Model(&SysBiz{}).Count(&remaining)
	if remaining != 1 {
		t.Fatalf("未过期记录应保留, remaining=%d", remaining)
	}

	seed(103, "stale-3")
	seed(104, "stale-4")
	backdate([]int64{103, 104})
	count, err = repo.DeleteStaleProcessing(ctx, time.Now().Add(-time.Hour), 1)
	if err != nil || count != 1 {
		t.Fatalf("limit=1 应只清理 1 条, count=%d, err=%v", count, err)
	}
}

// TestTryInsert_DialectSQLShape 校验两个方言生成的 SQL 都满足「冲突不报错、事务不中止」
func TestTryInsert_DialectSQLShape(t *testing.T) {
	row := &SysBiz{
		BizID: "b", BizSubID: "s", OperationType: "OP", Status: StatusProcessing,
		BaseModel: orm.BaseModel{ID: 1},
	}
	buildSQL := func(db *gorm.DB) string {
		return db.ToSQL(func(conn *gorm.DB) *gorm.DB {
			return conn.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "biz_id"}, {Name: "biz_sub_id"}},
				DoNothing: true,
			}).Create(row)
		})
	}

	// MySQL：翻译为 ON DUPLICATE KEY UPDATE 主键空更新
	mysqlDB, cleanup := dbtest.New(t)
	defer cleanup()
	mysqlSQL := buildSQL(mysqlDB)
	if !strings.Contains(mysqlSQL, "ON DUPLICATE KEY UPDATE") || !strings.Contains(mysqlSQL, "`id`=`id`") {
		t.Fatalf("MySQL 方言 SQL 形态不符: %s", mysqlSQL)
	}

	// PG：翻译为 ON CONFLICT (biz_id, biz_sub_id) DO NOTHING
	pgDB, err := gorm.Open(postgres.Open("host=127.0.0.1 port=1 user=test dbname=test sslmode=disable"),
		&gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("打开 PG 方言失败: %v", err)
	}
	pgSQL := buildSQL(pgDB)
	if !strings.Contains(pgSQL, "ON CONFLICT") || !strings.Contains(pgSQL, "DO NOTHING") || !strings.Contains(pgSQL, `"biz_id"`) {
		t.Fatalf("PG 方言 SQL 形态不符: %s", pgSQL)
	}
}
