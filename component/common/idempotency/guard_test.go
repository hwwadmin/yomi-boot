// 业务幂等守卫 Execute / ExecuteInTx 编排的集成测试：内存 MySQL 灌入权威 sys_biz DDL + 内存 Redis
//
// @author zwei

package idempotency

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/db/dbtest"
	"github.com/hwwadmin/yomi-boot/component/common/db/tx"
	"github.com/hwwadmin/yomi-boot/component/common/distlock"
	yomiredis "github.com/hwwadmin/yomi-boot/component/common/redis"
	"github.com/hwwadmin/yomi-boot/component/common/redis/redistest"
	"github.com/hwwadmin/yomi-boot/component/common/snowflake"
	"github.com/hwwadmin/yomi-boot/component/core/errors"

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
		ID:    900,
		BizID: "biz-k6", BizSubID: "sub-k6", OperationType: "TEST_OP", Status: StatusProcessing,
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

// TestExecuteInTx_CommitsWithOuterTxAndNoCache 嵌套形态跟随外层事务提交：落库完成但缓存不写，
// 后续最外层请求缓存 miss 走 DB 查证回放并自愈缓存，业务不重复执行
func TestExecuteInTx_CommitsWithOuterTxAndNoCache(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "nested-result", nil
	}

	var got string
	var execErr error
	if err := env.runner.Run(ctx, func(txCtx context.Context) error {
		got, execErr = ExecuteInTx(env.guard, env.runner, txCtx, testKey("k8"), fn)
		return execErr
	}); err != nil {
		t.Fatalf("外层事务失败: %v", err)
	}
	if got != "nested-result" || calls != 1 {
		t.Fatalf("嵌套执行结果不符: got=%q, calls=%d", got, calls)
	}

	var row SysBiz
	if err := env.db.Where("biz_id = ? AND biz_sub_id = ?", "biz-k8", "sub-k8").First(&row).Error; err != nil {
		t.Fatalf("查询幂等记录失败: %v", err)
	}
	if row.Status != StatusCompleted {
		t.Fatalf("幂等记录应随外层事务提交且已完成, status=%d", row.Status)
	}
	if _, err := env.client.Get(ctx, resultRedisKey(testKey("k8"))).Result(); !errors.Is(err, redis.Nil) {
		t.Fatalf("嵌套形态不应写结果缓存, err=%v", err)
	}

	// 后续最外层请求：缓存 miss → 锁内 DB 查证回放历史，业务不重复执行，并自愈缓存
	got, err := Execute(env.guard, env.runner, ctx, testKey("k8"), fn)
	if err != nil || got != "nested-result" || calls != 1 {
		t.Fatalf("后续回放结果不符: got=%q, calls=%d, err=%v", got, calls, err)
	}
	if _, err := env.client.Get(ctx, resultRedisKey(testKey("k8"))).Result(); err != nil {
		t.Fatalf("回放后应自愈缓存: %v", err)
	}
}

// TestExecuteInTx_OuterRollbackDropsRecord 嵌套形态下外层回滚：幂等记录随之消失、无缓存残留，
// 重试重新执行业务（与最外层形态失败重试语义一致）
func TestExecuteInTx_OuterRollbackDropsRecord(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "rolled-back-result", nil
	}

	// 外层事务在幂等执行成功后主动回滚
	if err := env.runner.Run(ctx, func(txCtx context.Context) error {
		if _, err := ExecuteInTx(env.guard, env.runner, txCtx, testKey("k9"), fn); err != nil {
			return err
		}
		return errors.New("主动回滚")
	}); err == nil {
		t.Fatal("外层事务应回滚")
	}
	if calls != 1 {
		t.Fatalf("业务应执行一次, calls=%d", calls)
	}
	var count int64
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 0 {
		t.Fatalf("幂等记录应随外层回滚消失, count=%d", count)
	}
	if _, err := env.client.Get(ctx, resultRedisKey(testKey("k9"))).Result(); !errors.Is(err, redis.Nil) {
		t.Fatalf("嵌套形态不应写结果缓存, err=%v", err)
	}

	// 重试：记录已消失，重新执行业务并随新外层事务提交
	var got string
	if err := env.runner.Run(ctx, func(txCtx context.Context) error {
		var err error
		got, err = ExecuteInTx(env.guard, env.runner, txCtx, testKey("k9"), fn)
		return err
	}); err != nil {
		t.Fatalf("重试失败: %v", err)
	}
	if got != "rolled-back-result" || calls != 2 {
		t.Fatalf("重试应重新执行业务: got=%q, calls=%d", got, calls)
	}
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 1 {
		t.Fatalf("重试后应落一条已完成记录, count=%d", count)
	}
}

