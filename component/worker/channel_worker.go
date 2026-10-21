// Package worker 基于 channel 的任务队列工作池实现，支持阻塞/非阻塞提交、超时控制与优雅停机
//
// @author zwei
package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/appctx"
	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/logger"
)

var _ WorkerService = (*channelWorker)(nil)

const channelBackend = "channel"

// channelStatus channel 工作池状态
type channelStatus int

const (
	channelStatusInit    channelStatus = iota // 初始化
	channelStatusRunning                      // 启动
	channelStatusClosed                       // 关闭
)

// channelWorker 基于 channel 的任务队列工作池
type channelWorker struct {
	status    channelStatus
	mu        sync.Mutex
	stopOnce  sync.Once
	stopDone  chan struct{}
	submitWg  sync.WaitGroup
	stopWg    sync.WaitGroup
	cfg       WorkConfig
	parentCtx context.Context
	ctx       context.Context
	cancel    context.CancelFunc
	jobs      chan WorkerHandler
}

// newChannelWorker 创建 channel 工作池
func newChannelWorker(cfg WorkConfig, parentCtx context.Context) (*channelWorker, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	parentCtx = appctx.OrBackground(parentCtx)
	ctx, cancel := context.WithCancel(parentCtx)
	return &channelWorker{
		status:    channelStatusInit,
		stopDone:  make(chan struct{}),
		cfg:       cfg,
		parentCtx: parentCtx,
		ctx:       ctx,
		cancel:    cancel,
		jobs:      make(chan WorkerHandler, cfg.Size),
	}, nil
}

// Start 启动工作池，按 WorkNum 拉起协程消费任务队列
func (w *channelWorker) Start() {
	w.mu.Lock()
	if w.status != channelStatusInit {
		w.mu.Unlock()
		return
	}
	w.status = channelStatusRunning
	w.mu.Unlock()

	for i := 0; i < w.cfg.WorkNum; i++ {
		w.stopWg.Add(1)
		go w.execute(i)
	}

	go w.watchParentCtx()

	logger.InfoWithoutCtx(fmt.Sprintf("worker.%s.start", w.cfg.Name), "backend", channelBackend)
}

// execute 消费任务队列；jobs 关闭后 range 会排空剩余任务，再退出协程
func (w *channelWorker) execute(i int) {
	defer w.stopWg.Done()
	for job := range w.jobs {
		runChannelHandler(w.ctx, w.cfg.Name, i, job)
	}
}

// Submit 提交任务到工作池
func (w *channelWorker) Submit(handler WorkerHandler) error {
	return w.SubmitContext(context.Background(), handler)
}

// SubmitContext 提交任务并在入队阶段响应 ctx 取消与入队超时
func (w *channelWorker) SubmitContext(ctx context.Context, handler WorkerHandler) error {
	if handler == nil {
		return errors.New("worker.handler 不能为空")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	w.mu.Lock()
	if w.status != channelStatusRunning {
		w.mu.Unlock()
		return errors.Newf("worker.%s.not_run", w.cfg.Name)
	}
	w.submitWg.Add(1)
	w.mu.Unlock()
	defer w.submitWg.Done()

	timeout := w.cfg.effectiveSubmitTimeout()
	var timer *time.Timer
	var timerC <-chan time.Time
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		timerC = timer.C
		defer stopChannelTimer(timer)
	}

	select {
	case w.jobs <- handler:
		return nil
	case <-w.ctx.Done():
		return errors.Newf("worker.%s.stopped", w.cfg.Name)
	case <-ctx.Done():
		return ctx.Err()
	case <-timerC:
		return errors.Newf("worker.%s.submit_timeout", w.cfg.Name)
	}
}

// TrySubmit 尝试提交任务，队列满时立即返回错误
func (w *channelWorker) TrySubmit(handler WorkerHandler) error {
	if handler == nil {
		return errors.New("worker.handler 不能为空")
	}

	w.mu.Lock()
	if w.status != channelStatusRunning {
		w.mu.Unlock()
		return errors.Newf("worker.%s.not_run", w.cfg.Name)
	}
	w.submitWg.Add(1)
	w.mu.Unlock()
	defer w.submitWg.Done()

	select {
	case w.jobs <- handler:
		return nil
	case <-w.ctx.Done():
		return errors.Newf("worker.%s.stopped", w.cfg.Name)
	default:
		return errors.Newf("worker.%s.queue_full", w.cfg.Name)
	}
}

// Pending 返回 channel 中待处理任务数
func (w *channelWorker) Pending() int {
	return len(w.jobs)
}

// Close 停止接收新任务并排空已排队任务
func (w *channelWorker) Close() error {
	w.shutdown()
	return nil
}

// Wait 阻塞直至工作池停机完成
func (w *channelWorker) Wait() {
	<-w.stopDone
}

// shutdown 优雅停止工作池
func (w *channelWorker) shutdown() {
	w.stopOnce.Do(func() {
		defer close(w.stopDone)

		w.mu.Lock()
		if w.status == channelStatusClosed {
			w.mu.Unlock()
			return
		}
		w.status = channelStatusClosed
		w.mu.Unlock()

		w.cancel()
		w.submitWg.Wait()
		close(w.jobs)
		w.stopWg.Wait()

		logger.InfoWithoutCtx(fmt.Sprintf("worker.%s.stop", w.cfg.Name), "backend", channelBackend)
	})
}

// watchParentCtx 监听 parentCtx 取消并触发自动停机
func (w *channelWorker) watchParentCtx() {
	<-w.parentCtx.Done()
	w.shutdown()
}

// runChannelHandler 执行任务并统一处理 panic 与 error 日志
func runChannelHandler(ctx context.Context, name string, workerIndex int, handler WorkerHandler) {
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

// stopChannelTimer 停止 timer 并排空 channel，避免泄漏
func stopChannelTimer(timer *time.Timer) {
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
