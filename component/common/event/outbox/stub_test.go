// Package outbox 的共享测试桩
//
// @author zwei
package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db/dbtest"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
	"gitee.com/hq_project/yomi-boot/component/common/worker"

	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newOutboxTestDB 启动内存 MySQL 并建发件箱相关表，返回 gorm 客户端与两个仓储
func newOutboxTestDB(t *testing.T) (*gorm.DB, *repo.OutboxRepo, *repo.OutboxDlqRepo, func()) {
	t.Helper()
	gormDB, cleanup := dbtest.New(t, dbtest.WithTables(model.SysEventOutbox{}, model.SysEventOutboxDlq{}))
	return gormDB, repo.NewOutboxRepo(gormDB), repo.NewOutboxDlqRepo(gormDB), cleanup
}

// stubPublisher 记录 PublishJSON 调用的发布器桩，err 固定作为发布结果返回
type stubPublisher struct {
	mu     sync.Mutex
	topics []string
	keys   []string
	err    error
}

// Publish 空实现
func (s *stubPublisher) Publish(context.Context, yomikafka.PublishRequest) error { return nil }

// PublishBatch 空实现
func (s *stubPublisher) PublishBatch(context.Context, []yomikafka.PublishRequest) error { return nil }

// PublishJSON 记录调用并返回预设错误
func (s *stubPublisher) PublishJSON(_ context.Context, topic, key string, _ any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.topics = append(s.topics, topic)
	s.keys = append(s.keys, key)
	return s.err
}

// PublishAsync 空实现
func (s *stubPublisher) PublishAsync(context.Context, yomikafka.PublishRequest) error { return nil }

// PublishBatchAsync 空实现
func (s *stubPublisher) PublishBatchAsync(context.Context, []yomikafka.PublishRequest) error {
	return nil
}

// PublishJSONAsync 空实现
func (s *stubPublisher) PublishJSONAsync(context.Context, string, string, any) error { return nil }

// Close 空实现
func (s *stubPublisher) Close() error { return nil }

// calls 返回 PublishJSON 的调用次数
func (s *stubPublisher) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.topics)
}

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

// failExecutor 始终失败的事务执行器桩
type failExecutor struct {
	err error
}

// Run 返回预设错误
func (f failExecutor) Run(context.Context, func(context.Context) error) error { return f.err }

// InTx 恒返回 false
func (failExecutor) InTx(context.Context) bool { return false }

// RunAutonomous 返回预设错误
func (f failExecutor) RunAutonomous(context.Context, time.Duration, func(context.Context) error) error {
	return f.err
}

// syncWorker 同步执行提交任务的工作池桩
type syncWorker struct {
	mu    sync.Mutex
	count int
}

// Start 空实现
func (s *syncWorker) Start() {}

// Submit 同步执行任务
func (s *syncWorker) Submit(h worker.WorkerHandler) error { return s.TrySubmit(h) }

// SubmitContext 同步执行任务
func (s *syncWorker) SubmitContext(_ context.Context, h worker.WorkerHandler) error {
	return s.TrySubmit(h)
}

// TrySubmit 立即同步执行任务
func (s *syncWorker) TrySubmit(h worker.WorkerHandler) error {
	s.mu.Lock()
	s.count++
	s.mu.Unlock()
	return h(context.Background())
}

// Pending 返回待处理任务数（同步执行恒为 0）
func (s *syncWorker) Pending() int { return 0 }

// Close 空实现
func (s *syncWorker) Close() error { return nil }

// Wait 空实现
func (s *syncWorker) Wait() {}

// submitted 返回已执行的任务数
func (s *syncWorker) submitted() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

// gateWorker 前 accept 次提交成功并同步执行，之后返回队列已满错误的工作池桩
type gateWorker struct {
	mu       sync.Mutex
	accept   int
	rejected int
}

// Start 空实现
func (g *gateWorker) Start() {}

// Submit 按剩余配额执行或拒绝
func (g *gateWorker) Submit(h worker.WorkerHandler) error { return g.TrySubmit(h) }

// SubmitContext 按剩余配额执行或拒绝
func (g *gateWorker) SubmitContext(_ context.Context, h worker.WorkerHandler) error {
	return g.TrySubmit(h)
}

// TrySubmit 有配额时同步执行任务，否则返回队列已满错误
func (g *gateWorker) TrySubmit(h worker.WorkerHandler) error {
	g.mu.Lock()
	if g.accept > 0 {
		g.accept--
		g.mu.Unlock()
		return h(context.Background())
	}
	g.rejected++
	g.mu.Unlock()
	return errors.New("worker.event.outbox.processor.queue_full")
}

// Pending 空实现
func (g *gateWorker) Pending() int { return 0 }

// Close 空实现
func (g *gateWorker) Close() error { return nil }

// Wait 空实现
func (g *gateWorker) Wait() {}

// rejectedCount 返回被拒绝的提交次数
func (g *gateWorker) rejectedCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rejected
}

// fullWorker 始终返回队列已满错误的工作池桩
type fullWorker struct {
	pending int
}

// Start 空实现
func (f *fullWorker) Start() {}

// Submit 返回队列已满错误
func (f *fullWorker) Submit(worker.WorkerHandler) error { return f.TrySubmit(nil) }

// SubmitContext 返回队列已满错误
func (f *fullWorker) SubmitContext(context.Context, worker.WorkerHandler) error {
	return f.TrySubmit(nil)
}

// TrySubmit 返回队列已满错误
func (f *fullWorker) TrySubmit(worker.WorkerHandler) error {
	return errors.New("worker.event.outbox.processor.queue_full")
}

// Pending 返回预设待处理任务数
func (f *fullWorker) Pending() int { return f.pending }

// Close 空实现
func (f *fullWorker) Close() error { return nil }

// Wait 空实现
func (f *fullWorker) Wait() {}

// errWorker 始终返回普通错误的工作池桩（不可恢复类错误）
type errWorker struct {
	err error
}

// Start 空实现
func (e *errWorker) Start() {}

// Submit 返回预设错误
func (e *errWorker) Submit(worker.WorkerHandler) error { return e.err }

// SubmitContext 返回预设错误
func (e *errWorker) SubmitContext(context.Context, worker.WorkerHandler) error { return e.err }

// TrySubmit 返回预设错误
func (e *errWorker) TrySubmit(worker.WorkerHandler) error { return e.err }

// Pending 空实现
func (e *errWorker) Pending() int { return 0 }

// Close 空实现
func (e *errWorker) Close() error { return nil }

// Wait 空实现
func (e *errWorker) Wait() {}

// errIDGen 固定返回错误的雪花 ID 生成器桩
type errIDGen struct {
	err error
}

// NextID 返回预设错误
func (g errIDGen) NextID() (int64, error) { return 0, g.err }

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
