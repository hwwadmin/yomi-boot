package dtm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	saga, err := NewSaga(ctx, srv.URL)
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

	start := time.Now()
	saga, err := NewSagaWithTimeout(context.Background(), srv.URL, 50*time.Millisecond)
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

	saga, err := NewSagaWithTimeout(context.Background(), srv.URL, 7*time.Second)
	if err != nil {
		t.Fatalf("创建 Saga 失败: %v", err)
	}
	if saga.RequestTimeout != 7 {
		t.Fatalf("RequestTimeout 期望 7，实际 %d", saga.RequestTimeout)
	}
}

// TestNewSagaDefaultRequestTimeout 验证默认超时会落到 Saga 上
func TestNewSagaDefaultRequestTimeout(t *testing.T) {
	srv := newJSONGidServer(t, http.StatusOK, `{"gid":"g-1"}`)
	defer srv.Close()

	saga, err := NewSaga(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("创建 Saga 失败: %v", err)
	}
	want := int64(DefaultRequestTimeout / time.Second)
	if saga.RequestTimeout != want {
		t.Fatalf("RequestTimeout 期望 %d，实际 %d", want, saga.RequestTimeout)
	}
}

// TestNewSagaRoundsSubSecondTimeoutUp 验证不足 1 秒的配置不会退化成“不超时”
func TestNewSagaRoundsSubSecondTimeoutUp(t *testing.T) {
	srv := newJSONGidServer(t, http.StatusOK, `{"gid":"g-1"}`)
	defer srv.Close()

	saga, err := NewSagaWithTimeout(context.Background(), srv.URL, 500*time.Millisecond)
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

	saga, err := NewSaga(context.Background(), srv.URL)
	if err == nil || saga != nil {
		t.Fatalf("非 200 响应应返回错误，saga=%v err=%v", saga, err)
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("错误信息应包含状态码，实际: %v", err)
	}
}
