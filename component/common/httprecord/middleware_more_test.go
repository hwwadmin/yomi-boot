// 中间件辅助函数、wire 转换与各服务边界的补充测试
//
// @author zwei
package httprecord

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/redis/go-redis/v9"

	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
	"gitee.com/hq_project/yomi-boot/component/common/redisstream"
)

// TestBodyRecordWriter_WriteString 验证 WriteString 同步采集且客户端收到完整内容
func TestBodyRecordWriter_WriteString(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	w := newBodyRecordWriter(c.Writer, 3)

	n, err := w.WriteString("你好")
	if err != nil || n != len("你好") {
		t.Fatalf("WriteString 返回不符: n=%d err=%v", n, err)
	}
	if rec.Body.String() != "你好" {
		t.Fatalf("客户端应收到完整内容: %q", rec.Body.String())
	}
	if got := w.recordedBody(); got != "你" {
		t.Fatalf("采集应按字节上限截断: %q", got)
	}
}

// TestBodyRecordWriter_ZeroMaxSize 验证上限为 0 时不采集
func TestBodyRecordWriter_ZeroMaxSize(t *testing.T) {
	w := newTestBodyWriter(0)
	if _, err := w.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if got := w.recordedBody(); got != "" {
		t.Fatalf("上限为 0 不应采集，实际 %q", got)
	}
	if w.truncated {
		t.Fatal("上限为 0 不应标记截断")
	}
}

// TestBodyRecordWriterNilSafe 验证 nil 接收器的读取安全
func TestBodyRecordWriterNilSafe(t *testing.T) {
	var w *bodyRecordWriter
	if got := w.recordedBody(); got != "" {
		t.Fatalf("nil 接收器应返回空串，实际 %q", got)
	}
}

// errBody 始终返回读取错误的请求体
type errBody struct{}

// Read 返回预设错误
func (errBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }

// Close 空实现
func (errBody) Close() error { return nil }

// TestReadRequestBodyForRecord_Guards 验证空请求、空 body 与读取失败的兜底
func TestReadRequestBodyForRecord_Guards(t *testing.T) {
	if got, truncated := readRequestBodyForRecord(nil, 10); got != "" || truncated {
		t.Fatalf("nil 请求应返回空: %q %v", got, truncated)
	}
	req := httptest.NewRequest("POST", "/x", nil)
	req.Body = nil
	if got, truncated := readRequestBodyForRecord(req, 10); got != "" || truncated {
		t.Fatalf("nil body 应返回空: %q %v", got, truncated)
	}
	req2 := httptest.NewRequest("POST", "/x", errBody{})
	if got, truncated := readRequestBodyForRecord(req2, 10); got != "" || truncated {
		t.Fatalf("读取失败应返回空: %q %v", got, truncated)
	}
	if req2.Body != http.NoBody {
		t.Fatal("读取失败后 body 应重置为 NoBody")
	}
}

// TestWireRecordNilCases 验证 wire 转换的空值边界
func TestWireRecordNilCases(t *testing.T) {
	if wire := toWireRecord(nil); wire.TraceID != "" || wire.Path != "" || wire.Method != "" {
		t.Fatalf("nil 记录应转出零值 wire，实际 %+v", wire)
	}
	if got := fromWireRecord(wireRecord{}); got != nil {
		t.Fatalf("空 wire 应转出 nil，实际 %+v", got)
	}
}

// TestMemoryServiceBoundaries 验证内存服务的 Fail Fast 与空值/关闭边界
func TestMemoryServiceBoundaries(t *testing.T) {
	t.Run("queueSize 为 0 应 panic", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("queueSize 为 0 应 panic")
			}
		}()
		NewMemoryHttpRecordService(0)
	})

	t.Run("空记录与关闭后入队不 panic", func(t *testing.T) {
		svc := NewMemoryHttpRecordService(4)
		svc.Handle(context.Background(), nil)
		svc.Close()
		svc.Handle(context.Background(), &HttpRecord{TraceID: "closed"})
	})
}

// TestRedisStreamServicePanics 验证 Redis Stream 服务的 Fail Fast
func TestRedisStreamServicePanics(t *testing.T) {
	cases := []struct {
		name      string
		publisher redisstream.PublisherService
		stream    string
	}{
		{name: "publisher 为空", publisher: nil, stream: "s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s 应 panic", tc.name)
				}
			}()
			NewRedisStreamHttpRecordService(tc.publisher, tc.stream)
		})
	}

	defer func() {
		if recover() == nil {
			t.Fatal("stream 为空应 panic")
		}
	}()
	// 用真实组件充当非 nil publisher
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	components, err := redisstream.NewComponents(redisstream.Config{}, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()
	NewRedisStreamHttpRecordService(components.Publisher, "")
}

