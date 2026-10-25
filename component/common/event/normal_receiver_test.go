// Package event 的普通消息事件接收器测试
//
// @author zwei
package event

import (
	"context"
	"testing"

	yomikafka "gitee.com/hq_project/yomi-boot/component/common/kafka"
)

// errIDGen 固定返回错误的雪花 ID 生成器桩
type errIDGen struct {
	err error
}

// NextID 返回预设错误
func (g errIDGen) NextID() (int64, error) { return 0, g.err }

// TestNormalReceiverWrapHandler 覆盖普通消费 handler 的各分支
func TestNormalReceiverWrapHandler(t *testing.T) {
	ctx := context.Background()
	valid := `{"eventId":"event-1","topic":"topic","payload":"payload-1"}`

	t.Run("action 未配置", func(t *testing.T) {
		r := NewNormalReceiver()
		result := r.WrapHandler(nil)(ctx, yomikafka.Message{Value: []byte(valid)})
		if result != yomikafka.AckDrop() {
			t.Fatalf("未配置 action 应 AckDrop，实际 %+v", result)
		}
	})

	t.Run("空消息抛弃", func(t *testing.T) {
		called := false
		r := NewNormalReceiver()
		result := r.WrapHandler(func(context.Context, Event) error {
			called = true
			return nil
		})(ctx, yomikafka.Message{Value: nil})
		if result != yomikafka.AckDrop() {
			t.Fatalf("空消息应 AckDrop，实际 %+v", result)
		}
		if called {
			t.Fatal("空消息不应调用业务函数")
		}
	})

	t.Run("非法消息抛弃", func(t *testing.T) {
		called := false
		r := NewNormalReceiver()
		result := r.WrapHandler(func(context.Context, Event) error {
			called = true
			return nil
		})(ctx, yomikafka.Message{Value: []byte("not-json")})
		if result != yomikafka.AckDrop() {
			t.Fatalf("非法消息应 AckDrop，实际 %+v", result)
		}
		if called {
			t.Fatal("非法消息不应调用业务函数")
		}
	})

	t.Run("正常消费", func(t *testing.T) {
		var got Event
		r := NewNormalReceiver()
		result := r.WrapHandler(func(_ context.Context, evt Event) error {
			got = evt
			return nil
		})(ctx, yomikafka.Message{Value: []byte(valid)})
		if result != yomikafka.AckOK() {
			t.Fatalf("正常消费应 AckOK，实际 %+v", result)
		}
		if got.EventID != "event-1" || got.Topic != "topic" || got.Payload != "payload-1" {
			t.Fatalf("业务函数收到的事件不符: %+v", got)
		}
	})

	t.Run("业务异常仍提交偏移量", func(t *testing.T) {
		r := NewNormalReceiver()
		result := r.WrapHandler(func(context.Context, Event) error {
			return errTestPublish
		})(ctx, yomikafka.Message{Value: []byte(valid)})
		if result != yomikafka.AckOK() {
			t.Fatalf("业务异常不应阻塞消费，应 AckOK，实际 %+v", result)
		}
	})
}
