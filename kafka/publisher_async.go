// Package kafka 提供 Kafka 消息发布能力
//
// @author zwei
package kafka

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/worker"
)

var _ PublisherService = (*asyncPublisher)(nil)

// asyncPublisher 通过工作池异步调度底层同步发布；队列满或不可用时自动降级为同步发送
type asyncPublisher struct {
	sync        PublisherService
	worker      worker.WorkerService
	sendTimeout time.Duration
}

// NewAsyncPublisher 创建带异步发送能力的 Kafka 生产者装饰器
func NewAsyncPublisher(sync PublisherService, publishWorker worker.WorkerService, sendTimeout time.Duration) PublisherService {
	return &asyncPublisher{
		sync:        sync,
		worker:      publishWorker,
		sendTimeout: sendTimeout,
	}
}

// Publish 向指定主题发布一条消息
func (p *asyncPublisher) Publish(ctx context.Context, req PublishRequest) error {
	return p.sync.Publish(ctx, req)
}

// PublishBatch 批量发布多条消息
func (p *asyncPublisher) PublishBatch(ctx context.Context, reqs []PublishRequest) error {
	return p.sync.PublishBatch(ctx, reqs)
}

// PublishJSON 将对象序列化为 JSON 后发布到指定主题
func (p *asyncPublisher) PublishJSON(ctx context.Context, topic string, key string, value any) error {
	return p.sync.PublishJSON(ctx, topic, key, value)
}

// PublishAsync 异步入队发布；队列满或工作池不可用时降级为同步 Publish
func (p *asyncPublisher) PublishAsync(ctx context.Context, req PublishRequest) error {
	if err := validatePublishRequest(req); err != nil {
		return err
	}
	cloned := clonePublishRequest(req)
	return p.enqueueOrSync(
		ctx,
		func(sendCtx context.Context) error { return p.sync.Publish(sendCtx, cloned) },
		func(callCtx context.Context) error { return p.sync.Publish(callCtx, cloned) },
	)
}

// PublishBatchAsync 异步入队批量发布；队列满或工作池不可用时降级为同步 PublishBatch
func (p *asyncPublisher) PublishBatchAsync(ctx context.Context, reqs []PublishRequest) error {
	if len(reqs) == 0 {
		return nil
	}
	cloned := make([]PublishRequest, len(reqs))
	for i, req := range reqs {
		if err := validatePublishRequest(req); err != nil {
			return errors.Wrap(err, "批量消息第 %d 条校验失败", i)
		}
		cloned[i] = clonePublishRequest(req)
	}
	return p.enqueueOrSync(
		ctx,
		func(sendCtx context.Context) error { return p.sync.PublishBatch(sendCtx, cloned) },
		func(callCtx context.Context) error { return p.sync.PublishBatch(callCtx, cloned) },
	)
}

// PublishJSONAsync 序列化 JSON 后异步入队发布；队列满或工作池不可用时降级为同步 PublishJSON
func (p *asyncPublisher) PublishJSONAsync(ctx context.Context, topic string, key string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return errors.Wrap(err, "序列化 Kafka JSON 消息失败")
	}

	var keyBytes []byte
	if key != "" {
		keyBytes = []byte(key)
	}

	return p.PublishAsync(ctx, PublishRequest{
		Topic: topic,
		Key:   keyBytes,
		Value: payload,
	})
}

// Close 先排空发布工作池，再释放底层生产者
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

// enqueueOrSync 尝试异步入队，队列满或工作池不可用时降级为同步发送
func (p *asyncPublisher) enqueueOrSync(
	ctx context.Context,
	asyncSend func(context.Context) error,
	syncSend func(context.Context) error,
) error {
	if p.worker == nil {
		return syncSend(ctx)
	}

	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.sendTimeout)
	err := p.worker.TrySubmit(func(_ context.Context) error {
		defer cancel()
		return asyncSend(sendCtx)
	})
	if err == nil {
		return nil
	}
	cancel()
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
		strings.Contains(msg, ".stopped")
}

// clonePublishRequest 深拷贝发布请求，避免异步执行时与调用方共享底层字节切片
func clonePublishRequest(req PublishRequest) PublishRequest {
	cloned := PublishRequest{
		Topic: req.Topic,
		Key:   cloneBytes(req.Key),
		Value: cloneBytes(req.Value),
	}
	if len(req.Headers) > 0 {
		cloned.Headers = make([]Header, len(req.Headers))
		for i, header := range req.Headers {
			cloned.Headers[i] = Header{
				Key:   header.Key,
				Value: cloneBytes(header.Value),
			}
		}
	}
	return cloned
}

// cloneBytes 拷贝字节切片
func cloneBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out
}
