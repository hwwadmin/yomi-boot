// Package worker 基于 ants 协程池的工作池实现，支持阻塞/非阻塞提交、超时控制与优雅停机
//
// @author zwei
package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/appctx"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"

	"github.com/panjf2000/ants/v2"
)

var _ WorkerService = (*antsWorker)(nil)

const antsBackend = "ants"

// antsStatus ants 工作池状态
type antsStatus int

const (
	antsStatusInit    antsStatus = iota // 初始化
	antsStatusRunning                   // 启动
	antsStatusClosed                    // 关闭
)

// antsWorker 基于 ants goroutine 池的工作池
type antsWorker struct {
	status    antsStatus
	mu        sync.Mutex
	stopOnce  sync.Once
	stopDone  chan struct{}
	submitWg  sync.WaitGroup
	cfg       WorkConfig
	parentCtx context.Context
	ctx       context.Context
	cancel    context.CancelFunc
	pool      *ants.Pool
}

// newAntsWorker 创建 ants 工作池
func newAntsWorker(cfg WorkConfig, parentCtx context.Context) (*antsWorker, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	parentCtx = appctx.OrBackground(parentCtx)

	ctx, cancel := context.WithCancel(parentCtx)
	w := &antsWorker{
		status:    antsStatusInit,
		stopDone:  make(chan struct{}),
		cfg:       cfg,
		parentCtx: parentCtx,
		ctx:       ctx,
		cancel:    cancel,
	}

	pool, err := ants.NewPool(cfg.WorkNum,
		ants.WithMaxBlockingTasks(cfg.Size),
		ants.WithNonblocking(false),
	)
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "创建 ants 工作池失败")
	}
	w.pool = pool

	return w, nil
}

// Start 启动工作池并监听 parentCtx 自动停机
func (w *antsWorker) Start() {
	w.mu.Lock()
	if w.status != antsStatusInit {
		w.mu.Unlock()
		return
	}
	w.status = antsStatusRunning
	w.mu.Unlock()

	go w.watchParentCtx()

	logger.InfoWithoutCtx(fmt.Sprintf("worker.%s.start", w.cfg.Name), "backend", antsBackend)
}

// Submit 提交任务到工作池
func (w *antsWorker) Submit(handler WorkerHandler) error {
	return w.SubmitContext(context.Background(), handler)
}

// SubmitContext 提交任务并在入队阶段响应 ctx 取消与入队超时
func (w *antsWorker) SubmitContext(ctx context.Context, handler WorkerHandler) error {
	return w.submit(ctx, handler, true)
}

// TrySubmit 尝试提交任务，队列满时立即返回错误
func (w *antsWorker) TrySubmit(handler WorkerHandler) error {
	return w.submit(w.ctx, handler, false)
}

// Pending 返回 ants 池中阻塞等待 worker 的 Submit 调用数
func (w *antsWorker) Pending() int {
	if w.pool == nil {
		return 0
	}
	return w.pool.Waiting()
}

// Close 停止接收新任务并等待运行中任务结束
func (w *antsWorker) Close() error {
	w.shutdown()
	return nil
}

// Wait 阻塞直至工作池停机完成
func (w *antsWorker) Wait() {
	<-w.stopDone
}

// submit 提交任务；block 为 false 时 worker 已满则立即返回 queue_full（与 channel TrySubmit 对齐）
func (w *antsWorker) submit(ctx context.Context, handler WorkerHandler, block bool) error {
	if handler == nil {
		return errors.New("worker.handler 不能为空")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	w.mu.Lock()
	if w.status != antsStatusRunning {
		w.mu.Unlock()
		return errors.Newf("worker.%s.not_run", w.cfg.Name)
	}
	w.submitWg.Add(1)
	w.mu.Unlock()
	releaseSubmit := func() { w.submitWg.Done() }

	if !block {
		if w.pool.Running() >= w.pool.Cap() {
			releaseSubmit()
			return errors.Newf("worker.%s.queue_full", w.cfg.Name)
		}
		err := w.pool.Submit(func() {
			runAntsHandler(w.ctx, w.cfg.Name, 0, handler)
		})
		releaseSubmit()
		if errors.Is(err, ants.ErrPoolOverload) {
			return errors.Newf("worker.%s.queue_full", w.cfg.Name)
		}
		return err
	}

	defer releaseSubmit()

	var dropped atomic.Bool
	result := make(chan error, 1)
	go func() {
		result <- w.pool.Submit(func() {
			if dropped.Load() {
				return
			}
			runAntsHandler(w.ctx, w.cfg.Name, 0, handler)
		})
	}()

	timeout := w.cfg.effectiveSubmitTimeout()
	var timerC <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		timerC = timer.C
		defer stopAntsTimer(timer)
	}

	select {
	case err := <-result:
		if errors.Is(err, ants.ErrPoolOverload) {
			return errors.Newf("worker.%s.queue_full", w.cfg.Name)
		}
		return err
	case <-ctx.Done():
		return w.abandonBlockedSubmit(result, &dropped, ctx.Err())
	case <-w.ctx.Done():
		return w.abandonBlockedSubmit(result, &dropped, errors.Newf("worker.%s.stopped", w.cfg.Name))
	case <-timerC:
		return w.abandonBlockedSubmit(result, &dropped, errors.Newf("worker.%s.submit_timeout", w.cfg.Name))
	}
}

// abandonBlockedSubmit 放弃阻塞入队；若入队已完成则返回实际结果，避免误丢弃已成功提交的任务
func (w *antsWorker) abandonBlockedSubmit(result <-chan error, dropped *atomic.Bool, err error) error {
	select {
	case submitErr := <-result:
		if errors.Is(submitErr, ants.ErrPoolOverload) {
			return errors.Newf("worker.%s.queue_full", w.cfg.Name)
		}
		return submitErr
	default:
		dropped.Store(true)
		return err
	}
}

// shutdown 优雅停止工作池
func (w *antsWorker) shutdown() {
	w.stopOnce.Do(func() {
		defer close(w.stopDone)

		w.mu.Lock()
		if w.status == antsStatusClosed {
			w.mu.Unlock()
			return
		}
		w.status = antsStatusClosed
		w.mu.Unlock()

		w.cancel()
		w.submitWg.Wait()
		if w.pool != nil {
			w.pool.Release()
		}

		logger.InfoWithoutCtx(fmt.Sprintf("worker.%s.stop", w.cfg.Name), "backend", antsBackend)
	})
}

// watchParentCtx 监听 parentCtx 取消并触发自动停机
func (w *antsWorker) watchParentCtx() {
	defer func() {
		if r := recover(); r != nil {
			logger.WarnWithoutCtx(fmt.Sprintf("worker.%s.watch_parent_ctx.panic", w.cfg.Name), "panic", r)
		}
	}()
	<-w.parentCtx.Done()
	w.shutdown()
}

// runAntsHandler 执行任务并统一处理 panic 与 error 日志
func runAntsHandler(ctx context.Context, name string, workerIndex int, handler WorkerHandler) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warn(ctx, fmt.Sprintf("worker.%s.%d.job.panic", name, workerIndex), "panic", r)
		}
	}()
	if err := handler(ctx); err != nil {
		logErr := errors.Wrap(err, "worker.%s.%d job error", name, workerIndex)
		logger.Warn(ctx, fmt.Sprintf("worker.%s.%d.error", name, workerIndex), "error", logErr)
	}
}

// stopAntsTimer 停止 timer 并排空 channel，避免泄漏
func stopAntsTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
