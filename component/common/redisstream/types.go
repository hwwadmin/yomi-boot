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

// DeadMetaField 是死信消息中存放框架元数据的独立字段名（JSON），避免覆盖业务字段
const DeadMetaField = "__rs_meta"

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

// 消费处理结果：框架对每条投递只调用一次 Handler，是否当场重试/阻塞由业务在 Handler 内决定。
// 整体为至少一次投递，重复消费时业务须幂等。
const (
	HandleAck   HandleResult = iota // 处理成功，XACK
	HandleRetry                     // 不 XACK，留在 PEL，继续拉取后续消息；再投递依赖 XAUTOCLAIM（可能被其它实例并发处理）
	HandleDead                      // 不可恢复：写入 dead stream 后 XACK
)

// Handler 消费处理函数；框架不代为进程内重试，业务可在函数内循环后再返回结果。
// 须按至少一次语义编写，自行保证幂等。
type Handler func(ctx context.Context, entry Entry) HandleResult

// GroupConfig 表示单个消费组订阅参数
type GroupConfig struct {
	Stream        string        // Stream 是逻辑 stream 名
	Group         string        // Group 是消费组名
	Consumer      string        // Consumer 是本实例消费者名；同一 group 下各实例必须唯一（建议 hostname+进程号）
	Start         string        // Start 是建组起始 id，空则 "$"（仅新消息）；常用 "0" 从头
	Count         int64         // Count 是单次拉取条数，0 使用全局默认
	Block         time.Duration // Block 是 XREADGROUP 阻塞时长，0 使用全局默认
	ClaimMinIdle  time.Duration // ClaimMinIdle 是 PEL 认领最小空闲，0 使用全局默认；过小会加大多实例重复投递概率
	ClaimInterval time.Duration // ClaimInterval 是 PEL 回收周期，0 使用全局默认
	DeadStream    string        // DeadStream 是死信 stream 逻辑名，空则 "{Stream}_dead"
}

// RuntimeConfig 表示组件运行时默认参数（由 Config 转换）
type RuntimeConfig struct {
	KeyPrefix     string        // KeyPrefix 是 stream key 前缀（与逻辑名直接拼接；非空时宜以 ':' 结尾）
	DefaultMaxLen int64         // DefaultMaxLen 是发布/死信默认近似裁剪长度，0 表示不裁剪
	Block         time.Duration // Block 是 XREADGROUP 默认阻塞时长
	Count         int64         // Count 是单次拉取默认条数
	ClaimMinIdle  time.Duration // ClaimMinIdle 是 PEL 认领默认最小空闲
	ClaimInterval time.Duration // ClaimInterval 是 PEL 回收默认周期
	StopTimeout   time.Duration // StopTimeout 是 Stop/Close 等待消费协程退出的超时
}
