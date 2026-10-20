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
	ServiceName       string
	FetchFrequency    time.Duration
	FetchSize         int
	SentThreadCount   int
	DlqFrequency      time.Duration
	DlqSize           int
	DlqMaxRetry       int
	FetchTimeout      time.Duration
	ConsumerMaxRetry  int           // ConsumerMaxRetry 是消费失败最大重试次数（不含首次处理），0 不重试，负数无限
	InboxDlqFrequency time.Duration // InboxDlqFrequency 是收件箱死信后台重投频率
	InboxDlqSize      int           // InboxDlqSize 是收件箱死信单次重投数量
	PublisherEnabled  bool          // 发布侧（写 Outbox + 发件箱后台投递）
	ConsumerEnabled   bool          // 消费侧（Receiver/Inbox）
}

// LockKey 返回消息队列事件消费侧分布式锁 Redis Key
func (c Runtime) LockKey(eventID string) string {
	return "event:receiver:lock:" + c.ServiceName + ":" + eventID
}

// RoundRobinName 构造轮询任务名
func (c Runtime) RoundRobinName(suffix string) string {
	return strings.TrimSpace(suffix) + "_" + c.ServiceName
}
