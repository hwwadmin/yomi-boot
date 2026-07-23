// Package outbox 提供发件箱后台抓取与发送能力
//
// @author zwei
package outbox

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/event/cfg"
	"gitee.com/hq_project/yomi-boot/event/model"
	"gitee.com/hq_project/yomi-boot/event/repo"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/roundrobin"
)

// Fetcher 发件箱抓取器
type Fetcher struct {
	cfg        cfg.Runtime
	outboxRepo *repo.OutboxRepo
	processor  *Processor
}

// NewFetcher 创建发件箱抓取器
func NewFetcher(runtime cfg.Runtime, outboxRepo *repo.OutboxRepo, processor *Processor) *Fetcher {
	return &Fetcher{
		cfg:        runtime,
		outboxRepo: outboxRepo,
		processor:  processor,
	}
}

// Start 启动抓取轮询
func (f *Fetcher) Start(ctx context.Context, factory distlock.DistLockFactory) {
	name := f.cfg.RoundRobinName("MqEventPublisherFetch")
	roundrobin.Start(ctx, roundrobin.Simple(name, f.cfg.FetchFrequency), f.doTask, factory)
	logger.InfoWithoutCtx("[消息队列事件]抓取器服务名", "name", name)
}

// doTask 抓取待处理事件，标记为已抓取并提交到发送处理器
func (f *Fetcher) doTask(ctx context.Context) error {
	rows, err := f.outboxRepo.FetchPending(ctx, f.cfg.ServiceName, f.cfg.FetchSize)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	now := time.Now()
	for i := range rows {
		rows[i].Status = model.OutboxStatusFetched
		rows[i].FetchTime = &now
	}
	if err := f.outboxRepo.UpdateBatch(ctx, rows); err != nil {
		return err
	}

	for i := range rows {
		f.processor.Offer(ctx, rows[i])
	}
	return nil
}
