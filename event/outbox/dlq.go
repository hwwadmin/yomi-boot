// Package outbox 提供发件箱后台抓取与发送能力
//
// @author zwei
package outbox

import (
	"context"
	"strings"
	"time"
	"yt-boot/db"
	"yt-boot/distlock"
	"yt-boot/event/cfg"
	"yt-boot/event/model"
	"yt-boot/event/repo"
	"yt-boot/logger"
	"yt-boot/roundrobin"
	"yt-boot/snowflake"

	ytkafka "yt-boot/kafka"
)

const dlqExhaustedErrorPrefix = "[EXHAUSTED] "

// DlqHandler 发件箱死信处理器
type DlqHandler struct {
	cfg            cfg.Runtime
	outboxRepo     *repo.OutboxRepo
	dlqRepo        *repo.OutboxDlqRepo
	kafkaPublisher ytkafka.PublisherService
	txExecutor     db.TxExecutor
	idGen          snowflake.Generator
}

// NewDlqHandler 创建死信处理器
func NewDlqHandler(
	runtime cfg.Runtime,
	outboxRepo *repo.OutboxRepo,
	dlqRepo *repo.OutboxDlqRepo,
	kafkaPublisher ytkafka.PublisherService,
	txExecutor db.TxExecutor,
	idGen snowflake.Generator,
) *DlqHandler {
	return &DlqHandler{
		cfg:            runtime,
		outboxRepo:     outboxRepo,
		dlqRepo:        dlqRepo,
		kafkaPublisher: kafkaPublisher,
		txExecutor:     txExecutor,
		idGen:          idGen,
	}
}

// Enqueue 将失败事件写入 DLQ
func (d *DlqHandler) Enqueue(ctx context.Context, row model.SysEventOutbox, cause error) error {
	id, err := d.idGen.NextID()
	if err != nil {
		return err
	}
	dlq := &model.SysEventOutboxDlq{
		ServiceName:   row.ServiceName,
		EventOutboxID: row.ID,
		ErrorMsg:      coreErrorMessage(cause),
		BaseModel: db.BaseModel{
			ID: id,
		},
	}
	return d.dlqRepo.Insert(ctx, dlq)
}

// ResetRetryCountInTimeRange 将指定创建时间范围内的 DLQ 重试计数清零，供人工恢复后重新进入重试
func (d *DlqHandler) ResetRetryCountInTimeRange(ctx context.Context, from, to time.Time) (int64, error) {
	affected, err := d.dlqRepo.ResetRetryCountInTimeRange(ctx, d.cfg.ServiceName, from, to)
	if err != nil {
		return 0, err
	}
	logger.Info(ctx, "DLQ 重试计数已人工重置",
		"serviceName", d.cfg.ServiceName,
		"from", from,
		"to", to,
		"affected", affected)
	return affected, nil
}

// Start 启动 DLQ 重试轮询
func (d *DlqHandler) Start(ctx context.Context, factory distlock.DistLockFactory) {
	name := d.cfg.RoundRobinName("MqEventPublisherDlq")
	roundrobin.Start(ctx, roundrobin.Simple(name, d.cfg.DlqFrequency), d.processDlq, factory)
	logger.InfoWithoutCtx("[消息队列事件]死信队列服务名", "name", name)
}

// processDlq 从 DLQ 拉取待重试记录并逐一处理
func (d *DlqHandler) processDlq(ctx context.Context) error {
	entries, err := d.dlqRepo.Fetch(ctx, d.cfg.ServiceName, d.cfg.DlqSize, d.cfg.DlqMaxRetry, time.Now())
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.EventOutboxID)
	}
	outboxes, err := d.outboxRepo.FindByIDs(ctx, ids)
	if err != nil {
		return err
	}
	outboxMap := make(map[int64]model.SysEventOutbox, len(outboxes))
	for _, row := range outboxes {
		outboxMap[row.ID] = row
	}

	for _, dlq := range entries {
		d.processOne(ctx, dlq, outboxMap[dlq.EventOutboxID])
	}
	return nil
}

