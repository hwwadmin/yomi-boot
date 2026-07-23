// Package outbox 提供发件箱后台抓取与发送能力
//
// @author zwei
package outbox

import (
	"context"
	"fmt"
	"gitee.com/hq_project/yomi-boot/distlock"
	"gitee.com/hq_project/yomi-boot/event/cfg"
	"gitee.com/hq_project/yomi-boot/event/model"
	"gitee.com/hq_project/yomi-boot/event/repo"
	"gitee.com/hq_project/yomi-boot/logger"
	"gitee.com/hq_project/yomi-boot/roundrobin"
	"strings"
	"time"
)

const fetchTimeoutLogSampleSize = 10

// FetchTimeout 发件箱抓取超时回收器
type FetchTimeout struct {
	cfg        cfg.Runtime
	outboxRepo *repo.OutboxRepo
}

// NewFetchTimeout 创建抓取超时回收器
func NewFetchTimeout(runtime cfg.Runtime, outboxRepo *repo.OutboxRepo) *FetchTimeout {
	return &FetchTimeout{cfg: runtime, outboxRepo: outboxRepo}
}

// Start 启动超时回收轮询
func (f *FetchTimeout) Start(ctx context.Context, factory distlock.DistLockFactory) {
	name := f.cfg.RoundRobinName("MqEventPublisherFetchTimeout")
	roundrobin.Start(ctx, roundrobin.Simple(name, f.cfg.FetchFrequency), f.doTask, factory)
	logger.InfoWithoutCtx("[消息队列事件]超时抓取器服务名", "name", name)
}

// doTask 检测并重置抓取超时的事件为待处理状态
func (f *FetchTimeout) doTask(ctx context.Context) error {
	timeoutBefore := time.Now().Add(-f.cfg.FetchTimeout)
	rows, err := f.outboxRepo.FetchFetchedTimeout(ctx, f.cfg.ServiceName, timeoutBefore)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	logger.Warn(ctx, "[消息队列事件]检测到 FETCHED 超时消息，准备重置为 PENDING",
		"count", len(rows),
		"serviceName", f.cfg.ServiceName,
		"fetchTimeout", f.cfg.FetchTimeout.String(),
		"sample", buildTimeoutSample(rows),
	)

	for i := range rows {
		rows[i].Status = model.OutboxStatusPending
		rows[i].FetchTime = nil
	}
	if err := f.outboxRepo.UpdateBatch(ctx, rows); err != nil {
		return err
	}

	logger.Warn(ctx, "[消息队列事件]FETCHED 超时消息重置完成",
		"count", len(rows), "serviceName", f.cfg.ServiceName)
	return nil
}

// buildTimeoutSample 构造超时事件的日志采样字符串
func buildTimeoutSample(rows []model.SysEventOutbox) string {
	limit := fetchTimeoutLogSampleSize
	if len(rows) < limit {
		limit = len(rows)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		row := rows[i]
		fetchTime := ""
		if row.FetchTime != nil {
			fetchTime = row.FetchTime.String()
		}
		parts = append(parts, fmt.Sprintf("eventId=%s|topic=%s|fetchTime=%s", row.EventID, row.Topic, fetchTime))
	}
	return strings.Join(parts, ", ")
}
