// Package redisstream 提供 Redis Stream 发布能力
//
// @author zwei
package redisstream

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"gitee.com/hq_project/yomi-boot/core/errors"

	redisv9 "github.com/redis/go-redis/v9"
)

// PublisherService 表示 Redis Stream 发布能力
type PublisherService interface {
	// Add 向指定 stream 追加一条消息（字段 map）
	Add(ctx context.Context, stream string, values map[string]string) (id string, err error)
	// AddBytes 将 payload 写入默认字段后追加
	AddBytes(ctx context.Context, stream string, payload []byte) (id string, err error)
	// AddJSON 将对象序列化为 JSON 后写入默认字段并追加
	AddJSON(ctx context.Context, stream string, value any) (id string, err error)
	// AddAsync 异步入队发布；成功表示任务已进入工作池，不代表 Redis 已确认
	AddAsync(ctx context.Context, stream string, values map[string]string) error
	// AddBytesAsync 异步入队字节载荷发布
	AddBytesAsync(ctx context.Context, stream string, payload []byte) error
	// AddJSONAsync 序列化 JSON 后异步入队发布
	AddJSONAsync(ctx context.Context, stream string, value any) error
	// Close 释放发布侧资源（当前为幂等空操作，保留对称关闭语义）
	Close() error
}

// Publisher 封装 Redis Stream 发布
type Publisher struct {
	client        *redisv9.Client // client 是底层 Redis 客户端
	keyPrefix     string          // keyPrefix 拼在逻辑 stream 名之前
	defaultMaxLen int64           // defaultMaxLen 是默认近似裁剪长度，0 表示不裁剪
	closeMu       sync.Mutex      // closeMu 保护 closed 并发访问
	closed        bool            // closed 标记是否已关闭
}

var _ PublisherService = (*Publisher)(nil)

// NewPublisher 创建发布者；client 不可为 nil
func NewPublisher(client *redisv9.Client, runtime RuntimeConfig) *Publisher {
	if client == nil {
		panic("redisstream: NewPublisher client 不能为 nil")
	}
	return &Publisher{
		client:        client,
		keyPrefix:     runtime.KeyPrefix,
		defaultMaxLen: runtime.DefaultMaxLen,
	}
}

// Add 向指定 stream 追加一条消息
func (p *Publisher) Add(ctx context.Context, stream string, values map[string]string) (string, error) {
	if err := p.ensureOpen(); err != nil {
		return "", err
	}
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return "", errors.New("redisstream: stream 不能为空")
	}
	if len(values) == 0 {
		return "", errors.New("redisstream: values 不能为空")
	}

	args := &redisv9.XAddArgs{
		Stream: p.streamKey(stream),
		Values: toInterfaceMap(values),
	}
	if p.defaultMaxLen > 0 {
		args.MaxLen = p.defaultMaxLen
		args.Approx = true
	}

	id, err := p.client.XAdd(ctx, args).Result()
	if err != nil {
		return "", errors.Wrap(err, "Redis Stream XADD 失败")
	}
	return id, nil
}

// AddBytes 将 payload 写入默认字段后追加
func (p *Publisher) AddBytes(ctx context.Context, stream string, payload []byte) (string, error) {
	if payload == nil {
		return "", errors.New("redisstream: payload 不能为 nil")
	}
	return p.Add(ctx, stream, map[string]string{PayloadField: string(payload)})
}

// AddJSON 将对象序列化为 JSON 后写入默认字段并追加
func (p *Publisher) AddJSON(ctx context.Context, stream string, value any) (string, error) {
	if value == nil {
		return "", errors.New("redisstream: value 不能为 nil")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", errors.Wrap(err, "序列化 Redis Stream 消息失败")
	}
	return p.AddBytes(ctx, stream, raw)
}

// AddAsync 异步入队发布；无工作池时退化为同步 Add
func (p *Publisher) AddAsync(ctx context.Context, stream string, values map[string]string) error {
	_, err := p.Add(ctx, stream, values)
	return err
}

// AddBytesAsync 异步入队字节载荷发布；无工作池时退化为同步 AddBytes
func (p *Publisher) AddBytesAsync(ctx context.Context, stream string, payload []byte) error {
	_, err := p.AddBytes(ctx, stream, payload)
	return err
}

// AddJSONAsync 序列化 JSON 后异步入队发布；无工作池时退化为同步 AddJSON
func (p *Publisher) AddJSONAsync(ctx context.Context, stream string, value any) error {
	_, err := p.AddJSON(ctx, stream, value)
	return err
}

// Close 幂等关闭发布者
func (p *Publisher) Close() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	p.closed = true
	return nil
}

// ensureOpen 检查发布者是否仍可使用
func (p *Publisher) ensureOpen() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.closed {
		return errors.New("redisstream: Publisher 已关闭")
	}
	return nil
}

// streamKey 将逻辑 stream 名转换为带前缀的 Redis key（前缀与逻辑名直接拼接，前缀宜自带 ':'）
func (p *Publisher) streamKey(stream string) string {
	return p.keyPrefix + stream
}

// toInterfaceMap 将 string map 转为 XADD 所需的 any map
func toInterfaceMap(values map[string]string) map[string]any {
	out := make(map[string]any, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
