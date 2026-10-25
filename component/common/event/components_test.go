// Package event 的组件装配测试
//
// @author zwei
package event

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
	"gitee.com/hq_project/yomi-boot/component/common/snowflake/snowflaketest"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// stubDistLockFactory 永不实际取锁的分布式锁工厂桩（仅用于启动前置校验路径）
type stubDistLockFactory struct{}

// GetLock 返回 nil 锁
func (stubDistLockFactory) GetLock(string, ...distlock.LockOption) distlock.Lock { return nil }

// passthroughExecutor 直接执行事务函数的执行器桩
type passthroughExecutor struct{}

// Run 直接执行事务函数
func (passthroughExecutor) Run(_ context.Context, fn func(ctx context.Context) error) error {
	return fn(context.Background())
}

// InTx 恒返回 false
func (passthroughExecutor) InTx(context.Context) bool { return false }

// RunAutonomous 直接执行事务函数
func (passthroughExecutor) RunAutonomous(_ context.Context, _ time.Duration, fn func(ctx context.Context) error) error {
	return fn(context.Background())
}

// newDeadDB 构造一个必然连接失败的 gorm 客户端，用于触发数据库错误路径
func newDeadDB(t *testing.T) *gorm.DB {
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

// newTestLockFactory 启动内存 Redis 并构建分布式锁工厂
func newTestLockFactory(t *testing.T) distlock.DistLockFactory {
	t.Helper()
	client, cleanup := redistest.NewClient(t)
	t.Cleanup(cleanup)
	factory, err := distlock.NewFactory(client)
	if err != nil {
		t.Fatalf("创建分布式锁工厂失败: %v", err)
	}
	return factory
}

// TestNewComponentsValidation 覆盖装配阶段的依赖校验分支
func TestNewComponentsValidation(t *testing.T) {
	gormDB, cleanup := dbtest.New(t,
		dbtest.WithTables(modelSysEventTables()...),
	)
	defer cleanup()

	pub := &stubPublisher{}
	executor := passthroughExecutor{}
	idGen := snowflaketest.NewTestGenerator()
	factory := newTestLockFactory(t)

	buildDeps := func(mutate func(*Deps)) Deps {
		deps := Deps{
			DB:              gormDB,
			TxExecutor:      executor,
			KafkaPublisher:  pub,
			DistLockFactory: factory,
			IDGenerator:     idGen,
			DbType:          enums.MySQL,
			AppName:         "app",
		}
		mutate(&deps)
		return deps
	}

	if _, err := NewComponents(buildDeps(func(d *Deps) {}), Config{}); err == nil {
		t.Fatal("发布侧与消费侧均未启用应返回错误")
	}
	if _, err := NewComponents(buildDeps(func(d *Deps) { d.DB = nil }), Config{OutboxEnabled: true}); err == nil {
		t.Fatal("数据库缺失应返回错误")
	}
	if _, err := NewComponents(buildDeps(func(d *Deps) { d.TxExecutor = nil }), Config{OutboxEnabled: true}); err == nil {
		t.Fatal("事务执行器缺失应返回错误")
	}
	if _, err := NewComponents(buildDeps(func(d *Deps) { d.IDGenerator = nil }), Config{OutboxEnabled: true}); err == nil {
		t.Fatal("ID 生成器缺失应返回错误")
	}
	if _, err := NewComponents(buildDeps(func(d *Deps) { d.KafkaPublisher = nil }), Config{OutboxEnabled: true}); err == nil {
		t.Fatal("发布侧启用但 Kafka 发布者缺失应返回错误")
	}
	if _, err := NewComponents(buildDeps(func(d *Deps) { d.DistLockFactory = nil }), Config{InboxEnabled: true}); err == nil {
		t.Fatal("消费侧启用但分布式锁工厂缺失应返回错误")
	}
}

// TestNewComponentsAndClose 验证按开关装配出全部子组件并可安全关闭
func TestNewComponentsAndClose(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(modelSysEventTables()...))
	defer cleanup()

	c, err := NewComponents(
		Deps{
			DB:              gormDB,
			TxExecutor:      passthroughExecutor{},
			KafkaPublisher:  &stubPublisher{},
			DistLockFactory: newTestLockFactory(t),
			IDGenerator:     snowflaketest.NewTestGenerator(),
			DbType:          enums.MySQL,
			AppName:         "app",
		},
		Config{
			OutboxEnabled:            true,
			InboxEnabled:             true,
			OutboxCleanupEnabled:     true,
			InboxCleanupEnabled:      true,
			FetchTimeoutMinutes:      5,
			OutboxDlqFrequencyMS:     1000,
			InboxDlqFrequencyMS:      1000,
			OutboxCleanupFrequencyMS: 1000,
			InboxCleanupFrequencyMS:  1000,
		},
	)
	if err != nil {
		t.Fatalf("装配事件组件失败: %v", err)
	}
	if c.Publisher == nil || c.Receiver == nil || c.NormalReceiver == nil {
		t.Fatal("发布侧与消费侧组件均应装配完成")
	}
	if c.retentionCleaner == nil {
		t.Fatal("启用清理后应装配过期数据清理器")
	}

	// 发布侧已启用时 DLQ 重置委托生效（from>to 触发仓储校验错误）
	if _, err := c.ResetDlqRetryCountInTimeRange(context.Background(), time.Now(), time.Now().Add(-time.Hour)); err == nil {
		t.Fatal("时间范围非法应返回错误")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("关闭事件组件失败: %v", err)
	}
	// 重复关闭应保持幂等
	if err := c.Close(); err != nil {
		t.Fatalf("重复关闭事件组件失败: %v", err)
	}
}

