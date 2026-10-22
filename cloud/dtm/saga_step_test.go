package dtm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"

	"github.com/dtm-labs/client/dtmcli"
)

// newBlockingGidServer 返回一个收到请求后一直等到客户端断开的 /newGid 服务
func newBlockingGidServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
}

// newJSONGidServer 返回一个固定状态码与响应体的 /newGid 服务
func newJSONGidServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// TestNewSagaHonorsContext 验证调用方 ctx 超时会中断 newGid 请求
func TestNewSagaHonorsContext(t *testing.T) {
	srv := newBlockingGidServer(t)
	defer srv.Close()

	client := NewClient(nil, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	saga, err := client.NewSaga(ctx)
	elapsed := time.Since(start)
	if err == nil || saga != nil {
		t.Fatalf("ctx 超时后应返回错误，saga=%v err=%v", saga, err)
	}
	if elapsed > time.Second {
		t.Fatalf("调用方 50ms 超时后仍等待 %s", elapsed)
	}
}

// TestNewSagaClientTimeout 验证客户端超时会中断 newGid 请求
func TestNewSagaClientTimeout(t *testing.T) {
	srv := newBlockingGidServer(t)
	defer srv.Close()

	client := NewClient(nil, srv.URL, WithTimeout(50*time.Millisecond))

	start := time.Now()
	saga, err := client.NewSaga(context.Background())
	elapsed := time.Since(start)
	if err == nil || saga != nil {
		t.Fatalf("客户端超时后应返回错误，saga=%v err=%v", saga, err)
	}
	if elapsed > time.Second {
		t.Fatalf("50ms 客户端超时后仍等待 %s", elapsed)
	}
}

// TestNewSagaSetsTransRequestTimeout 验证超时会传给 Saga 的控制面请求
func TestNewSagaSetsTransRequestTimeout(t *testing.T) {
	srv := newJSONGidServer(t, http.StatusOK, `{"gid":"g-1"}`)
	defer srv.Close()

	saga, err := NewClient(nil, srv.URL, WithTimeout(7*time.Second)).NewSaga(context.Background())
	if err != nil {
		t.Fatalf("创建 Saga 失败: %v", err)
	}
	if saga.RequestTimeout != 7 {
		t.Fatalf("RequestTimeout 期望 7，实际 %d", saga.RequestTimeout)
	}
}

// TestNewSagaDefaultTimeout 验证默认超时会落到 Saga 上
func TestNewSagaDefaultTimeout(t *testing.T) {
	srv := newJSONGidServer(t, http.StatusOK, `{"gid":"g-1"}`)
	defer srv.Close()

	saga, err := NewClient(nil, srv.URL).NewSaga(context.Background())
	if err != nil {
		t.Fatalf("创建 Saga 失败: %v", err)
	}
	want := int64(DefaultTimeout / time.Second)
	if saga.RequestTimeout != want {
		t.Fatalf("RequestTimeout 期望 %d，实际 %d", want, saga.RequestTimeout)
	}
}

// TestNewSagaRoundsSubSecondTimeoutUp 验证不足 1 秒的配置不会退化成“不超时”
func TestNewSagaRoundsSubSecondTimeoutUp(t *testing.T) {
	srv := newJSONGidServer(t, http.StatusOK, `{"gid":"g-1"}`)
	defer srv.Close()

	saga, err := NewClient(nil, srv.URL, WithTimeout(500*time.Millisecond)).NewSaga(context.Background())
	if err != nil {
		t.Fatalf("创建 Saga 失败: %v", err)
	}
	if saga.RequestTimeout != 1 {
		t.Fatalf("不足 1 秒的超时应按 1 秒下发，实际 %d", saga.RequestTimeout)
	}
}

// TestNewSagaRejectsNon200 验证非 200 响应会返回可读错误
func TestNewSagaRejectsNon200(t *testing.T) {
	srv := newJSONGidServer(t, http.StatusInternalServerError, `{"gid":"g-1"}`)
	defer srv.Close()

	saga, err := NewClient(nil, srv.URL).NewSaga(context.Background())
	if err == nil || saga != nil {
		t.Fatalf("非 200 响应应返回错误，saga=%v err=%v", saga, err)
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("错误信息应包含状态码，实际: %v", err)
	}
}

// TestAddSagaStepRejectsNilSaga 验证 saga 为空时返回错误而非 panic
func TestAddSagaStepRejectsNilSaga(t *testing.T) {
	client := NewClient(nil, "http://127.0.0.1:36789")
	err := client.AddSagaStep(context.Background(), nil, "svc", "/action", "/compensate", nil)
	if err == nil || !strings.Contains(err.Error(), "saga") {
		t.Fatalf("saga 为 nil 应返回可读错误，实际: %v", err)
	}
}

// TestAddSagaStepRejectsNilConsul 验证 consul 客户端为空时返回错误而非 panic
func TestAddSagaStepRejectsNilConsul(t *testing.T) {
	client := NewClient(nil, "http://127.0.0.1:36789")
	saga := dtmcli.NewSaga("http://127.0.0.1:36789", "g-1")
	err := client.AddSagaStep(context.Background(), saga, "svc", "/action", "/compensate", nil)
	if err == nil || !strings.Contains(err.Error(), "consul") {
		t.Fatalf("consul 为 nil 应返回可读错误，实际: %v", err)
	}
}

// TestAddSagaStepDisabled 验证未配置 DTM 时返回 ErrNotConfigured
func TestAddSagaStepDisabled(t *testing.T) {
	client := NewClient(nil, "")
	saga := dtmcli.NewSaga("http://127.0.0.1:36789", "g-1")
	err := client.AddSagaStep(context.Background(), saga, "svc", "/action", "/compensate", nil)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("未启用时应返回 ErrNotConfigured，实际: %v", err)
	}
}

