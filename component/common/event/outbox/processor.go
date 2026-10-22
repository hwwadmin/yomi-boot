// Package outbox 提供发件箱后台抓取与发送能力
//
// @author zwei
package outbox

import (
	"context"
	"strings"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/db"
	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/common/worker"
	"gitee.com/hq_project/yomi-boot/core/errors"

	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
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
	queueSize      int        // queueSize 是发送队列的实际容量
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
		queueSize:      workCfg.Size,
	}, nil
}

// Capacity 返回发送队列的总容量
func (p *Processor) Capacity() int {
	if p == nil {
		return 0
	}
	return p.queueSize
}

// Pending 返回发送队列中待处理事件数
func (p *Processor) Pending() int {
	if p == nil || p.worker == nil {
		return 0
	}
	return p.worker.Pending()
}

// AvailableCapacity 返回发送队列当前可用容量
func (p *Processor) AvailableCapacity() int {
	if p == nil || p.worker == nil || p.queueSize <= 0 {
		return 0
	}
	return max(0, p.queueSize-p.worker.Pending())
}

// Offer 提交发件箱事件到发送线程池
func (p *Processor) Offer(ctx context.Context, row model.SysEventOutbox) {
	if err := p.tryOffer(row); err != nil {
		if shouldRestorePending(err) {
			p.restoreFetchedEvent(ctx, row, err)
			return
		}
		logger.Error(ctx, "消息队列事件提交发送线程池失败",
			"eventId", row.EventID, "topic", row.Topic, "error", err)
	}
}

// OfferBatch 批量提交发件箱事件；被线程池拒绝的事件统一批量恢复为 PENDING
func (p *Processor) OfferBatch(ctx context.Context, rows []model.SysEventOutbox) {
	if len(rows) == 0 {
		return
	}
	rejected := make([]model.SysEventOutbox, 0, len(rows))
	var cause error
	for i := range rows {
		if err := p.tryOffer(rows[i]); err != nil {
			if shouldRestorePending(err) {
				rejected = append(rejected, rows[i])
				if cause == nil {
					cause = err
				}
				continue
			}
			logger.Error(ctx, "消息队列事件提交发送线程池失败",
				"eventId", rows[i].EventID, "topic", rows[i].Topic, "error", err)
		}
	}
	if len(rejected) > 0 {
		p.restoreFetchedEvents(ctx, rejected, cause)
	}
}

// tryOffer 尝试将单个发件箱事件提交到发送线程池
func (p *Processor) tryOffer(row model.SysEventOutbox) error {
	cloned := row
	return p.worker.TrySubmit(func(taskCtx context.Context) error {
		return p.push(taskCtx, cloned)
	})
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
			updated, updateErr := p.outboxRepo.MarkSentFail(txCtx, row.ID)
			if updateErr != nil {
				return updateErr
			}
			if !updated {
				logger.Warn(txCtx, "消息队列事件推送失败但状态已变化，跳过 DLQ",
					"eventId", row.EventID, "topic", row.Topic)
				return nil
			}
			row.Status = model.OutboxStatusSentFail
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
	updated, updateErr := p.outboxRepo.MarkSentSuccess(persistCtx, row.ID, sentTime)
	if updateErr != nil {
		logger.Error(ctx, "消息队列事件发送成功但状态回写失败",
			"eventId", row.EventID, "topic", row.Topic, "error", updateErr)
		if !p.markSentSuccessWithRetry(persistCtx, &row, sentTime) {
			logger.Error(ctx, "消息队列事件已发送成功但状态仍未回写成功，请尽快人工核查，避免超时回收后重复发送",
				"eventId", row.EventID, "topic", row.Topic)
		}
	} else if !updated {
		logger.Warn(ctx, "消息队列事件发送成功但状态已非 FETCHED，跳过状态回写",
			"eventId", row.EventID, "topic", row.Topic)
	}
	return nil
}

// restoreFetchedEvent 将单个提交线程池失败的事件恢复为 PENDING 状态
func (p *Processor) restoreFetchedEvent(ctx context.Context, row model.SysEventOutbox, cause error) {
	p.restoreFetchedEvents(ctx, []model.SysEventOutbox{row}, cause)
}

// restoreFetchedEvents 将批量提交线程池失败的事件恢复为 PENDING 状态
func (p *Processor) restoreFetchedEvents(ctx context.Context, rows []model.SysEventOutbox, cause error) {
	if len(rows) == 0 {
		return
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	logger.Error(ctx, "消息队列事件提交发送线程池失败，准备批量恢复为 PENDING",
		"count", len(ids), "error", cause, "sample", buildOutboxSample(rows))
	restoreCtx := context.WithoutCancel(ctx)
	if err := p.outboxRepo.RestorePendingBatch(restoreCtx, ids); err != nil {
		logger.Error(ctx, "消息队列事件批量恢复 PENDING 失败，将依赖 FETCHED 超时回收",
			"count", len(ids), "error", err)
	}
}

// markSentSuccessWithRetry 重试将发件箱状态回写为发送成功
func (p *Processor) markSentSuccessWithRetry(ctx context.Context, row *model.SysEventOutbox, sentTime time.Time) bool {
	row.Status = model.OutboxStatusSentSuccess
	row.SentTime = &sentTime
	for retry := 1; retry <= sentSuccessPersistRetryTimes; retry++ {
		updated, updateErr := p.outboxRepo.MarkSentSuccess(ctx, row.ID, sentTime)
		if updateErr == nil {
			if updated {
				return true
			}
			logger.Warn(ctx, "消息队列事件发送成功但状态已非 FETCHED，跳过状态回写",
				"eventId", row.EventID, "topic", row.Topic)
			return true
		}
		logger.Error(ctx, "消息队列事件发送成功状态回写失败",
			"retry", retry, "eventId", row.EventID, "topic", row.Topic, "error", updateErr)
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
