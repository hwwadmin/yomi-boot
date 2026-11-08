// 最小空闲连接保活的单元测试
//
// @author zwei

package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/db/dbtest"
)

func TestMaintainInterval(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		idle time.Duration
		want time.Duration
	}{
		{name: "过短夹到5s", idle: 2 * time.Second, want: 5 * time.Second},
		{name: "一半落在区间内", idle: 40 * time.Second, want: 20 * time.Second},
		{name: "过长夹到1m", idle: 10 * time.Minute, want: time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := maintainInterval(tc.idle); got != tc.want {
				t.Fatalf("maintainInterval(%v)=%v, want %v", tc.idle, got, tc.want)
			}
		})
	}
}

func TestIdleKeeperStopIdempotent(t *testing.T) {
	t.Parallel()

	var k *idleKeeper
	k.Stop() // nil 安全

	k = startIdleKeeper(nil, 2, 3, time.Second)
	if k != nil {
		t.Fatal("sqlDB=nil 不应启动 keeper")
	}

	k = startIdleKeeper(nil, 0, 0, time.Second)
	if k != nil {
		t.Fatal("minIdle=0 不应启动 keeper")
	}
}

// TestEnsureMinIdle_TopsUp 验证按 min_idle 补足空闲连接
func TestEnsureMinIdle_TopsUp(t *testing.T) {
	sqlDB, _, cleanup := dbtest.NewSQL(t, dbtest.WithTables(idleTestModel{}))
	defer cleanup()
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(3)

	if err := ensureMinIdle(context.Background(), sqlDB, 2, 3); err != nil {
		t.Fatalf("补连接失败: %v", err)
	}
	if got := sqlDB.Stats().Idle; got != 2 {
		t.Fatalf("期望空闲连接 2，实际 %d", got)
	}
}

// TestEnsureMinIdle_SkipsWhenSaturated 验证池满时不抢连接、不阻塞
func TestEnsureMinIdle_SkipsWhenSaturated(t *testing.T) {
	sqlDB, _, cleanup := dbtest.NewSQL(t, dbtest.WithTables(idleTestModel{}))
	defer cleanup()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("占用连接失败: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := ensureMinIdle(ctx, sqlDB, 1, 1); err != nil {
		t.Fatalf("池满时应跳过而非报错: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 50*time.Millisecond {
		t.Fatalf("池满时应立即返回，实际耗时 %v", elapsed)
	}
}

// TestEnsureMinIdle_NoOpWhenEnough 验证空闲达标后不再新开连接
func TestEnsureMinIdle_NoOpWhenEnough(t *testing.T) {
	sqlDB, _, cleanup := dbtest.NewSQL(t, dbtest.WithTables(idleTestModel{}))
	defer cleanup()
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(3)

	if err := ensureMinIdle(context.Background(), sqlDB, 1, 3); err != nil {
		t.Fatalf("补连接失败: %v", err)
	}
	before := sqlDB.Stats().OpenConnections
	if err := ensureMinIdle(context.Background(), sqlDB, 1, 3); err != nil {
		t.Fatalf("空闲达标时不应报错: %v", err)
	}
	if got := sqlDB.Stats().OpenConnections; got != before {
		t.Fatalf("空闲达标不应新开连接，期望 %d 实际 %d", before, got)
	}
}

// TestEnsureMinIdle_RespectsMaxIdle 验证持有量受 max_idle 约束
func TestEnsureMinIdle_RespectsMaxIdle(t *testing.T) {
	sqlDB, _, cleanup := dbtest.NewSQL(t, dbtest.WithTables(idleTestModel{}))
	defer cleanup()
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(1)

	if err := ensureMinIdle(context.Background(), sqlDB, 3, 1); err != nil {
		t.Fatalf("补连接失败: %v", err)
	}
	if got := sqlDB.Stats().Idle; got != 1 {
		t.Fatalf("空闲连接应受 max_idle 限制为 1，实际 %d", got)
	}
}

// idleTestModel 最小空闲保活单测用 GORM 模型
type idleTestModel struct {
	ID   int64  `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

// TableName 返回最小空闲保活单测表名
func (idleTestModel) TableName() string {
	return "idle_test_model"
}

func TestIdleHelpers_Edges(t *testing.T) {
	if err := warmIdle(context.Background(), nil, 0); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := sql.Open("mysql", "u:p@tcp(127.0.0.1:1)/db?timeout=50ms")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := warmIdle(ctx, sqlDB, 1); err == nil {
		t.Fatal("已取消的上下文预热应失败")
	}
	if err := ensureMinIdle(context.Background(), sqlDB, 0, 1); err != nil {
		t.Fatal(err)
	}

	keeper := startIdleKeeper(sqlDB, 1, 1, 0)
	if keeper == nil || keeper.interval != 30*time.Second {
		t.Fatalf("非正巡检间隔应回退 30s: %+v", keeper)
	}
	keeper.Stop()

	panicKeeper := &idleKeeper{
		db:       nil,
		minIdle:  1,
		maxIdle:  1,
		interval: time.Millisecond,
		done:     make(chan struct{}),
	}
	loopCtx, stopLoop := context.WithCancel(context.Background())
	go panicKeeper.loop(loopCtx)
	time.Sleep(30 * time.Millisecond)
	stopLoop()
	select {
	case <-panicKeeper.done:
	case <-time.After(time.Second):
		t.Fatal("保活协程 panic 后应退出")
	}
}