// TestGenGidPreservesContextError 验证调用方超时同时保留哨兵与 context 错误身份
func TestGenGidPreservesContextError(t *testing.T) {
	srv := newBlockingGidServer(t)
	defer srv.Close()

	client := NewClient(nil, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.NewSaga(ctx)
	if !errors.Is(err, ErrGenGidFailed) {
		t.Fatalf("应保留 ErrGenGidFailed 身份，实际: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("应保留 context.DeadlineExceeded 身份，实际: %v", err)
	}
}

// TestGenGidPreservesJSONError 验证响应解析失败时保留底层 json 错误
func TestGenGidPreservesJSONError(t *testing.T) {
	srv := newJSONGidServer(t, http.StatusOK, `not-json`)
	defer srv.Close()

	_, err := NewClient(nil, srv.URL).NewSaga(context.Background())
	if !errors.Is(err, ErrGenGidFailed) {
		t.Fatalf("应保留 ErrGenGidFailed 身份，实际: %v", err)
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("应保留 json.SyntaxError，实际: %v", err)
	}
}

// TestAppendSagaStepRejectsUnmarshalablePayload 验证 payload 无法序列化时返回错误而非 panic
func TestAppendSagaStepRejectsUnmarshalablePayload(t *testing.T) {
	saga := dtmcli.NewSaga("http://127.0.0.1:36789", "g-1")
	err := appendSagaStep(saga, "http://127.0.0.1:8080/a", "http://127.0.0.1:8080/c", make(chan int))
	if err == nil {
		t.Fatal("不可序列化的 payload 应返回错误")
	}
	if len(saga.Steps) != 0 || len(saga.Payloads) != 0 {
		t.Fatalf("序列化失败不应追加步骤，steps=%d payloads=%d", len(saga.Steps), len(saga.Payloads))
	}
}

// TestAppendSagaStepPayload 验证正常 payload 序列化后写入 Saga
func TestAppendSagaStepPayload(t *testing.T) {
	saga := dtmcli.NewSaga("http://127.0.0.1:36789", "g-1")
	err := appendSagaStep(saga, "http://127.0.0.1:8080/a", "http://127.0.0.1:8080/c", map[string]string{"name": "x"})
	if err != nil {
		t.Fatalf("追加分支失败: %v", err)
	}
	if len(saga.Steps) != 1 || len(saga.Payloads) != 1 {
		t.Fatalf("应追加 1 个分支，steps=%d payloads=%d", len(saga.Steps), len(saga.Payloads))
	}
	if saga.Payloads[0] != `{"name":"x"}` {
		t.Fatalf("payload 期望 {\"name\":\"x\"}，实际 %s", saga.Payloads[0])
	}
}

// TestValidateHTTPURL 验证地址只接受带 http/https 与 host 的绝对地址
func TestValidateHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "http", raw: "http://10.0.0.1:8080/internal/v1/resource", wantErr: false},
		{name: "https", raw: "https://10.0.0.1/internal/v1/resource", wantErr: false},
		{name: "无协议", raw: "10.0.0.1:8080/internal/v1/resource", wantErr: true},
		{name: "不支持的协议", raw: "ftp://10.0.0.1/resource", wantErr: true},
		{name: "缺少 host", raw: "http:///internal/v1/resource", wantErr: true},
		{name: "相对路径", raw: "/internal/v1/resource", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHTTPURL("action", tc.raw)
			if tc.wantErr && err == nil {
				t.Fatalf("期望拒绝 %q，实际通过", tc.raw)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望接受 %q，实际: %v", tc.raw, err)
			}
		})
	}
}
