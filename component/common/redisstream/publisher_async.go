// Package redisstream 提供 Redis Stream 异步发布能力
//
// @author zwei
package redisstream

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/worker"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

var _ PublisherService = (*asyncPublisher)(nil)

// asyncPublisher 通过工作池异步调度底层同步发布；队列满时按工作池 submit_timeout 等待空位，
// 等待超时或工作池不可用时自动降级为同步发送
type asyncPublisher struct {
	sync        PublisherService
	worker      worker.WorkerService
	sendTimeout time.Duration
}

// NewAsyncPublisher 创建带异步发送能力的 Redis Stream 发布者装饰器
func NewAsyncPublisher(sync PublisherService, publishWorker worker.WorkerService, sendTimeout time.Duration) PublisherService {
	if sendTimeout <= 0 {
		sendTimeout = defaultPublishSendTimeout
	}
	return &asyncPublisher{
		sync:        sync,
		worker:      publishWorker,
		sendTimeout: sendTimeout,
	}
}

// Add 向指定 stream 追加一条消息
func (p *asyncPublisher) Add(ctx context.Context, stream string, values map[string]string) (string, error) {
	return p.sync.Add(ctx, stream, values)
}

// AddBytes 将 payload 写入默认字段后追加
func (p *asyncPublisher) AddBytes(ctx context.Context, stream string, payload []byte) (string, error) {
	return p.sync.AddBytes(ctx, stream, payload)
}

// AddJSON 将对象序列化为 JSON 后写入默认字段并追加
func (p *asyncPublisher) AddJSON(ctx context.Context, stream string, value any) (string, error) {
	return p.sync.AddJSON(ctx, stream, value)
}

// AddAsync 异步入队发布；队列满时按 submit_timeout 等待空位，等待超时或工作池不可用时降级为同步 Add
func (p *asyncPublisher) AddAsync(ctx context.Context, stream string, values map[string]string) error {
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return errors.New("redisstream: stream 不能为空")
	}
	if len(values) == 0 {
		return errors.New("redisstream: values 不能为空")
	}
	clonedStream := stream
	clonedValues := cloneStringMap(values)
	return p.enqueueOrSync(
		ctx,
		func(sendCtx context.Context) error {
			_, err := p.sync.Add(sendCtx, clonedStream, clonedValues)
			return err
		},
		func(callCtx context.Context) error {
			_, err := p.sync.Add(callCtx, clonedStream, clonedValues)
			return err
		},
	)
}

// AddBytesAsync 异步入队字节载荷发布；队列满时按 submit_timeout 等待空位，等待超时或工作池不可用时降级为同步 AddBytes
func (p *asyncPublisher) AddBytesAsync(ctx context.Context, stream string, payload []byte) error {
	if payload == nil {
		return errors.New("redisstream: payload 不能为 nil")
	}
	return p.AddAsync(ctx, stream, map[string]string{PayloadField: string(payload)})
}

// AddJSONAsync 序列化 JSON 后异步入队发布；队列满时按 submit_timeout 等待空位，等待超时或工作池不可用时降级为同步 AddJSON
func (p *asyncPublisher) AddJSONAsync(ctx context.Context, stream string, value any) error {
	if value == nil {
		return errors.New("redisstream: value 不能为 nil")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return errors.Wrap(err, "序列化 Redis Stream 消息失败")
	}
	return p.AddBytesAsync(ctx, stream, payload)
}

// Close 先排空发布工作池，再释放底层发布者
func (p *asyncPublisher) Close() error {
	if p.worker != nil {
		_ = p.worker.Close()
		p.worker.Wait()
	}
	if p.sync != nil {
		return p.sync.Close()
	}
	return nil
}

// enqueueOrSync 尝试异步入队；队列满时按工作池 submit_timeout 等待空位，
// 等待超时或工作池不可用时降级为同步发送，调用方 ctx 取消/到期则直接失败不降级
func (p *asyncPublisher) enqueueOrSync(
	ctx context.Context,
	asyncSend func(context.Context) error,
	syncSend func(context.Context) error,
) error {
	if p.worker == nil {
		return syncSend(ctx)
	}

	err := p.worker.SubmitContext(ctx, func(workerCtx context.Context) error {
		// 发送 ctx 与工作池自身 ctx 解耦：工作池停机排空时会取消自身 ctx，排队任务仍须以独立超时完成发送
		sendCtx, cancel := context.WithTimeout(context.WithoutCancel(workerCtx), p.sendTimeout)
		defer cancel()
		return asyncSend(sendCtx)
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if shouldFallbackSync(err) {
		return syncSend(ctx)
	}
	return err
}

// shouldFallbackSync 判断异步入队失败是否应降级为同步发送
func shouldFallbackSync(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, ".queue_full") ||
		strings.Contains(msg, ".not_run") ||
		strings.Contains(msg, ".stopped") ||
		strings.Contains(msg, ".submit_timeout")
}

// cloneStringMap 拷贝字符串 map，避免异步执行时与调用方共享底层数据
func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
