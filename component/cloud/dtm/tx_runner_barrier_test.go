// TxRunner Barrier 链路的单元测试：解析 Barrier → Barrier 事务执行 → 幂等/空补偿语义
//
// 覆盖 dtmcli BranchBarrier.Call 的真实行为（insert-ignore 幂等），
// 依赖 dbtest 进程内真 MySQL 与 sys_barrier 表的唯一索引。

package dtm

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/db/tx"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"

	"github.com/dtm-labs/client/dtmcli"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// dtmTestBarrier DTM Barrier 表测试模型，唯一索引对齐 dtm 官方 DDL（gid+branch_id+op+barrier_id）
type dtmTestBarrier struct {
	ID         int64      `gorm:"column:id;primaryKey;autoIncrement"`
	TransType  string     `gorm:"column:trans_type;size:45"`
	Gid        string     `gorm:"column:gid;size:128;uniqueIndex:uniq_barrier"`
	BranchID   string     `gorm:"column:branch_id;size:128;uniqueIndex:uniq_barrier"`
	Op         string     `gorm:"column:op;size:45;uniqueIndex:uniq_barrier"`
	BarrierID  string     `gorm:"column:barrier_id;size:45;uniqueIndex:uniq_barrier"`
	Reason     string     `gorm:"column:reason;size:45"`
	CreateTime *time.Time `gorm:"column:create_time"`
	UpdateTime *time.Time `gorm:"column:update_time"`
}

// TableName 返回 Barrier 表测试表名（须与 dtmcli.SetBarrierTableName 的默认值一致）
func (dtmTestBarrier) TableName() string {
	return "sys_barrier"
}

// barrierQuery 构造 DTM 分支回调携带的 Barrier 查询参数
func barrierQuery(gid, branchID, op string) url.Values {
	q := url.Values{}
	q.Set("trans_type", "saga")
	q.Set("gid", gid)
	q.Set("branch_id", branchID)
	q.Set("op", op)
	return q
}

// mustBarrierCtx 构造已附加 Barrier 元数据的上下文
func mustBarrierCtx(t *testing.T, gid, branchID, op string) context.Context {
	t.Helper()
	ctx, err := ContextWithBarrier(context.Background(), barrierQuery(gid, branchID, op))
	if err != nil {
		t.Fatalf("构造 barrier ctx 失败: %v", err)
	}
	return ctx
}

// newBarrierTestRunner 启动内存 MySQL 并创建绑定 sys_barrier 表的 TxRunner
func newBarrierTestRunner(t *testing.T) (*testing.T, *TxRunner, *gorm.DB, func()) {
	t.Helper()
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(dtmTestModel{}, dtmTestBarrier{}))
	runner := NewTxRunner(gormDB, enums.MySQL, "")
	return t, runner, gormDB, cleanup
}

// countBarrierRows 统计指定 gid 的 Barrier 记录数
func countBarrierRows(t *testing.T, gormDB *gorm.DB, gid string) int64 {
	t.Helper()
	var count int64
	if err := gormDB.Model(&dtmTestBarrier{}).Where("gid = ?", gid).Count(&count).Error; err != nil {
		t.Fatalf("查询 Barrier 记录失败: %v", err)
	}
	return count
}

// getBarrierRow 读取指定 gid+op 的 Barrier 记录，不存在时报错
func getBarrierRow(t *testing.T, gormDB *gorm.DB, gid, op string) dtmTestBarrier {
	t.Helper()
	var row dtmTestBarrier
	if err := gormDB.Where("gid = ? AND op = ?", gid, op).First(&row).Error; err != nil {
		t.Fatalf("期望存在 gid=%s op=%s 的 Barrier 记录: %v", gid, op, err)
	}
	return row
}

