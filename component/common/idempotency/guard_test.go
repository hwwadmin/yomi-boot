// 业务幂等守卫 Execute 编排的集成测试：内存 MySQL 灌入权威 sys_biz DDL + 内存 Redis
//
// @author zwei

package idempotency

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/db/orm"
	"gitee.com/hq_project/yomi-boot/component/common/db/tx"
	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	yomiredis "gitee.com/hq_project/yomi-boot/component/common/redis"
	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake"
	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// testEnv 是守卫测试的完整依赖：内存 MySQL（权威 DDL）+ 内存 Redis
type testEnv struct {
	db      *gorm.DB
	guard   *Guard
	runner  tx.Runner
	client  *redis.Client
	cleanup func()
}

// newTestEnv 组装真实 MySQL（灌入权威 sys_biz DDL）与内存 Redis 支撑的完整守卫
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	gormDB, cleanupDB := dbtest.New(t, dbtest.WithSQLFiles("../../../sql/mysql/sys_biz.sql"))
	client, cleanupRedis := redistest.NewClient(t)
	factory, err := distlock.NewFactory(client)
	if err != nil {
		cleanupRedis()
		cleanupDB()
		t.Fatalf("构建分布式锁工厂失败: %v", err)
	}
	return &testEnv{
		db:     gormDB,
		guard:  NewGuard(NewRepository(gormDB), snowflake.MustNewGenerator(1), yomiredis.NewRedisHelper(client), factory),
		runner: tx.NewLocalRunner(gormDB),
		client: client,
		cleanup: func() {
			cleanupRedis()
			cleanupDB()
		},
	}
}

// testKey 构造测试幂等键
func testKey(id string) Key {
	return Key{BizID: "biz-" + id, BizSubID: "sub-" + id, OperationType: "TEST_OP"}
}

// TestExecute_FirstCallRunsBusinessAndReplayHitsCache 首次执行业务并落库，重放命中缓存不再执行业务
func TestExecute_FirstCallRunsBusinessAndReplayHitsCache(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "result-1", nil
	}

	got, err := Execute(env.guard, env.runner, ctx, testKey("k1"), fn)
	if err != nil || got != "result-1" || calls != 1 {
		t.Fatalf("首次执行结果不符: got=%q, calls=%d, err=%v", got, calls, err)
	}

	var row SysBiz
	if err = env.db.Where("biz_id = ? AND biz_sub_id = ?", "biz-k1", "sub-k1").First(&row).Error; err != nil {
		t.Fatalf("查询幂等记录失败: %v", err)
	}
	if row.Status != StatusCompleted {
		t.Fatalf("幂等记录应已完成, status=%d", row.Status)
	}
	// 回写缓存必须与 DB 快照字节一致
	payload, err := env.client.Get(ctx, resultRedisKey(testKey("k1"))).Result()
	if err != nil {
		t.Fatalf("结果缓存未回写: %v", err)
	}
	if payload != string(row.ResponseData) {
		t.Fatalf("缓存快照应与 DB 一致, cache=%q, db=%q", payload, string(row.ResponseData))
	}

	// 同键重放：命中 Redis 结果缓存，业务不重复执行
	got, err = Execute(env.guard, env.runner, ctx, testKey("k1"), fn)
	if err != nil || got != "result-1" || calls != 1 {
		t.Fatalf("缓存重放结果不符: got=%q, calls=%d, err=%v", got, calls, err)
	}
}

// TestExecute_ReplayReturnsHistoryFromDBAfterCacheLost 缓存丢失后锁内 DB 查证回放历史快照并自愈缓存，
// 业务不重复执行
func TestExecute_ReplayReturnsHistoryFromDBAfterCacheLost(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "history-result", nil
	}

	if _, err := Execute(env.guard, env.runner, ctx, testKey("k2"), fn); err != nil {
		t.Fatalf("首次执行失败: %v", err)
	}
	if err := env.client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("清空结果缓存失败: %v", err)
	}

	got, err := Execute(env.guard, env.runner, ctx, testKey("k2"), fn)
	if err != nil {
		t.Fatalf("缓存丢失后回放失败: %v", err)
	}
	if got != "history-result" {
		t.Fatalf("应返回历史结果, got=%q", got)
	}
	if calls != 1 {
		t.Fatalf("业务不应重复执行, calls=%d", calls)
	}
	var count int64
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 1 {
		t.Fatalf("不应产生重复幂等记录, count=%d", count)
	}
	// 自愈断言：回放后缓存已重建
	if _, err := env.client.Get(ctx, resultRedisKey(testKey("k2"))).Result(); err != nil {
		t.Fatalf("回放后应回写缓存自愈: %v", err)
	}
}

