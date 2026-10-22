// Package outbox 提供发件箱后台抓取与发送能力
//
// @author zwei
package outbox

import (
	"context"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/distlock"
	"gitee.com/hq_project/yomi-boot/component/common/event/cfg"
	"gitee.com/hq_project/yomi-boot/component/common/event/model"
	"gitee.com/hq_project/yomi-boot/component/common/event/repo"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/common/roundrobin"
	"gitee.com/hq_project/yomi-boot/core/errors"
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

// Start 启动抓取轮询；启动失败时返回错误
func (f *Fetcher) Start(ctx context.Context, factory distlock.DistLockFactory) error {
	name := f.cfg.RoundRobinName("MqEventPublisherFetch")
	if err := roundrobin.Start(ctx, roundrobin.Simple(name, f.cfg.FetchFrequency), f.doTask, factory); err != nil {
		return errors.Wrap(err, "event: 启动发件箱抓取器失败(%s)", name)
	}
	logger.InfoWithoutCtx("[消息队列事件]抓取器服务名", "name", name)
	return nil
}

// fetchSize 返回本轮实际抓取数量：取配置值与发送队列可用容量的较小者
func (f *Fetcher) fetchSize() int {
	if f.processor == nil || f.cfg.FetchSize <= 0 {
		return 0
	}
	return min(f.cfg.FetchSize, f.processor.AvailableCapacity())
}

// doTask 抓取待处理事件，标记为已抓取并提交到发送处理器
func (f *Fetcher) doTask(ctx context.Context) error {
	size := f.fetchSize()
	if size <= 0 {
		return nil
	}
	rows, err := f.outboxRepo.FetchPending(ctx, f.cfg.ServiceName, size)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	now := time.Now()
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		rows[i].Status = model.OutboxStatusFetched
		rows[i].FetchTime = &now
		ids = append(ids, rows[i].ID)
	}
	fetchedIDs, err := f.outboxRepo.MarkFetchedBatch(ctx, ids, now)
	if err != nil {
		return err
	}
	if len(fetchedIDs) == 0 {
		return nil
	}

	fetchedSet := make(map[int64]struct{}, len(fetchedIDs))
	for _, id := range fetchedIDs {
		fetchedSet[id] = struct{}{}
	}
	fetchedRows := make([]model.SysEventOutbox, 0, len(fetchedIDs))
	for i := range rows {
		if _, ok := fetchedSet[rows[i].ID]; ok {
			fetchedRows = append(fetchedRows, rows[i])
		}
	}
	f.processor.OfferBatch(ctx, fetchedRows)
	return nil
}
