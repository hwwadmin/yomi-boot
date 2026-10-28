// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import (
	"context"
	"time"
)

// recordPublishBudget 是访问记录投递的独立预算：发布队列满时最多等待该时长，
// 超时直接丢弃，避免把发布背压传给业务请求
const recordPublishBudget = 200 * time.Millisecond

// HttpRecordService 是 HTTP 访问记录的生产端处理门面：由采集中间件调用，
// 负责把记录写入内存队列或发布到 Kafka / Redis Stream。
type HttpRecordService interface {
	// Handle 处理一条 HTTP 访问记录，应尽快返回
	Handle(ctx context.Context, record *HttpRecord)
}

// publishContext 返回记录投递使用的短预算上下文；不随请求取消而放弃，
// 但最多等待 recordPublishBudget
func publishContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), recordPublishBudget)
}