// TestRedisStreamServiceHandleNilRecord 验证空记录不发布
func TestRedisStreamServiceHandleNilRecord(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	components, err := redisstream.NewComponents(redisstream.Config{}, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()

	svc := NewRedisStreamHttpRecordService(components.Publisher, "httprecord.nil")
	svc.Handle(context.Background(), nil)
}

// TestStartRedisStreamRecordConsumerDropsInvalid 验证空 payload 与非法 JSON 消息被丢弃
func TestStartRedisStreamRecordConsumerDropsInvalid(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	components, err := redisstream.NewComponents(redisstream.Config{}, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()

	const stream = "httprecord.invalid"
	collected := make(chan *HttpRecord, 4)
	if err := StartRedisStreamRecordConsumer(context.Background(), components.Consumer, RedisStreamConsumerID,
		RedisStreamRecordConsumerConfig{Stream: stream, Group: "invalid-group", Consumer: "c1"},
		func(_ context.Context, record *HttpRecord) RecordHandleResult {
			collected <- record
			return HandleOK()
		}); err != nil {
		t.Fatalf("启动消费者失败: %v", err)
	}

	ctx := context.Background()
	// 无 payload 字段的消息
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"other": "x"}}).Err(); err != nil {
		t.Fatalf("写入空 payload 消息失败: %v", err)
	}
	// payload 为非法 JSON 的消息
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{redisstream.PayloadField: "not-json"}}).Err(); err != nil {
		t.Fatalf("写入非法消息失败: %v", err)
	}
	// 空对象 wire（Method/Path/TraceID 均空）应视为无效
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{redisstream.PayloadField: "{}"}}).Err(); err != nil {
		t.Fatalf("写入空 wire 消息失败: %v", err)
	}

	// 合法记录仍可正常消费
	svc := NewRedisStreamHttpRecordService(components.Publisher, stream)
	svc.Handle(ctx, &HttpRecord{TraceID: "valid-1", Method: "GET", Path: "/ok"})

	select {
	case record := <-collected:
		if record.TraceID != "valid-1" {
			t.Fatalf("非法消息后应继续消费有效记录，实际 %+v", record)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("等待有效消费记录超时")
	}
}

// retryMemoryHandler 返回 Retry 结果的内存消费 handler，用于触发 Retry 告警分支
func retryMemoryHandler() RecordHandler {
	return func(context.Context, *HttpRecord) RecordHandleResult { return HandleRetry() }
}

// TestMemoryConsumerRetryResultLogged 验证 handler 返回 Retry 时告警后丢弃且不 panic
func TestMemoryConsumerRetryResultLogged(t *testing.T) {
	svc := NewMemoryHttpRecordService(4)
	defer svc.Close()

	if err := StartMemoryRecordConsumer(context.Background(), svc, MemoryRecordConsumerConfig{Workers: 1}, retryMemoryHandler()); err != nil {
		t.Fatalf("启动消费者失败: %v", err)
	}
	svc.Handle(context.Background(), &HttpRecord{TraceID: "retry-1", Method: "GET", Path: "/retry"})
	// Retry 分支只打日志并丢弃，无暴露行为可断言；等待确保消费循环执行完毕不 panic
	time.Sleep(200 * time.Millisecond)
}

// panicMemoryHandler 触发 panic 的内存消费 handler
func panicMemoryHandler() RecordHandler {
	return func(context.Context, *HttpRecord) RecordHandleResult { panic("handler boom") }
}

// TestMemoryConsumerPanicRecovered 验证 handler panic 被捕获且消费循环存活
func TestMemoryConsumerPanicRecovered(t *testing.T) {
	svc := NewMemoryHttpRecordService(4)
	defer svc.Close()

	if err := StartMemoryRecordConsumer(context.Background(), svc, MemoryRecordConsumerConfig{Workers: 1}, panicMemoryHandler()); err != nil {
		t.Fatalf("启动消费者失败: %v", err)
	}
	svc.Handle(context.Background(), &HttpRecord{TraceID: "panic-1", Method: "GET", Path: "/panic"})
	time.Sleep(200 * time.Millisecond)

	// panic 后消费循环仍应存活：后续消息仍会被处理（返回 Retry 也仅告警）
	svc.Handle(context.Background(), &HttpRecord{TraceID: "after-panic", Method: "GET", Path: "/after"})
	time.Sleep(200 * time.Millisecond)
}

// TestRedisStreamServiceHandlePublishError 验证发布失败仅告警不 panic
func TestRedisStreamServiceHandlePublishError(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()
	components, err := redisstream.NewComponents(redisstream.Config{}, client)
	if err != nil {
		t.Fatalf("NewComponents: %v", err)
	}
	defer func() { _ = components.Close() }()

	svc := NewRedisStreamHttpRecordService(failingStreamPublisher{}, "httprecord.pub-err")
	// 发布失败应仅告警
	svc.Handle(context.Background(), &HttpRecord{TraceID: "err-1", Method: "GET", Path: "/x"})
}

// failingStreamPublisher 始终发布失败的 Redis Stream 发布器桩
type failingStreamPublisher struct{}

// Add 返回预设错误
func (failingStreamPublisher) Add(context.Context, string, map[string]string) (string, error) {
	return "", errors.New("stream down")
}

// AddBytes 返回预设错误
func (failingStreamPublisher) AddBytes(context.Context, string, []byte) (string, error) {
	return "", errors.New("stream down")
}

// AddJSON 返回预设错误
func (failingStreamPublisher) AddJSON(context.Context, string, any) (string, error) {
	return "", errors.New("stream down")
}

// AddAsync 返回预设错误
func (failingStreamPublisher) AddAsync(context.Context, string, map[string]string) error {
	return errors.New("stream down")
}

// AddBytesAsync 返回预设错误
func (failingStreamPublisher) AddBytesAsync(context.Context, string, []byte) error {
	return errors.New("stream down")
}

// AddJSONAsync 返回预设错误
func (failingStreamPublisher) AddJSONAsync(context.Context, string, any) error {
	return errors.New("stream down")
}

// Close 空实现
func (failingStreamPublisher) Close() error { return nil }
