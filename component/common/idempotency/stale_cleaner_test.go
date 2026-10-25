// Processing 超时清理器的单元测试：内存 MySQL（权威 DDL）+ 内存 Redis
//
// @author zwei
package idempotency

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// cleanerEnv 清理器测试依赖
type cleanerEnv struct {
	db      *gorm.DB
	repo    *Repository
	factory distlock.DistLockFactory
	cleanup func()
}

// newCleanerEnv 组装内存 MySQL（权威 sys_biz DDL）与内存 Redis 分布式锁工厂
func newCleanerEnv(t *testing.T) *cleanerEnv {
	t.Helper()

	gormDB, cleanupDB := dbtest.New(t, dbtest.WithSQLFiles("../../../sql/mysql/sys_biz.sql"))
	client, cleanupRedis := redistest.NewClient(t)
	factory, err := distlock.NewFactory(client)
	if err != nil {
		cleanupRedis()
		cleanupDB()
		t.Fatalf("构建分布式锁工厂失败: %v", err)
	}
	return &cleanerEnv{
		db:      gormDB,
		repo:    NewRepository(gormDB),
		factory: factory,
		cleanup: func() {
			cleanupRedis()
			cleanupDB()
		},
	}
}

// newDeadDB 构造一个必然连接失败的 gorm 客户端，用于触发数据库错误路径
func newDeadDBForIdempotency(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "root@tcp(127.0.0.1:1)/dead",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("构造失效数据库失败: %v", err)
	}
	return db
}

// mustSeedProcessing 写入一条处理中记录并返回主键
func mustSeedProcessing(t *testing.T, db *gorm.DB, id int64, bizID string, updateTime time.Time) {
	t.Helper()
	row := SysBiz{BizID: bizID, BizSubID: "sub-" + bizID, OperationType: "TEST_OP", Status: StatusProcessing}
	row.ID = id
	row.UpdateTime = updateTime
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("造数失败: %v", err)
	}
}

