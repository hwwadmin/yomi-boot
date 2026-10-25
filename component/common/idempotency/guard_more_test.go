// 业务幂等守卫的依赖校验、故障注入与冲突回退边界测试
//
// @author zwei
package idempotency

import (
	"context"
	"testing"
	"time"

	yomiredis "gitee.com/hq_project/yomi-boot/component/common/redis"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// stubRedis 可配置读写失败的 Redis 服务桩
type stubRedis struct {
	getErr error
	setErr error
}

// OpsStr4Set 按预设返回写入错误
func (s *stubRedis) OpsStr4Set(context.Context, string, string, time.Duration) error {
	return s.setErr
}

// OpsStr4SetNoTTL 按预设返回写入错误
func (s *stubRedis) OpsStr4SetNoTTL(context.Context, string, string) error { return s.setErr }

// OpsStr4SetJSON 空实现
func (s *stubRedis) OpsStr4SetJSON(context.Context, string, any, time.Duration) error { return nil }

// OpsStr4SetJSONNoTTL 空实现
func (s *stubRedis) OpsStr4SetJSONNoTTL(context.Context, string, any) error { return nil }

// OpsStr4Get 按预设返回读取错误，否则报告键不存在
func (s *stubRedis) OpsStr4Get(context.Context, string) (string, bool, error) {
	return "", false, s.getErr
}

// OpsStr4GetJSON 空实现
func (s *stubRedis) OpsStr4GetJSON(context.Context, string, any) (bool, error) { return false, nil }

// OpsHash4Set 空实现
func (s *stubRedis) OpsHash4Set(context.Context, string, map[string]any, time.Duration) error {
	return nil
}

// OpsHash4Get 空实现
func (s *stubRedis) OpsHash4Get(context.Context, string, string) (string, bool, error) {
	return "", false, nil
}

// OpsHash4GetAll 空实现
func (s *stubRedis) OpsHash4GetAll(context.Context, string) (map[string]string, bool, error) {
	return nil, false, nil
}

// OpsKey4Delete 空实现
func (s *stubRedis) OpsKey4Delete(context.Context, ...string) error { return nil }

// OpsKey4Exists 空实现
func (s *stubRedis) OpsKey4Exists(context.Context, string) (bool, error) { return false, nil }

// OpsKey4Expire 空实现
func (s *stubRedis) OpsKey4Expire(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}

// OpsSet4Add 空实现
func (s *stubRedis) OpsSet4Add(context.Context, string, ...string) error { return nil }

// OpsSet4Remove 空实现
func (s *stubRedis) OpsSet4Remove(context.Context, string, ...string) error { return nil }

// OpsScript4Eval 空实现
func (s *stubRedis) OpsScript4Eval(context.Context, string, []string, ...any) (any, error) {
	return nil, nil
}

// OpsRateLimit4Allow 空实现
func (s *stubRedis) OpsRateLimit4Allow(context.Context, string, int64, int64, time.Duration) (bool, error) {
	return false, nil
}

// OpsSet4Members 空实现
func (s *stubRedis) OpsSet4Members(context.Context, string) ([]string, error) { return nil, nil }

var _ yomiredis.RedisService = (*stubRedis)(nil)

// errGen 固定返回错误的雪花 ID 生成器桩
type errGen struct{}

// NextID 返回预设错误
func (errGen) NextID() (int64, error) { return 0, errors.New("no id") }

// TestNewGuardPanics 验证必填依赖缺失时逐项 Fail Fast
func TestNewGuardPanics(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	cases := []struct {
		name  string
		build func() *Guard
	}{
		{name: "repo 为空", build: func() *Guard {
			return NewGuard(nil, snowflake.MustNewGenerator(1), env.guard.redisService, env.guard.distLockFactory)
		}},
		{name: "idGenerator 为空", build: func() *Guard {
			return NewGuard(env.guard.repo, nil, env.guard.redisService, env.guard.distLockFactory)
		}},
		{name: "redisService 为空", build: func() *Guard {
			return NewGuard(env.guard.repo, snowflake.MustNewGenerator(1), nil, env.guard.distLockFactory)
		}},
		{name: "distLockFactory 为空", build: func() *Guard {
			return NewGuard(env.guard.repo, snowflake.MustNewGenerator(1), env.guard.redisService, nil)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s 应 panic", tc.name)
				}
			}()
			tc.build()
		})
	}
}

// TestExecute_ToleratesCacheWriteFailure 验证缓存回写失败只告警不影响业务结果
func TestExecute_ToleratesCacheWriteFailure(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	guard := NewGuard(env.guard.repo, snowflake.MustNewGenerator(1), &stubRedis{setErr: errors.New("redis down")}, env.guard.distLockFactory)
	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "result-ok", nil
	}

	got, err := Execute(guard, env.runner, context.Background(), testKey("cache-fail"), fn)
	if err != nil || got != "result-ok" || calls != 1 {
		t.Fatalf("缓存回写失败不应影响业务: got=%q, calls=%d, err=%v", got, calls, err)
	}
}

