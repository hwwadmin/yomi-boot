// Package redisstream 类型定义
//
// @author zwei
package redisstream

import (
	"context"
	"time"
)

// PayloadField 是 AddBytes / AddJSON 写入 Stream 的默认字段名
const PayloadField = "payload"

// Entry 表示一条已读取的 Stream 消息
type Entry struct {
	Stream string            // Stream 是逻辑 stream 名（不含 key_prefix）
	ID     string            // ID 是 Redis Stream entry id
	Values map[string]string // Values 是消息字段
}

// Payload 返回 payload 字段字节；字段不存在时返回 nil
func (e Entry) Payload() []byte {
	if e.Values == nil {
		return nil
	}
	v, ok := e.Values[PayloadField]
	if !ok {
		return nil
	}
	return []byte(v)
}

// HandleResult 表示消费处理结果
type HandleResult int

// 消费处理结果
const (
	HandleAck   HandleResult = iota // 处理成功，XACK
	HandleRetry                     // 可重试，不 XACK；进程内退避重试，崩溃后靠 XAUTOCLAIM
	HandleDead                      // 不可恢复：写入 dead stream 后 XACK
)

// Handler 消费处理函数
type Handler func(ctx context.Context, entry Entry) HandleResult

// GroupConfig 表示单个消费组订阅参数
type GroupConfig struct {
	Stream            string        // Stream 是逻辑 stream 名
	Group             string        // Group 是消费组名
	Consumer          string        // Consumer 是本实例消费者名
	Start             string        // Start 是建组起始 id，空则 "$"（仅新消息）；常用 "0" 从头
	Count             int64         // Count 是单次拉取条数，0 使用全局默认
	Block             time.Duration // Block 是 XREADGROUP 阻塞时长，0 使用全局默认
	ClaimMinIdle      time.Duration // ClaimMinIdle 是 PEL 认领最小空闲，0 使用全局默认
	ClaimInterval     time.Duration // ClaimInterval 是 PEL 回收周期，0 使用全局默认
	DeadStream        string        // DeadStream 是死信 stream 逻辑名，空则 "{Stream}.dead"
	MaxHandlerRetries *int          // MaxHandlerRetries 覆盖全局；nil 用全局
}

// RuntimeConfig 表示组件运行时默认参数（由 Config 转换）
type RuntimeConfig struct {
	KeyPrefix         string
	DefaultMaxLen     int64
	Block             time.Duration
	Count             int64
	ClaimMinIdle      time.Duration
	ClaimInterval     time.Duration
	MaxHandlerRetries int
}