// TestStaleCleanerDoTask 验证只清理超时的 Processing 记录
func TestStaleCleanerDoTask(t *testing.T) {
	env := newCleanerEnv(t)
	defer env.cleanup()

	old := time.Now().Add(-time.Hour)
	mustSeedProcessing(t, env.db, 1, "stale-1", old)
	mustSeedProcessing(t, env.db, 2, "fresh-1", time.Now())
	// 已完成记录即使超时也不清理
	completed := SysBiz{BizID: "done-1", BizSubID: "sub-done-1", OperationType: "TEST_OP", Status: StatusCompleted}
	completed.ID = 3
	completed.UpdateTime = old
	if err := env.db.Create(&completed).Error; err != nil {
		t.Fatalf("造数失败: %v", err)
	}

	cleaner := NewStaleCleaner(env.repo, StaleCleanerConfig{
		StaleTimeout: 10 * time.Minute,
		BatchSize:    100,
		ServiceName:  "svc",
	})
	if err := cleaner.doTask(context.Background()); err != nil {
		t.Fatalf("清理任务失败: %v", err)
	}

	var count int64
	if err := env.db.Unscoped().Model(&SysBiz{}).Count(&count).Error; err != nil {
		t.Fatalf("统计记录失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("期望仅清理超时 Processing 记录，实际剩余 %d 条", count)
	}
	var remain []SysBiz
	if err := env.db.Unscoped().Order("id ASC").Find(&remain).Error; err != nil {
		t.Fatalf("查询记录失败: %v", err)
	}
	if remain[0].ID != 2 || remain[1].ID != 3 {
		t.Fatalf("应保留未超时与已完成记录: %+v", remain)
	}
}

// TestStaleCleanerDoTaskError 验证数据库故障时错误透传
func TestStaleCleanerDoTaskError(t *testing.T) {
	repo := NewRepository(newDeadDBForIdempotency(t))
	cleaner := NewStaleCleaner(repo, StaleCleanerConfig{StaleTimeout: time.Minute, BatchSize: 100})
	if err := cleaner.doTask(context.Background()); err == nil {
		t.Fatal("数据库不可用时应返回错误")
	}
}

// TestStaleCleanerLifecycle 验证启动、重复启动与关闭
func TestStaleCleanerLifecycle(t *testing.T) {
	env := newCleanerEnv(t)
	defer env.cleanup()

	cleaner := NewStaleCleaner(env.repo, StaleCleanerConfig{
		ScanInterval: 50 * time.Millisecond,
		StaleTimeout: time.Minute,
		BatchSize:    100,
		ServiceName:  "svc",
	})
	if err := cleaner.Start(nil, env.factory); err != nil {
		t.Fatalf("启动清理器失败: %v", err)
	}
	if err := cleaner.Start(nil, env.factory); err != nil {
		t.Fatalf("重复启动应幂等: %v", err)
	}
	// 给轮询 goroutine 一点时间跑一轮空任务
	time.Sleep(150 * time.Millisecond)
	if err := cleaner.Close(); err != nil {
		t.Fatalf("关闭清理器失败: %v", err)
	}
	// 关闭后 cancel 置空，重复关闭直接返回
	if err := cleaner.Close(); err != nil {
		t.Fatalf("重复关闭应幂等: %v", err)
	}
}

// TestStaleCleanerStartNilSafe 验证空清理器与缺仓储时不启动
func TestStaleCleanerStartNilSafe(t *testing.T) {
	env := newCleanerEnv(t)
	defer env.cleanup()

	var nilCleaner *StaleCleaner
	if err := nilCleaner.Start(context.Background(), env.factory); err != nil {
		t.Fatalf("nil 清理器应直接返回 nil: %v", err)
	}
	if err := nilCleaner.Close(); err != nil {
		t.Fatalf("nil 清理器关闭应幂等: %v", err)
	}

	noRepo := NewStaleCleaner(nil, StaleCleanerConfig{})
	if err := noRepo.Start(context.Background(), env.factory); err != nil {
		t.Fatalf("缺仓储应直接返回 nil: %v", err)
	}
}

// TestStaleCleanerStartRejectsNilFactory 验证缺少分布式锁工厂时启动报错
func TestStaleCleanerStartRejectsNilFactory(t *testing.T) {
	env := newCleanerEnv(t)
	defer env.cleanup()

	cleaner := NewStaleCleaner(env.repo, StaleCleanerConfig{ScanInterval: time.Second, ServiceName: "svc"})
	if err := cleaner.Start(context.Background(), nil); err == nil {
		t.Fatal("nil 分布式锁工厂应返回错误")
	}
	if cleaner.started {
		t.Fatal("启动失败不应置位 started")
	}
}

// TestStartStaleCleanerFromConfig 验证一键启动的依赖校验与成功路径
func TestStartStaleCleanerFromConfig(t *testing.T) {
	env := newCleanerEnv(t)
	defer env.cleanup()

	if got := StartStaleCleanerFromConfig(context.Background(), StaleCleanerDeps{}); got != nil {
		t.Fatal("缺仓储应返回 nil")
	}
	if got := StartStaleCleanerFromConfig(context.Background(), StaleCleanerDeps{
		Repo: env.repo,
	}); got != nil {
		t.Fatal("缺分布式锁工厂应返回 nil")
	}

	cleaner := StartStaleCleanerFromConfig(context.Background(), StaleCleanerDeps{
		Repo:            env.repo,
		DistLockFactory: env.factory,
		AppName:         "app",
		Config:          Config{ScanIntervalMinutes: 1, StaleTimeoutMinutes: 10, BatchSize: 50},
	})
	if cleaner == nil {
		t.Fatal("依赖齐全应启动成功")
	}
	if err := cleaner.Close(); err != nil {
		t.Fatalf("关闭清理器失败: %v", err)
	}
}

// TestMustStartStaleCleanerFromConfig 验证 Fail Fast 各分支与成功路径
func TestMustStartStaleCleanerFromConfig(t *testing.T) {
	env := newCleanerEnv(t)
	defer env.cleanup()

	cases := []struct {
		name string
		deps StaleCleanerDeps
	}{
		{name: "缺仓储", deps: StaleCleanerDeps{DistLockFactory: env.factory}},
		{name: "缺分布式锁工厂", deps: StaleCleanerDeps{Repo: env.repo}},
		{name: "配置非法", deps: StaleCleanerDeps{
			Repo:            env.repo,
			DistLockFactory: env.factory,
			Config:          Config{ScanIntervalMinutes: -1},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s 应 panic", tc.name)
				}
			}()
			MustStartStaleCleanerFromConfig(context.Background(), tc.deps)
		})
	}

	t.Run("成功", func(t *testing.T) {
		cleaner := MustStartStaleCleanerFromConfig(context.Background(), StaleCleanerDeps{
			Repo:            env.repo,
			DistLockFactory: env.factory,
			AppName:         "app",
		})
		if cleaner == nil {
			t.Fatal("依赖齐全应启动成功")
		}
		if err := cleaner.Close(); err != nil {
			t.Fatalf("关闭清理器失败: %v", err)
		}
	})
}