// processOne 处理单条 DLQ 记录，根据发件箱状态决定重试推送或清理
func (d *DlqHandler) processOne(ctx context.Context, dlq model.SysEventOutboxDlq, outbox model.SysEventOutbox) {
	now := time.Now()
	if outbox.ID == 0 {
		if err := d.dlqRepo.Delete(ctx, dlq.ID); err != nil {
			logger.Error(ctx, "DLQ 清理孤立记录失败", "dlqId", dlq.ID, "error", err)
		}
		return
	}
	if outbox.Status == model.OutboxStatusSentSuccess {
		if err := d.dlqRepo.Delete(ctx, dlq.ID); err != nil {
			logger.Error(ctx, "DLQ 清理已成功事件记录失败",
				"eventId", outbox.EventID, "topic", outbox.Topic, "error", err)
		}
		return
	}

	if dlq.RetryCount >= d.cfg.DlqMaxRetry {
		return
	}

	evt := model.WireEvent{
		EventID: outbox.EventID,
		Topic:   outbox.Topic,
		Payload: outbox.Payload,
	}
	if err := d.kafkaPublisher.PublishJSON(ctx, outbox.Topic, outbox.EventID, evt); err != nil {
		logger.Error(ctx, "DLQ 消息队列事件推送失败",
			"eventId", outbox.EventID, "eventOutboxId", dlq.EventOutboxID, "topic", outbox.Topic, "error", err)
		dlq.RetryCount++
		dlq.RetryLastTime = &now
		retryAfter := now.Add(dlqRetryDelay(dlq.RetryCount))
		dlq.RetryAfterTime = &retryAfter
		if dlq.RetryCount >= d.cfg.DlqMaxRetry {
			dlq.ErrorMsg = dlqExhaustedErrorMsg(dlq.ErrorMsg)
			logger.Error(ctx, "DLQ 消息重试次数已超限，已停止重试",
				"eventId", outbox.EventID,
				"eventOutboxId", dlq.EventOutboxID,
				"topic", outbox.Topic,
				"retryCount", dlq.RetryCount,
				"dlqMaxRetry", d.cfg.DlqMaxRetry,
				"error", err)
		}
		if updateErr := d.dlqRepo.Update(ctx, &dlq); updateErr != nil {
			logger.Error(ctx, "DLQ 重试计数更新失败",
				"eventId", outbox.EventID, "error", updateErr)
		}
		return
	}

	logger.Info(ctx, "DLQ 消息队列事件推送成功", "eventId", outbox.EventID, "topic", outbox.Topic)
	if err := d.dlqRepo.DeleteWithOutboxSuccess(ctx, &outbox, &dlq, now, d.outboxRepo); err != nil {
		logger.Error(ctx, "DLQ 消息已发送成功但状态回写失败",
			"eventId", outbox.EventID, "topic", outbox.Topic, "error", err)
		if !d.markSentSuccessAndClearDlqWithRetry(ctx, &outbox, &dlq, now) {
			logger.Error(ctx, "DLQ 消息已发送成功但状态或 DLQ 清理仍未完成，请尽快人工核查，避免重复发送",
				"eventId", outbox.EventID, "topic", outbox.Topic)
		}
	}
}

// markSentSuccessAndClearDlqWithRetry 重试标记发件箱为发送成功并清理 DLQ 记录
func (d *DlqHandler) markSentSuccessAndClearDlqWithRetry(
	ctx context.Context,
	outbox *model.SysEventOutbox,
	dlq *model.SysEventOutboxDlq,
	sentTime time.Time,
) bool {
	for retry := 1; retry <= sentSuccessPersistRetryTimes; retry++ {
		err := d.txExecutor.Execute(ctx, func(txCtx context.Context) error {
			return d.dlqRepo.DeleteWithOutboxSuccess(txCtx, outbox, dlq, sentTime, d.outboxRepo)
		})
		if err == nil {
			return true
		}
		logger.Error(ctx, "DLQ 消息发送成功后的状态回写补偿失败",
			"retry", retry, "eventId", outbox.EventID, "topic", outbox.Topic, "error", err)
		if retry < sentSuccessPersistRetryTimes {
			time.Sleep(sentSuccessPersistRetryInterval)
		}
	}
	return false
}

// coreErrorMessage 提取错误信息字符串，nil 时返回空字符串
func coreErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// dlqExhaustedErrorMsg 为已耗尽重试次数的错误信息追加标记前缀
func dlqExhaustedErrorMsg(original string) string {
	if strings.HasPrefix(original, dlqExhaustedErrorPrefix) {
		return original
	}
	if original == "" {
		return dlqExhaustedErrorPrefix + "已达最大重试次数"
	}
	return dlqExhaustedErrorPrefix + "已达最大重试次数，" + original
}