// TestExecuteInTx_SameTxSameKeyReplays 同一外层事务内同键二次调用：经 Step 4 读到本事务内
// 未提交的完成记录直接回放，业务不重复执行
func TestExecuteInTx_SameTxSameKeyReplays(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "in-tx-result", nil
	}

	var second string
	var secondErr error
	if err := env.runner.Run(context.Background(), func(txCtx context.Context) error {
		if _, err := ExecuteInTx(env.guard, env.runner, txCtx, testKey("k10"), fn); err != nil {
			return err
		}
		second, secondErr = ExecuteInTx(env.guard, env.runner, txCtx, testKey("k10"), fn)
		return secondErr
	}); err != nil {
		t.Fatalf("同事务二次调用失败: %v", err)
	}
	if calls != 1 {
		t.Fatalf("同事务同键应回放而非重复执行, calls=%d", calls)
	}
	if second != "in-tx-result" {
		t.Fatalf("应回放第一次结果, got=%q", second)
	}
}

// TestExecuteInTx_RequiresTxCtx 裸 ctx 调用 ExecuteInTx 应 fail-fast，不产生任何副作用
func TestExecuteInTx_RequiresTxCtx(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}
	if _, err := ExecuteInTx(env.guard, env.runner, ctx, testKey("k11"), fn); !errors.Is(err, ErrNotInTx) {
		t.Fatalf("无事务应返回 ErrNotInTx, err=%v", err)
	}
	if calls != 0 {
		t.Fatalf("业务不应执行, calls=%d", calls)
	}
	var count int64
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 0 {
		t.Fatalf("不应产生幂等记录, count=%d", count)
	}
}

// TestRunBusinessOnce_ConflictReplaysCompleted 抢占冲突且反查到提交态完成记录时回放历史快照，
// 业务不执行
func TestRunBusinessOnce_ConflictReplaysCompleted(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	// 预置一条提交态已完成记录，模拟并发方已提交完成
	repo := NewRepository(env.db)
	if claimed, err := repo.TryInsert(ctx, &SysBiz{
		ID:    910,
		BizID: "biz-k12", BizSubID: "sub-k12", OperationType: "TEST_OP", Status: StatusProcessing,
	}); err != nil || !claimed {
		t.Fatalf("造数失败, claimed=%v, err=%v", claimed, err)
	}
	if err := repo.MarkCompleted(ctx, 910, []byte(`"committed-result"`)); err != nil {
		t.Fatalf("标记完成失败: %v", err)
	}

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}
	snap, err := runBusinessOnce[string](env.guard, ctx, testKey("k12"), fn)
	if err != nil {
		t.Fatalf("冲突回退失败: %v", err)
	}
	if snap == nil || snap.result != "committed-result" || calls != 0 {
		t.Fatalf("应回放历史快照且业务不执行: snap=%v, calls=%d", snap, calls)
	}
}

// lockLostOnRecheck 在 RunWithLock 于回调返回后再次读取持锁上下文时取消锁。
// 第一次 HeldContext 留给 BindHeldContext，业务执行期间锁仍有效。
type lockLostOnRecheck struct {
	held   context.Context
	cancel context.CancelFunc
	reads  int
}

func newLockLostOnRecheck() *lockLostOnRecheck {
	held, cancel := context.WithCancel(context.Background())
	return &lockLostOnRecheck{held: held, cancel: cancel}
}

func (l *lockLostOnRecheck) Lock(context.Context) error { return nil }
func (l *lockLostOnRecheck) TryLock(context.Context) (bool, error) {
	return true, nil
}
func (l *lockLostOnRecheck) TryLockWithTimeout(context.Context, time.Duration) (bool, error) {
	return true, nil
}
func (l *lockLostOnRecheck) Unlock(context.Context) error { return nil }
func (l *lockLostOnRecheck) Key() string                  { return "test-lock-lost-on-recheck" }
func (l *lockLostOnRecheck) HeldContext() context.Context {
	l.reads++
	if l.reads >= 2 {
		l.cancel()
	}
	return l.held
}

// fixedLockFactory 每次都返回同一把锁，用来把锁丢失时机固定在测试里
type fixedLockFactory struct {
	lock distlock.Lock
}