// TestExecute_PropagatesRedisReadError 验证缓存读取失败时错误透传，业务不执行
func TestExecute_PropagatesRedisReadError(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	guard := NewGuard(env.guard.repo, snowflake.MustNewGenerator(1), &stubRedis{getErr: errors.New("redis read failed")}, env.guard.distLockFactory)
	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}

	if _, err := Execute(guard, env.runner, context.Background(), testKey("redis-err"), fn); err == nil {
		t.Fatal("缓存读取失败应返回错误")
	}
	if calls != 0 {
		t.Fatalf("业务不应执行, calls=%d", calls)
	}
}

// TestExecute_DuplicateWithoutRecord 验证唯一键被软删记录占用时的 fail-closed 边界：
// 抢占冲突且反查无可见记录，有限重试后返回 ErrDuplicateWithoutRecord，业务绝不执行
func TestExecute_DuplicateWithoutRecord(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	// 预置记录后软删除：唯一键仍被占用，但普通查询不可见
	row := SysBiz{BizID: "biz-dup", BizSubID: "sub-dup", OperationType: "TEST_OP", Status: StatusCompleted}
	row.ID = 920
	if err := env.db.Create(&row).Error; err != nil {
		t.Fatalf("造数失败: %v", err)
	}
	if err := env.db.Delete(&SysBiz{}, row.ID).Error; err != nil {
		t.Fatalf("软删除造数失败: %v", err)
	}

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}
	if _, err := Execute(env.guard, env.runner, ctx, Key{BizID: "biz-dup", BizSubID: "sub-dup", OperationType: "TEST_OP"}, fn); !errors.Is(err, ErrDuplicateWithoutRecord) {
		t.Fatalf("软删占用唯一键应返回 ErrDuplicateWithoutRecord, err=%v", err)
	}
	if calls != 0 {
		t.Fatalf("业务不应执行, calls=%d", calls)
	}
}

// TestRunBusinessOnce_GeneratorFailure 验证 ID 生成失败时错误透传
func TestRunBusinessOnce_GeneratorFailure(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	guard := NewGuard(env.guard.repo, errGen{}, env.guard.redisService, env.guard.distLockFactory)
	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}
	if _, err := runBusinessOnce[string](guard, context.Background(), testKey("gen-fail"), fn); err == nil {
		t.Fatal("ID 生成失败应返回错误")
	}
	if calls != 0 {
		t.Fatalf("业务不应执行, calls=%d", calls)
	}
}

// TestRunBusinessOnce_RepoError 验证数据库不可用时抢占失败透传
func TestRunBusinessOnce_RepoError(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	guard := NewGuard(NewRepository(newDeadDBForIdempotency(t)), snowflake.MustNewGenerator(1), env.guard.redisService, env.guard.distLockFactory)
	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}
	if _, err := runBusinessOnce[string](guard, context.Background(), testKey("repo-fail"), fn); err == nil {
		t.Fatal("数据库不可用应返回错误")
	}
	if calls != 0 {
		t.Fatalf("业务不应执行, calls=%d", calls)
	}
}

// TestExecute_MarshalFailureRollsBack 验证结果不可序列化时业务回滚、记录不落库
func TestExecute_MarshalFailureRollsBack(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	fn := func(context.Context) (map[string]any, error) {
		return map[string]any{"ch": make(chan int)}, nil
	}
	if _, err := Execute[map[string]any](env.guard, env.runner, ctx, testKey("marshal-fail"), fn); err == nil {
		t.Fatal("结果不可序列化应返回错误")
	}
	var count int64
	env.db.Model(&SysBiz{}).Count(&count)
	if count != 0 {
		t.Fatalf("序列化失败应随事务回滚, count=%d", count)
	}
}

// TestExecuteInTx_ReplaySkipsCacheWrite 验证嵌套形态 DB 查证回放不回写缓存
func TestExecuteInTx_ReplaySkipsCacheWrite(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	// 预置一条已完成记录（无缓存）
	row := SysBiz{BizID: "biz-replay-nocache", BizSubID: "sub-replay-nocache", OperationType: "TEST_OP", Status: StatusCompleted}
	row.ID = 930
	row.ResponseData = []byte(`"history"`)
	if err := env.db.Create(&row).Error; err != nil {
		t.Fatalf("造数失败: %v", err)
	}

	calls := 0
	fn := func(context.Context) (string, error) {
		calls++
		return "x", nil
	}
	var got string
	if err := env.runner.Run(ctx, func(txCtx context.Context) error {
		var err error
		got, err = ExecuteInTx(env.guard, env.runner, txCtx, Key{BizID: "biz-replay-nocache", BizSubID: "sub-replay-nocache", OperationType: "TEST_OP"}, fn)
		return err
	}); err != nil {
		t.Fatalf("嵌套回放失败: %v", err)
	}
	if got != "history" || calls != 0 {
		t.Fatalf("应回放历史快照且业务不执行: got=%q, calls=%d", got, calls)
	}
	// 嵌套形态回放不应写缓存
	if _, err := env.client.Get(ctx, "sysbiz:result:biz-replay-nocache:sub-replay-nocache").Result(); err == nil {
		t.Fatal("嵌套形态回放不应写缓存")
	}
}

// snowflakeGenForTest 返回真实雪花生成器，供错误路径测试使用
func snowflakeGenForTest() snowflake.Generator { return snowflake.MustNewGenerator(7) }
