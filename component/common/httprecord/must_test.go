// HTTP 访问记录 Fail Fast 消费端点的单元测试
//
// @author zwei
package httprecord

import (
	"context"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
	"gitee.com/hq_project/yomi-boot/component/common/redisstream"
)

// TestMustStartMemoryRecordConsumer 验证内存消费者 Fail Fast 与成功路径
func TestMustStartMemoryRecordConsumer(t *testing.T) {
	t.Run("nil 服务应 panic", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("nil 服务应 panic")
			}
		}()
		MustStartMemoryRecordConsumer(context.Background(), nil, MemoryRecordConsumerConfig{Workers: 1},
			func(context.Context, *HttpRecord) RecordHandleResult { return HandleOK() })
	})

	t.Run("成功启动并消费", func(t *testing.T) {
		svc := NewMemoryHttpRecordService(8)
		defer svc.Close()

		collected := make(chan *HttpRecord, 4)
		MustStartMemoryRecordConsumer(context.Background(), svc, MemoryRecordConsumerConfig{Workers: 1},
			func(_ context.Context, record *HttpRecord) RecordHandleResult {
				collected <- record
				return HandleOK()
			})

		svc.Handle(context.Background(), &HttpRecord{TraceID: "mem-1", Method: "GET", Path: "/mem"})
		select {
		case record := <-collected:
			if record.TraceID != "mem-1" {
				t.Fatalf("消费记录不符: %+v", record)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("等待内存消费记录超时")
		}
	})
}

// TestMustStartRedisStreamRecordConsumer 验证 Redis Stream 消费者 Fail Fast 与成功路径
func TestMustStartRedisStreamRecordConsumer(t *testing.T) {
	t.Run("nil 消费者应 panic", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("nil 消费者应 panic")
			}
		}()
		MustStartRedisStreamRecordConsumer(context.Background(), nil, RedisStreamConsumerID,
			RedisStreamRecordConsumerConfig{Stream: "s", Group: "g", Consumer: "c"},
			func(context.Context, *HttpRecord) RecordHandleResult { return HandleOK() })
	})

	t.Run("成功启动并消费", func(t *testing.T) {
		client, cleanup := redistest.NewClient(t)
		defer cleanup()

		components, err := redisstream.NewComponents(redisstream.Config{}, client)
		if err != nil {
			t.Fatalf("NewComponents: %v", err)
		}
		defer func() { _ = components.Close() }()

		collected := make(chan *HttpRecord, 4)
		MustStartRedisStreamRecordConsumer(context.Background(), components.Consumer, RedisStreamConsumerID,
			RedisStreamRecordConsumerConfig{Stream: "httprecord.must", Group: "must-group", Consumer: "c1"},
			func(_ context.Context, record *HttpRecord) RecordHandleResult {
				collected <- record
				return HandleOK()
			})

		svc := NewRedisStreamHttpRecordService(components.Publisher, "httprecord.must")
		svc.Handle(context.Background(), &HttpRecord{TraceID: "rs-1", Method: "GET", Path: "/rs"})

		select {
		case record := <-collected:
			if record.TraceID != "rs-1" {
				t.Fatalf("消费记录不符: %+v", record)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("等待 Redis Stream 消费记录超时")
		}
	})
}
