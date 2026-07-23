// Package outbox 提供发件箱后台抓取与发送能力
//
// @author zwei
package outbox

import (
	"context"
	"strings"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/db"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/event/cfg"
	"gitee.com/hq_project/yomi-boot/event/model"
	"gitee.com/hq_project/yomi-boot/event/repo"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/worker"

	yomikafka "gitee.com/hq_project/yomi-boot/kafka"
)

const (
	sentSuccessPersistRetryTimes    = 3
	sentSuccessPersistRetryInterval = 200 * time.Millisecond
)

// Processor 发件箱发送处理器
type Processor struct {
	cfg            cfg.Runtime
	outboxRepo     *repo.OutboxRepo
	dlq            *DlqHandler
	kafkaPublisher yomikafka.PublisherService
	txExecutor     db.TxExecutor
	worker         worker.WorkerService
	closeMu        sync.Mutex // closeMu 保护 closed 并发调用
	closed         bool       // closed 标记是否已关闭
}

// NewProcessor 创建发件箱发送处理器
// outboxRepo、dlq、kafkaPublisher、txExecutor 为必填依赖，缺失时返回错误
func NewProcessor(
	runtime cfg.Runtime,
	outboxRepo *repo.OutboxRepo,
	dlq *DlqHandler,
	kafkaPublisher yomikafka.PublisherService,
	txExecutor db.TxExecutor,
	parentCtx context.Context,
) (*Processor, error) {
	if outboxRepo == nil {
		return nil, errors.New("event: outboxRepo 不能为 nil")
	}
	if dlq == nil {
		return nil, errors.New("event: dlq 不能为 nil")
	}
	if kafkaPublisher == nil {
		return nil, errors.New("event: kafkaPublisher 不能为 nil")
	}
	if txExecutor == nil {
		return nil, errors.New("event: txExecutor 不能为 nil")
	}
	workCfg := worker.WorkConfig{
		Name:    "event.outbox.processor",
		WorkNum: runtime.SentThreadCount,
		Size:    runtime.SentThreadCount * 128,
	}
	if workCfg.WorkNum <= 0 {
		workCfg.WorkNum = 2
	}
	if workCfg.Size <= 0 {
		workCfg.Size = 256
	}
	w, err := worker.NewWorker(workCfg, parentCtx)
	if err != nil {
		return nil, err
	}
	w.Start()

	return &Processor{
		cfg:            runtime,
		outboxRepo:     outboxRepo,
		dlq:            dlq,
		kafkaPublisher: kafkaPublisher,
		txExecutor:     txExecutor,
		worker:         w,
	}, nil
}

// Offer 提交发件箱事件到发送线程池
func (p *Processor) Offer(ctx context.Context, row model.SysEventOutbox) {
	cloned := row
	err := p.worker.TrySubmit(func(taskCtx context.Context) error {
		return p.push(taskCtx, cloned)
	})
	if err == nil {
		return
	}
	if shouldRestorePending(err) {
		p.restoreFetchedEvent(ctx, cloned, err)
		return
	}
	logger.Error(ctx, "消息队列事件提交发送线程池失败",
		"eventId", row.EventID, "topic", row.Topic, "error", err)
}

// Close 关闭发送线程池，幂等安全
func (p *Processor) Close() {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	_ = p.worker.Close()
	p.worker.Wait()
}

// push 将发件箱事件推送到 Kafka，成功则标记 SentSuccess，失败则写入 DLQ
func (p *Processor) push(ctx context.Context, row model.SysEventOutbox) error {
	evt := model.WireEvent{
		EventID: row.EventID,
		Topic:   row.Topic,
		Payload: row.Payload,
	}
	sentTime := time.Now()
	if err := p.kafkaPublisher.PublishJSON(ctx, row.Topic, row.EventID, evt); err != nil {
		logger.Error(ctx, "消息队列事件推送失败",
			"eventId", row.EventID, "topic", row.Topic, "error", err)
		if txErr := p.txExecutor.Execute(ctx, func(txCtx context.Context) error {
			row.Status = model.OutboxStatusSentFail
			if updateErr := p.outboxRepo.Update(txCtx, &row); updateErr != nil {
				return updateErr
			}
			return p.dlq.Enqueue(txCtx, row, err)
		}); txErr != nil {
			logger.Error(ctx, "消息队列事件写入 DLQ 失败",
				"eventId", row.EventID, "topic", row.Topic, "error", txErr)
			return txErr
		}
		return err
	}

	logger.Info(ctx, "消息队列事件推送成功", "eventId", row.EventID, "topic", row.Topic)
	row.Status = model.OutboxStatusSentSuccess
	row.SentTime = &sentTime
	persistCtx := context.WithoutCancel(ctx)
	if err := p.outboxRepo.Update(persistCtx, &row); err != nil {
		logger.Error(ctx, "消息队列事件发送成功但状态回写失败",
			"eventId", row.EventID, "topic", row.Topic, "error", err)
		if !p.markSentSuccessWithRetry(persistCtx, &row, sentTime) {
			logger.Error(ctx, "消息队列事件已发送成功但状态仍未回写成功，请尽快人工核查，避免超时回收后重复发送",
				"eventId", row.EventID, "topic", row.Topic)
		}
	}
	return nil
}

// restoreFetchedEvent 将提交线程池失败的事件恢复为 PENDING 状态
func (p *Processor) restoreFetchedEvent(ctx context.Context, row model.SysEventOutbox, cause error) {
	logger.Error(ctx, "消息队列事件提交发送线程池失败，准备恢复为 PENDING",
		"eventId", row.EventID, "topic", row.Topic, "error", cause)
	restoreCtx := context.WithoutCancel(ctx)
	if err := p.txExecutor.Execute(restoreCtx, func(txCtx context.Context) error {
		row.Status = model.OutboxStatusPending
		row.FetchTime = nil
		return p.outboxRepo.Update(txCtx, &row)
	}); err != nil {
		logger.Error(ctx, "消息队列事件恢复 PENDING 失败，将依赖 FETCHED 超时回收",
			"eventId", row.EventID, "topic", row.Topic, "error", err)
	}
}

// markSentSuccessWithRetry 重试将发件箱状态回写为发送成功
func (p *Processor) markSentSuccessWithRetry(ctx context.Context, row *model.SysEventOutbox, sentTime time.Time) bool {
	row.Status = model.OutboxStatusSentSuccess
	row.SentTime = &sentTime
	for retry := 1; retry <= sentSuccessPersistRetryTimes; retry++ {
		if updateErr := p.outboxRepo.Update(ctx, row); updateErr == nil {
			return true
		} else {
			logger.Error(ctx, "消息队列事件发送成功状态回写失败",
				"retry", retry, "eventId", row.EventID, "topic", row.Topic, "error", updateErr)
		}
		if retry < sentSuccessPersistRetryTimes {
			time.Sleep(sentSuccessPersistRetryInterval)
		}
	}
	return false
}

// shouldRestorePending 判断错误是否为线程池满、未启动或已停止等可恢复错误
func shouldRestorePending(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, ".queue_full") ||
		strings.Contains(msg, ".not_run") ||
		strings.Contains(msg, ".stopped")
}
