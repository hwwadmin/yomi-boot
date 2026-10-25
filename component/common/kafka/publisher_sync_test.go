// Kafka 同步生产者降级方法与重试退避的单元测试
//
// @author zwei
package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// TestNewSyncPublisherNilWriterPanics 验证缺失 writer 时 Fail Fast
func TestNewSyncPublisherNilWriterPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("writer 为 nil 应 panic")
		}
	}()
	newSyncPublisher(nil)
}

// TestSyncPublisherAsyncDegenerateMethods 验证无工作池时异步方法退化为同步发送并透传错误
func TestSyncPublisherAsyncDegenerateMethods(t *testing.T) {
	// 指向必然不可达的地址，WriteMessages 返回错误即可覆盖委托链路
	p := newSyncPublisher(&kafkago.Writer{Addr: kafkago.TCP("127.0.0.1:1")})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := PublishRequest{Topic: "t", Value: []byte("v")}
	if err := p.PublishAsync(ctx, req); err == nil {
		t.Fatal("发送失败应返回错误")
	}
	if err := p.PublishBatchAsync(ctx, []PublishRequest{req}); err == nil {
		t.Fatal("批量发送失败应返回错误")
	}
	if err := p.PublishJSONAsync(ctx, "t", "k", map[string]string{"a": "1"}); err == nil {
		t.Fatal("JSON 发送失败应返回错误")
	}
	if err := p.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
}

// TestRetryWithBackoff 验证瞬时故障重试成功与上下文取消立即返回
func TestRetryWithBackoff(t *testing.T) {
	t.Run("瞬时故障后成功", func(t *testing.T) {
		calls := 0
		err := retryWithBackoff(context.Background(), "测试重试", func() error {
			calls++
			if calls < 2 {
				return errors.New("transient")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("重试后应成功: %v", err)
		}
		if calls != 2 {
			t.Fatalf("期望调用 2 次，实际 %d 次", calls)
		}
	})

	t.Run("上下文取消立即返回", func(t *testing.T) {
		calls := 0
		err := retryWithBackoff(context.Background(), "测试重试", func() error {
			calls++
			return context.Canceled
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("上下文错误应原样返回: %v", err)
		}
		if calls != 1 {
			t.Fatalf("上下文错误不应重试，实际调用 %d 次", calls)
		}
	})
}