// TestTxRunner_RunBarrierActionExecutesOnce 验证首次 action 分支在 Barrier 事务中执行并提交
func TestTxRunner_RunBarrierActionExecutesOnce(t *testing.T) {
	_, runner, gormDB, cleanup := newBarrierTestRunner(t)
	defer cleanup()

	ctx := mustBarrierCtx(t, "g-action", "b1", "action")
	calls := 0
	err := runner.Run(ctx, func(txCtx context.Context) error {
		calls++
		bb, ok := BarrierFromContext(txCtx)
		if !ok || bb.Gid != "g-action" {
			t.Errorf("回调 ctx 应携带 Barrier 元数据，ok=%v", ok)
		}
		return tx.Exec(txCtx, gormDB).Create(&dtmTestModel{ID: 10, Name: "barrier-action"}).Error
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if calls != 1 {
		t.Fatalf("回调应执行 1 次，实际 %d", calls)
	}

	var business int64
	if err := gormDB.Model(&dtmTestModel{}).Where("id = ?", 10).Count(&business).Error; err != nil || business != 1 {
		t.Fatalf("业务数据应随 Barrier 事务提交，count=%d err=%v", business, err)
	}
	row := getBarrierRow(t, gormDB, "g-action", "action")
	if row.BarrierID != "01" || row.Reason != "action" || row.BranchID != "b1" {
		t.Fatalf("Barrier 记录不符合预期: %+v", row)
	}
}

// TestTxRunner_RunBarrierSkipsRepeatedRequest 验证重复的分支请求被 Barrier 幂等跳过
func TestTxRunner_RunBarrierSkipsRepeatedRequest(t *testing.T) {
	_, runner, gormDB, cleanup := newBarrierTestRunner(t)
	defer cleanup()

	first := mustBarrierCtx(t, "g-dup", "b1", "action")
	if err := runner.Run(first, func(txCtx context.Context) error {
		return tx.Exec(txCtx, gormDB).Create(&dtmTestModel{ID: 11, Name: "dup-first"}).Error
	}); err != nil {
		t.Fatalf("首次 Run 失败: %v", err)
	}

	// 模拟 DTM 重试再次回调：同样的 gid/branch/op，但重新解析出新 BranchBarrier
	second := mustBarrierCtx(t, "g-dup", "b1", "action")
	calls := 0
	if err := runner.Run(second, func(txCtx context.Context) error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("重复 Run 不应报错: %v", err)
	}
	if calls != 0 {
		t.Fatalf("重复请求应被 Barrier 跳过，实际执行 %d 次", calls)
	}

	var business int64
	if err := gormDB.Model(&dtmTestModel{}).Where("name = ?", "dup-first").Count(&business).Error; err != nil || business != 1 {
		t.Fatalf("业务数据不应被重复写入，count=%d err=%v", business, err)
	}
}

// TestTxRunner_RunBarrierNullCompensateSkips 验证未执行过 action 的空补偿被跳过
func TestTxRunner_RunBarrierNullCompensateSkips(t *testing.T) {
	_, runner, gormDB, cleanup := newBarrierTestRunner(t)
	defer cleanup()

	ctx := mustBarrierCtx(t, "g-null", "b1", "compensate")
	calls := 0
	if err := runner.Run(ctx, func(txCtx context.Context) error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("空补偿不应报错: %v", err)
	}
	if calls != 0 {
		t.Fatalf("空补偿应被跳过，实际执行 %d 次", calls)
	}
	// 空补偿写入 origin(action)+current(compensate) 两条防悬挂记录
	if got := countBarrierRows(t, gormDB, "g-null"); got != 2 {
		t.Fatalf("空补偿应留下 origin+current 两条 Barrier 记录，实际 %d", got)
	}
}

// TestTxRunner_RunBarrierCompensateAfterActionExecutes 验证 action 已执行后的补偿正常执行
func TestTxRunner_RunBarrierCompensateAfterActionExecutes(t *testing.T) {
	_, runner, gormDB, cleanup := newBarrierTestRunner(t)
	defer cleanup()

	if err := runner.Run(mustBarrierCtx(t, "g-comp", "b1", "action"), func(txCtx context.Context) error {
		return tx.Exec(txCtx, gormDB).Create(&dtmTestModel{ID: 12, Name: "compensated"}).Error
	}); err != nil {
		t.Fatalf("action Run 失败: %v", err)
	}

	calls := 0
	if err := runner.Run(mustBarrierCtx(t, "g-comp", "b1", "compensate"), func(txCtx context.Context) error {
		calls++
		return tx.Exec(txCtx, gormDB).Create(&dtmTestModel{ID: 13, Name: "compensated-back"}).Error
	}); err != nil {
		t.Fatalf("compensate Run 失败: %v", err)
	}
	if calls != 1 {
		t.Fatalf("action 之后的补偿应执行，实际 %d 次", calls)
	}

	var business int64
	if err := gormDB.Model(&dtmTestModel{}).Where("id = ?", 13).Count(&business).Error; err != nil || business != 1 {
		t.Fatalf("补偿写入应提交，count=%d err=%v", business, err)
	}
	getBarrierRow(t, gormDB, "g-comp", "compensate")
}

// TestTxRunner_RunBarrierRollbackOnError 验证 Barrier 事务回调出错时整体回滚且保留错误身份
func TestTxRunner_RunBarrierRollbackOnError(t *testing.T) {
	_, runner, gormDB, cleanup := newBarrierTestRunner(t)
	defer cleanup()

	sentinel := errors.NewNoStack("barrier 业务失败")
	ctx := mustBarrierCtx(t, "g-rollback", "b1", "action")
	err := runner.Run(ctx, func(txCtx context.Context) error {
		if err := tx.Exec(txCtx, gormDB).Create(&dtmTestModel{ID: 14, Name: "rolled-back"}).Error; err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("应保留回调错误身份，实际: %v", err)
	}

	var business int64
	if err := gormDB.Model(&dtmTestModel{}).Where("id = ?", 14).Count(&business).Error; err != nil || business != 0 {
		t.Fatalf("回调出错后业务数据应回滚，count=%d err=%v", business, err)
	}
	if got := countBarrierRows(t, gormDB, "g-rollback"); got != 0 {
		t.Fatalf("回调出错后 Barrier 记录应回滚，实际 %d", got)
	}
}

// TestContextWithBarrierInvalid 验证 Barrier 元数据不完整时报错且上下文保持原样
func TestContextWithBarrierInvalid(t *testing.T) {
	base := context.Background()
	cases := []struct {
		name  string
		query url.Values
	}{
		{name: "全空", query: url.Values{}},
		{name: "缺 gid", query: func() url.Values { q := barrierQuery("", "b1", "action"); q.Del("gid"); return q }()},
		{name: "缺 op", query: func() url.Values { q := barrierQuery("g-1", "b1", ""); q.Del("op"); return q }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ContextWithBarrier(base, tc.query)
			if err == nil {
				t.Fatal("元数据不完整应返回错误")
			}
			if got != base {
				t.Fatal("解析失败时应返回原上下文")
			}
			if _, ok := BarrierFromContext(got); ok {
				t.Fatal("解析失败后不应存在 Barrier")
			}
		})
	}
}

// TestContextWithRequestNil 验证 nil 请求原样返回上下文
func TestContextWithRequestNil(t *testing.T) {
	base := context.Background()
	got, err := ContextWithRequest(base, nil)
	if err != nil {
		t.Fatalf("nil 请求不应报错: %v", err)
	}
	if got != base {
		t.Fatal("nil 请求应返回原上下文")
	}
}

// TestContextWithRequestAttachesBarrier 验证从 HTTP 请求查询串解析 Barrier
func TestContextWithRequestAttachesBarrier(t *testing.T) {
	target := "/dtm/action?" + barrierQuery("g-req", "b1", "action").Encode()
	req := httptest.NewRequest(http.MethodPost, target, nil)

	got, err := ContextWithRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("解析 Barrier 失败: %v", err)
	}
	bb, ok := BarrierFromContext(got)
	if !ok || bb.Gid != "g-req" || bb.BranchID != "b1" || bb.Op != "action" {
		t.Fatalf("ctx 应携带完整 Barrier 元数据，ok=%v bb=%v", ok, bb)
	}
}

// TestGinContextWithBarrierNilCases 验证 Gin 上下文缺失时的防御
func TestGinContextWithBarrierNilCases(t *testing.T) {
	gin.SetMode(gin.TestMode)

	got, err := GinContextWithBarrier(nil)
	if err != nil {
		t.Fatalf("nil Gin 上下文不应报错: %v", err)
	}
	if got == nil {
		t.Fatal("nil Gin 上下文应返回可用 ctx")
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if c.Request != nil {
		t.Fatal("前置条件失效：CreateTestContext 的 Request 应为 nil")
	}
	got, err = GinContextWithBarrier(c)
	if err != nil {
		t.Fatalf("Request 为 nil 不应报错: %v", err)
	}
	if got == nil {
		t.Fatal("Request 为 nil 应返回可用 ctx")
	}
	if _, ok := BarrierFromContext(got); ok {
		t.Fatal("无 Barrier 查询参数不应产生 Barrier")
	}
}

// TestGinContextWithBarrierAttaches 验证从 Gin 请求解析 Barrier
func TestGinContextWithBarrierAttaches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/dtm/action?"+barrierQuery("g-gin", "b1", "action").Encode(), nil)

	got, err := GinContextWithBarrier(c)
	if err != nil {
		t.Fatalf("解析 Barrier 失败: %v", err)
	}
	bb, ok := BarrierFromContext(got)
	if !ok || bb.Gid != "g-gin" {
		t.Fatalf("ctx 应携带 Barrier 元数据，ok=%v bb=%v", ok, bb)
	}
}

// TestBarrierFromContextNilValue 验证上下文中存入 nil Barrier 时不误判为存在
func TestBarrierFromContextNilValue(t *testing.T) {
	ctx := context.WithValue(context.Background(), barrierKey{}, (*dtmcli.BranchBarrier)(nil))
	if _, ok := BarrierFromContext(ctx); ok {
		t.Fatal("nil Barrier 不应判定为存在")
	}
}

// TestTxRunner_RunBarrierBeginFails 验证数据源不可用时开启 Barrier 事务失败
func TestTxRunner_RunBarrierBeginFails(t *testing.T) {
	_, runner, gormDB, cleanup := newBarrierTestRunner(t)
	defer cleanup()

	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("关闭数据源失败: %v", err)
	}

	err = runner.Run(mustBarrierCtx(t, "g-broken", "b1", "action"), func(txCtx context.Context) error {
		t.Error("开启事务失败不应执行回调")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "开启 DTM Barrier 事务失败") {
		t.Fatalf("数据源关闭后期望报开启事务失败，实际: %v", err)
	}
}

// barrierFakeConnPool 实现 gorm.ConnPoolBeginner，BeginTx 返回非 *sql.Tx 的连接以触发 runBarrier 防御分支
type barrierFakeConnPool struct{ gorm.ConnPool }

func (p barrierFakeConnPool) BeginTx(context.Context, *sql.TxOptions) (gorm.ConnPool, error) {
	return barrierFakeConn{}, nil
}

type barrierFakeConn struct{ gorm.ConnPool }

// barrierFakeDialector 包装 mysql Dialector，把 ConnPool 替换为非 *sql.Tx 形态
type barrierFakeDialector struct{ gorm.Dialector }

func (d barrierFakeDialector) Initialize(db *gorm.DB) error {
	if err := d.Dialector.Initialize(db); err != nil {
		return err
	}
	db.ConnPool = barrierFakeConnPool{db.ConnPool}
	return nil
}

// TestTxRunner_RunBarrierNonSqlTxConnPool 验证 ConnPool 非 *sql.Tx 时的防御
func TestTxRunner_RunBarrierNonSqlTxConnPool(t *testing.T) {
	_, _, gormDB, cleanup := newBarrierTestRunner(t)
	defer cleanup()

	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	fakeDB, err := gorm.Open(barrierFakeDialector{
		mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}),
	}, &gorm.Config{})
	if err != nil {
		t.Fatalf("构造假 ConnPool 的 gorm 失败: %v", err)
	}

	runner := NewTxRunner(fakeDB, enums.MySQL, "")
	err = runner.Run(mustBarrierCtx(t, "g-fake", "b1", "action"), func(txCtx context.Context) error {
		t.Error("ConnPool 非 *sql.Tx 时不应执行回调")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "无法获取 *sql.Tx") {
		t.Fatalf("ConnPool 非 *sql.Tx 应报错，实际: %v", err)
	}
}

// TestNewTxRunnerUnsupportedDBType 验证未知数据库类型在构造期 panic
func TestNewTxRunnerUnsupportedDBType(t *testing.T) {
	_, _, gormDB, cleanup := newBarrierTestRunner(t)
	defer cleanup()

	// Postgres 类型合法，构造仅做类型字符串转换
	if NewTxRunner(gormDB, enums.Postgres, "") == nil {
		t.Fatal("Postgres 构造不应返回 nil")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("未知 DB 类型期望 panic")
		}
	}()
	NewTxRunner(gormDB, enums.DbType("oracle"), "")
}