// TestComponentsResetDlqRequiresOutbox 验证发布侧未启用时无 DLQ 能力
func TestComponentsResetDlqRequiresOutbox(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(modelSysEventTables()...))
	defer cleanup()

	c, err := NewComponents(
		Deps{
			DB:              gormDB,
			TxExecutor:      passthroughExecutor{},
			KafkaPublisher:  &stubPublisher{},
			DistLockFactory: newTestLockFactory(t),
			IDGenerator:     snowflaketest.NewTestGenerator(),
			DbType:          enums.MySQL,
			AppName:         "app",
		},
		Config{InboxEnabled: true},
	)
	if err != nil {
		t.Fatalf("装配事件组件失败: %v", err)
	}
	if _, err := c.ResetDlqRetryCountInTimeRange(context.Background(), time.Now(), time.Now()); err == nil {
		t.Fatal("发布侧未启用时应返回错误")
	}
}

// TestComponentsStartRequiresFactory 验证后台任务启动前校验分布式锁工厂
func TestComponentsStartRequiresFactory(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(modelSysEventTables()...))
	defer cleanup()

	c, err := NewComponents(
		Deps{
			DB:             gormDB,
			TxExecutor:     passthroughExecutor{},
			KafkaPublisher: &stubPublisher{},
			IDGenerator:    snowflaketest.NewTestGenerator(),
			DbType:         enums.MySQL,
			AppName:        "app",
		},
		Config{OutboxEnabled: true},
	)
	if err != nil {
		t.Fatalf("装配事件组件失败: %v", err)
	}
	if err := c.Start(context.Background()); err == nil {
		t.Fatal("缺少分布式锁工厂时启动应返回错误")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("关闭事件组件失败: %v", err)
	}
}

// TestNewAndStartFailureClosesComponents 验证启动失败时释放已创建资源
func TestNewAndStartFailureClosesComponents(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(modelSysEventTables()...))
	defer cleanup()

	c, err := NewAndStart(
		Deps{
			DB:             gormDB,
			TxExecutor:     passthroughExecutor{},
			KafkaPublisher: &stubPublisher{},
			IDGenerator:    snowflaketest.NewTestGenerator(),
			DbType:         enums.MySQL,
			AppName:        "app",
		},
		Config{OutboxEnabled: true},
	)
	if err == nil {
		_ = c.Close()
		t.Fatal("缺少分布式锁工厂时启动应失败")
	}
	if c != nil {
		t.Fatal("启动失败应返回 nil 组件")
	}
}

// TestComponentsStartAndClose 验证后台任务可正常启动并关闭
func TestComponentsStartAndClose(t *testing.T) {
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(modelSysEventTables()...))
	defer cleanup()

	c, err := NewComponents(
		Deps{
			DB:              gormDB,
			TxExecutor:      passthroughExecutor{},
			KafkaPublisher:  &stubPublisher{},
			DistLockFactory: newTestLockFactory(t),
			IDGenerator:     snowflaketest.NewTestGenerator(),
			DbType:          enums.MySQL,
			AppName:         "app",
		},
		Config{OutboxEnabled: true},
	)
	if err != nil {
		t.Fatalf("装配事件组件失败: %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("启动后台任务失败: %v", err)
	}
	// 重复启动应幂等返回
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("重复启动应返回 nil，实际 %v", err)
	}
	// 给轮询 goroutine 一点时间跑一轮空任务
	time.Sleep(200 * time.Millisecond)
	if err := c.Close(); err != nil {
		t.Fatalf("关闭事件组件失败: %v", err)
	}
}

// TestMustNewAndStartPanicsOnFailure 验证初始化失败时 Fail Fast
func TestMustNewAndStartPanicsOnFailure(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("初始化失败应 panic")
		}
	}()
	// 发布侧与消费侧均未启用，NewComponents 失败 → panic
	MustNewAndStart(Deps{}, Config{})
}

// modelSysEventTables 返回事件组件所需的全部数据表模型
func modelSysEventTables() []schema.Tabler {
	return []schema.Tabler{
		model.SysEventOutbox{}, model.SysEventOutboxDlq{},
		model.SysEventInbox{}, model.SysEventInboxDlq{},
	}
}
