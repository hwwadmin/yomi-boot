package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/worker"
	"github.com/hwwadmin/yomi-boot/component/core/errors"
)

var _ PublisherService = (*asyncPublisher)(nil)

// asyncPublisher 通过工作池异步调度底层同步发布。
// 入队失败（队列满、等待超时、工作池未启动或已停止）直接返回错误，不改为同步发送。
// 未提供工作池时，异步方法用调用方 ctx 当场发送。
type asyncPublisher struct {
	sync        PublisherService
	worker      worker.WorkerService
	sendTimeout time.Duration
}

// newAsyncPublisher 创建带异步发送能力的 Kafka 生产者装饰器
func newAsyncPublisher(sync PublisherService, publishWorker worker.WorkerService, sendTimeout time.Duration) PublisherService {
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

// PublishAsync 异步入队发布。成功仅表示任务已入队，不代表 Kafka 已确认。入队失败直接返回错误。
func (p *asyncPublisher) PublishAsync(ctx context.Context, req PublishRequest) error {
	if err := validatePublishRequest(req); err != nil {
		return err
	}
	cloned := clonePublishRequest(req)
	return p.enqueue(ctx, func(sendCtx context.Context) error {
		return p.sync.Publish(sendCtx, cloned)
	})
}

// PublishBatchAsync 异步入队批量发布；入队失败直接返回错误，策略同 PublishAsync
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
	return p.enqueue(ctx, func(sendCtx context.Context) error {
		return p.sync.PublishBatch(sendCtx, cloned)
	})
}

// PublishJSONAsync 序列化 JSON 后异步入队发布；入队失败直接返回错误，策略同 PublishAsync
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

// enqueue 把发送任务放入工作池。没有工作池时用调用方 ctx 当场发送。
// 入队失败原样返回，不改为同步发送。发送预算在任务执行时起算，入队等待不消耗发送超时。
func (p *asyncPublisher) enqueue(ctx context.Context, send func(context.Context) error) error {
	if p.worker == nil {
		return send(ctx)
	}
	return p.worker.SubmitContext(ctx, func(_ context.Context) error {
		sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.sendTimeout)
		defer cancel()
		return send(sendCtx)
	})
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

// cloneBytes 拷贝字节切片。nil 保持 nil；空切片保持为空切片，避免把空 key 收成 nil 后改走轮询分区
func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out
}