func (f fixedLockFactory) GetLock(string, ...distlock.LockOption) distlock.Lock { return f.lock }

func newGuardWithLock(env *testEnv, lock distlock.Lock) *Guard {
	return NewGuard(env.guard.repo, env.guard.idGenerator, env.guard.redisService, fixedLockFactory{lock: lock})
}

// TestExecute_LockLostAfterCommitReturnsResult 验证最外层事务提交后锁丢失仍返回业务结果，
// 缓存清掉后的回放同样返回历史结果，业务不重复执行
func TestExecute_LockLostAfterCommitReturnsResult(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "committed-under-lost-lock", nil
	}
	key := testKey("lost-commit")

	got, err := Execute(newGuardWithLock(env, newLockLostOnRecheck()), env.runner, ctx, key, fn)
	if err != nil || got != "committed-under-lost-lock" || calls != 1 {
		t.Fatalf("提交后锁丢失应返回业务结果: got=%q, calls=%d, err=%v", got, calls, err)
	}
	var row SysBiz
	if err = env.db.Where("biz_id = ? AND biz_sub_id = ?", key.BizID, key.BizSubID).First(&row).Error; err != nil {
		t.Fatalf("查询幂等记录失败: %v", err)
	}
	if row.Status != StatusCompleted {
		t.Fatalf("幂等记录应已完成, status=%d", row.Status)
	}

	if err = env.client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("清空结果缓存失败: %v", err)
	}
	got, err = Execute(newGuardWithLock(env, newLockLostOnRecheck()), env.runner, ctx, key, fn)
	if err != nil || got != "committed-under-lost-lock" || calls != 1 {
		t.Fatalf("回放时锁丢失应返回历史结果: got=%q, calls=%d, err=%v", got, calls, err)
	}
}

// TestExecute_BusinessErrorWithLockLostRollsBack 验证业务失败与锁丢失同时出现时事务回滚，不返回成功
func TestExecute_BusinessErrorWithLockLostRollsBack(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	bizErr := errors.New("业务失败")
	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "", bizErr
	}
	_, err := Execute(newGuardWithLock(env, newLockLostOnRecheck()), env.runner, context.Background(), testKey("lost-rollback"), fn)
	if !errors.Is(err, bizErr) || !errors.Is(err, distlock.ErrLockLost) {
		t.Fatalf("应同时返回业务错误和锁丢失, err=%v", err)
	}
	if calls != 1 {
		t.Fatalf("业务应执行一次, calls=%d", calls)
	}
	var count int64
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 0 {
		t.Fatalf("失败后幂等记录应回滚, count=%d", count)
	}
}

// TestExecuteInTx_LockLostBeforeOuterCommitReturnsError 验证嵌套形态尚未提交时锁丢失仍返回错误，外层回滚
func TestExecuteInTx_LockLostBeforeOuterCommitReturnsError(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "nested", nil
	}
	err := env.runner.Run(context.Background(), func(txCtx context.Context) error {
		_, err := ExecuteInTx(newGuardWithLock(env, newLockLostOnRecheck()), env.runner, txCtx, testKey("lost-nested"), fn)
		return err
	})
	if !errors.Is(err, distlock.ErrLockLost) {
		t.Fatalf("嵌套形态锁丢失应返回 ErrLockLost, err=%v", err)
	}
	if calls != 1 {
		t.Fatalf("业务应已在外层事务内执行, calls=%d", calls)
	}
	var count int64
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 0 {
		t.Fatalf("外层应回滚幂等记录, count=%d", count)
	}
}

// TestIsOnlyLockLost 验证只有锁丢失的错误链才被识别，混有业务错误时必须原样返回
func TestIsOnlyLockLost(t *testing.T) {
	bizErr := errors.New("业务失败")
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "空错误", err: nil, want: false},
		{name: "哨兵", err: errors.Sentinel(distlock.ErrLockLost), want: true},
		{name: "合并两次锁丢失", err: errors.Join(distlock.ErrLockLost, errors.Sentinel(distlock.ErrLockLost)), want: true},
		{name: "合并业务错误", err: errors.Join(distlock.ErrLockLost, bizErr), want: false},
		{name: "其他错误", err: errors.New("释放分布式锁失败"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOnlyLockLost(tc.err); got != tc.want {
				t.Fatalf("isOnlyLockLost()=%v, want %v, err=%v", got, tc.want, tc.err)
			}
		})
	}
}