// TestRepositoryMarkCompletedNoEffect 验证完成标记只对处理中记录生效
func TestRepositoryMarkCompletedNoEffect(t *testing.T) {
	env := newCleanerEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	mustSeedProcessing(t, env.db, 1, "mark-1", time.Now())
	if err := env.repo.MarkCompleted(ctx, 1, []byte(`"ok"`)); err != nil {
		t.Fatalf("标记完成失败: %v", err)
	}
	row, err := env.repo.FindByKey(ctx, "mark-1", "sub-mark-1")
	if err != nil || row == nil || row.Status != StatusCompleted {
		t.Fatalf("记录应已完成: %+v, %v", row, err)
	}

	// 已完成记录再次标记：状态条件不匹配，更新未生效应报错
	if err := env.repo.MarkCompleted(ctx, 1, []byte(`"again"`)); err == nil {
		t.Fatal("已完成记录再次标记应返回错误")
	}
	// 不存在的 ID 同理
	if err := env.repo.MarkCompleted(ctx, 999, []byte(`"x"`)); err == nil {
		t.Fatal("不存在的记录标记应返回错误")
	}
}

// TestRepositoryDeleteStaleProcessingGuards 验证批量大小校验与无命中短路
func TestRepositoryDeleteStaleProcessingGuards(t *testing.T) {
	env := newCleanerEnv(t)
	defer env.cleanup()
	ctx := context.Background()

	if _, err := env.repo.DeleteStaleProcessing(ctx, time.Now(), 0); err == nil {
		t.Fatal("批量大小为 0 应返回错误")
	}
	count, err := env.repo.DeleteStaleProcessing(ctx, time.Now().Add(-time.Hour), 100)
	if err != nil || count != 0 {
		t.Fatalf("无命中应返回 0: %d, %v", count, err)
	}
}

// TestRepositoryErrorPathsOnDeadDB 验证仓储查询与完成标记的数据库错误透传
func TestRepositoryErrorPathsOnDeadDB(t *testing.T) {
	repo := NewRepository(newDeadDBForIdempotency(t))
	ctx := context.Background()

	if _, err := repo.FindByKey(ctx, "b", "s"); err == nil {
		t.Fatal("数据库不可用时查询应返回错误")
	}
	if err := repo.MarkCompleted(ctx, 1, []byte("x")); err == nil {
		t.Fatal("数据库不可用时标记完成应返回错误")
	}

	guard := &Guard{repo: repo, idGenerator: snowflakeGenForTest()}
	if _, err := findCompletedSnapshot[string](guard, ctx, Key{BizID: "b", BizSubID: "s"}); err == nil {
		t.Fatal("查证路径应透传数据库错误")
	}
}
