// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import (
	"context"
	"sync"

	"gitee.com/hq_project/yomi-boot/boot/errors"
	"gitee.com/hq_project/yomi-boot/boot/logger"
)

// MemoryRecordConsumerConfig 表示 HTTP 访问记录内存消费者参数
type MemoryRecordConsumerConfig struct {
	Workers int // Workers 是消费协程数
}

// StartMemoryRecordConsumer 启动 HTTP 访问记录内存队列消费者
// 业务 handler 返回 Retry 时记 Warn 后丢弃（内存队列无可靠重投）
func StartMemoryRecordConsumer(
	parentCtx context.Context,
	svc *MemoryHttpRecordService,
	cfg MemoryRecordConsumerConfig,
	handler RecordHandler,
) error {
	if svc == nil {
		return errors.New("HTTP 访问记录内存服务未初始化")
	}
	if handler == nil {
		return errors.New("HTTP 访问记录消费处理函数不能为空")
	}
	if cfg.Workers <= 0 {
		return errors.New("HTTP 访问记录内存消费协程数必须大于 0")
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	if !svc.consumerStarted.CompareAndSwap(false, true) {
		return errors.New("HTTP 访问记录内存消费者已启动")
	}

	runCtx, cancel := context.WithCancel(parentCtx)
	svc.consumerMu.Lock()
	svc.consumerCancel = cancel
	svc.consumerMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func(workerIndex int) {
			defer wg.Done()
			runMemoryRecordWorker(runCtx, svc, workerIndex, handler)
		}(i)
	}

	go func() {
		<-runCtx.Done()
		wg.Wait()
	}()

	return nil
}

// runMemoryRecordWorker 从内存队列拉取记录并委托业务 handler
// 关闭时优先退出，不排空队列中的剩余记录
func runMemoryRecordWorker(
	ctx context.Context,
	svc *MemoryHttpRecordService,
	workerIndex int,
	handler RecordHandler,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-svc.ctx.Done():
			return
		default:
		}

		select {
		case <-ctx.Done():
			return
		case <-svc.ctx.Done():
			return
		case record := <-svc.queue:
			if record == nil {
				continue
			}
			invokeMemoryRecordHandler(ctx, workerIndex, record, handler)
		}
	}
}

// invokeMemoryRecordHandler 调用业务 handler 并统一处理 panic / Retry
func invokeMemoryRecordHandler(
	ctx context.Context,
	workerIndex int,
	record *HttpRecord,
	handler RecordHandler,
) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warn(ctx, "HTTP 访问记录内存消费 panic",
				"panic", r,
				"worker", workerIndex,
				"traceId", record.TraceID,
				"path", record.Path,
			)
		}
	}()

	result := handler(ctx, record)
	if result.Status == RecordHandleRetry {
		logger.Warn(ctx, "HTTP 访问记录内存消费返回 Retry，已丢弃",
			"worker", workerIndex,
			"traceId", record.TraceID,
			"path", record.Path,
			"method", record.Method,
		)
	}
}
