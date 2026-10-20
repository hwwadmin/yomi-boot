// Package event 提供收件箱死信后台重投能力
//
// @author zwei
package event

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/event/cfg"
	"gitee.com/hq_project/yomi-boot/event/model"
	"gitee.com/hq_project/yomi-boot/event/repo"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/roundrobin"

	yomikafka "gitee.com/hq_project/yomi-boot/kafka"
)

// InboxDlqHandler 收件箱死信后台重投处理器
type InboxDlqHandler struct {
	cfg            cfg.Runtime
	dlqRepo        *repo.InboxDlqRepo
	inboxRepo      *repo.InboxRepo
	kafkaPublisher yomikafka.PublisherService
}

// NewInboxDlqHandler 创建收件箱死信后台重投处理器
func NewInboxDlqHandler(
	runtime cfg.Runtime,
	dlqRepo *repo.InboxDlqRepo,
	inboxRepo *repo.InboxRepo,
	kafkaPublisher yomikafka.PublisherService,
) *InboxDlqHandler {
	return &InboxDlqHandler{
		cfg:            runtime,
		dlqRepo:        dlqRepo,
		inboxRepo:      inboxRepo,
		kafkaPublisher: kafkaPublisher,
	}
}

// Start 启动收件箱死信重投轮询；启动失败时返回错误
func (d *InboxDlqHandler) Start(ctx context.Context, factory distlock.DistLockFactory) error {
	if d == nil || d.dlqRepo == nil || d.inboxRepo == nil {
		return errors.New("event: 收件箱死信仓储未初始化")
	}
	if d.kafkaPublisher == nil {
		logger.WarnWithoutCtx("[消息队列事件]收件箱死信重投未启动：Kafka 发布者未初始化")
		return nil
	}
	name := d.cfg.RoundRobinName("MqEventInboxDlqReplay")
	if err := roundrobin.Start(ctx, roundrobin.Simple(name, d.cfg.InboxDlqFrequency), d.processDlq, factory); err != nil {
		return errors.Wrap(err, "event: 启动收件箱死信重投失败(%s)", name)
	}
	logger.InfoWithoutCtx("[消息队列事件]收件箱死信重投服务名", "name", name)
	return nil
}

// processDlq 拉取到期的收件箱死信并逐条重新投递
func (d *InboxDlqHandler) processDlq(ctx context.Context) error {
	rows, err := d.dlqRepo.Fetch(ctx, d.cfg.ServiceName, d.cfg.InboxDlqSize, time.Now())
	if err != nil {
		return err
	}
	for i := range rows {
		d.processOne(ctx, &rows[i])
	}
	return nil
}

// processOne 处理单条收件箱死信：已消费则清理，未消费则重新投递
func (d *InboxDlqHandler) processOne(ctx context.Context, row *model.SysEventInboxDlq) {
	exists, err := d.inboxRepo.Exists(ctx, row.ServiceName, row.EventID)
	if err != nil {
		logger.Error(ctx, "收件箱死信重投前查询消费状态失败",
			"eventId", row.EventID, "topic", row.Topic, "error", err)
		return
	}
	if exists {
		if err := d.dlqRepo.Delete(ctx, row.ID); err != nil {
			logger.Error(ctx, "收件箱死信已消费但清理失败",
				"eventId", row.EventID, "topic", row.Topic, "error", err)
		}
		return
	}

	evt := model.WireEvent{
		EventID: row.EventID,
		Topic:   row.Topic,
		Payload: row.Payload,
	}
	if err := d.kafkaPublisher.PublishJSON(ctx, row.Topic, row.EventID, evt); err != nil {
		now := time.Now()
		retryCount := row.RetryCount + 1
		afterTime := now.Add(d.cfg.InboxDlqFrequency)
		logger.Error(ctx, "收件箱死信重新投递失败",
			"eventId", row.EventID, "topic", row.Topic,
			"retryCount", retryCount, "error", err)
		if updateErr := d.dlqRepo.UpdateRetry(ctx, row.ID, retryCount, now, afterTime); updateErr != nil {
			logger.Error(ctx, "收件箱死信重投次数更新失败",
				"eventId", row.EventID, "topic", row.Topic, "error", updateErr)
		}
		return
	}

	if err := d.dlqRepo.Delete(ctx, row.ID); err != nil {
		logger.Error(ctx, "收件箱死信重新投递成功但清理失败",
			"eventId", row.EventID, "topic", row.Topic, "error", err)
		return
	}
	logger.Info(ctx, "收件箱死信已重新投递", "eventId", row.EventID, "topic", row.Topic)
}