// TestExecute_ProcessingRowFailsFast 防御场景：已提交的处理中记录快速失败，业务不执行
func TestExecute_ProcessingRowFailsFast(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	// 直接种一条提交态的处理中记录（正常编排不可见，属防御场景）
	repo := NewRepository(env.db)
	claimed, err := repo.TryInsert(ctx, &SysBiz{
		BizID: "biz-k6", BizSubID: "sub-k6", OperationType: "TEST_OP", Status: StatusProcessing,
		BaseModel: orm.BaseModel{ID: 900},
	})
	if err != nil || !claimed {
		t.Fatalf("造数失败, claimed=%v, err=%v", claimed, err)
	}

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}
	if _, err := Execute(env.guard, env.runner, ctx, testKey("k6"), fn); !errors.Is(err, ErrProcessing) {
		t.Fatalf("处理中记录应返回 ErrProcessing, err=%v", err)
	}
	if calls != 0 {
		t.Fatalf("业务不应执行, calls=%d", calls)
	}
}

// TestExecute_FnErrorRollsBackAndRetryReexecutes 业务失败整体回滚，重试可重新执行
func TestExecute_FnErrorRollsBackAndRetryReexecutes(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("业务失败")
		}
		return "ok", nil
	}

	if _, err := Execute(env.guard, env.runner, ctx, testKey("k3"), fn); err == nil {
		t.Fatal("首次业务失败应返回错误")
	}
	var count int64
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 0 {
		t.Fatalf("失败后幂等记录应随事务回滚, count=%d", count)
	}

	got, err := Execute(env.guard, env.runner, ctx, testKey("k3"), fn)
	if err != nil || got != "ok" || calls != 2 {
		t.Fatalf("重试应重新执行业务: got=%q, calls=%d, err=%v", got, calls, err)
	}
}

// TestExecute_ConcurrentSameKeyRunsOnce 同键并发请求经分布式锁串行后只执行一次业务
func TestExecute_ConcurrentSameKeyRunsOnce(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	var calls atomic.Int64
	fn := func(context.Context) (string, error) {
		calls.Add(1)
		time.Sleep(100 * time.Millisecond) // 放大并发窗口
		return "shared", nil
	}

	const n = 2
	results := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = Execute(env.guard, env.runner, context.Background(), testKey("k4"), fn)
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("并发请求 %d 不应失败: %v", i, errs[i])
		}
		if results[i] != "shared" {
			t.Fatalf("并发请求 %d 应拿到一致结果, got=%q", i, results[i])
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("业务应只执行一次, calls=%d", calls.Load())
	}
}

// TestExecute_InvalidInput 参数不合法时启动期哨兵错误兜底
func TestExecute_InvalidInput(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()
	fn := func(context.Context) (string, error) { return "", nil }

	if _, err := Execute[string](nil, nil, ctx, testKey("k5"), fn); !errors.Is(err, ErrGuardNotReady) {
		t.Fatalf("事务运行器缺失应返回 ErrGuardNotReady, err=%v", err)
	}
	if _, err := Execute(env.guard, env.runner, ctx, Key{BizID: "only-biz"}, fn); !errors.Is(err, ErrKeyInvalid) {
		t.Fatalf("幂等键不完整应返回 ErrKeyInvalid, err=%v", err)
	}
}

// TestExecute_RejectsNestedTx 调用方 ctx 已绑定同源事务时 fail-fast，不产生任何副作用
func TestExecute_RejectsNestedTx(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}

	err := env.runner.Run(ctx, func(txCtx context.Context) error {
		_, err := Execute(env.guard, env.runner, txCtx, testKey("k7"), fn)
		return err
	})
	if !errors.Is(err, ErrNestedTx) {
		t.Fatalf("嵌套事务应返回 ErrNestedTx, err=%v", err)
	}
	if calls != 0 {
		t.Fatalf("业务不应执行, calls=%d", calls)
	}
	var count int64
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 0 {
		t.Fatalf("不应产生幂等记录, count=%d", count)
	}
	if _, err := env.client.Get(ctx, resultRedisKey(testKey("k7"))).Result(); !errors.Is(err, redis.Nil) {
		t.Fatalf("不应写结果缓存, err=%v", err)
	}
}
