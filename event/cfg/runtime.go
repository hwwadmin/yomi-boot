// Package cfg 提供消息队列事件运行时配置
//
// @author zwei
package cfg

import (
	"strings"
	"time"
)

// Runtime 消息队列事件运行时配置
type Runtime struct {
	ServiceName      string
	FetchFrequency   time.Duration
	FetchSize        int
	SentThreadCount  int
	DlqFrequency     time.Duration
	DlqSize          int
	DlqMaxRetry      int
	FetchTimeout     time.Duration
	PublisherEnabled bool // 发布侧（写 Outbox + 发件箱后台投递）
	ConsumerEnabled  bool // 消费侧（Receiver/Inbox）
}

// LockKey 返回消息队列事件消费侧分布式锁 Redis Key
func (c Runtime) LockKey(eventID string) string {
	return "event:receiver:lock:" + c.ServiceName + ":" + eventID
}

// RoundRobinName 构造轮询任务名
func (c Runtime) RoundRobinName(suffix string) string {
	return strings.TrimSpace(suffix) + "_" + c.ServiceName
}
