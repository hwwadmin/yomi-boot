// Package cfg 提供消息队列事件运行时配置
//
// @author zwei
package cfg

import (
	"strings"
	"time"
)

// CleanupRuntime 是单张表的过期数据清理运行时配置
type CleanupRuntime struct {
	Enabled    bool          // Enabled 是否启用该表清理
	Retention  time.Duration // Retention 是数据保留时长
	Frequency  time.Duration // Frequency 是清理任务轮询频率
	BatchSize  int           // BatchSize 是单批清理数量
	MaxBatches int           // MaxBatches 是单轮最大清理批数
}

// Runtime 消息队列事件运行时配置
type Runtime struct {
	ServiceName        string
	FetchFrequency     time.Duration
	FetchSize          int
	SentThreadCount    int
	OutboxDlqFrequency time.Duration // OutboxDlqFrequency 是发件箱死信队列轮询频率
	OutboxDlqSize      int           // OutboxDlqSize 是发件箱死信队列单次抓取数量
	OutboxDlqMaxRetry  int           // OutboxDlqMaxRetry 是发件箱死信队列最大重试次数
	FetchTimeout       time.Duration
	InboxDlqMaxRetry   int           // InboxDlqMaxRetry 是消费失败最大重试次数（不含首次处理），0 不重试，负数无限
	InboxDlqFrequency  time.Duration // InboxDlqFrequency 是收件箱死信后台重投频率
	InboxDlqSize       int           // InboxDlqSize 是收件箱死信单次重投数量
	OutboxCleanup      CleanupRuntime
	InboxCleanup       CleanupRuntime
	OutboxEnabled      bool // 发布侧（写 Outbox + 发件箱后台投递）
	InboxEnabled       bool // 消费侧（Receiver/Inbox）
}

// LockKey 返回消息队列事件消费侧分布式锁 Redis Key
func (c Runtime) LockKey(eventID string) string {
	return "event:receiver:lock:" + c.ServiceName + ":" + eventID
}

// RoundRobinName 构造轮询任务名
func (c Runtime) RoundRobinName(suffix string) string {
	return strings.TrimSpace(suffix) + "_" + c.ServiceName
}
