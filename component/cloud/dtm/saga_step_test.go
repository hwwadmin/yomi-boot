package dtm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/cloud/consul"
	"gitee.com/hq_project/yomi-boot/component/core/errors"

	"github.com/dtm-labs/client/dtmcli"
)

// newFakeConsulAgent 返回伪装 Consul Agent 健康查询的 httptest 服务，固定返回 entriesJSON
func newFakeConsulAgent(t *testing.T, entriesJSON string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Consul-Index", "1")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(entriesJSON))
	}))
}

// newResolveClient 构造带 consul 客户端且解析结果带 http 协议的 DTM 客户端
func newResolveClient(t *testing.T, agentURL string, discoveryURLScheme string) *Client {
	t.Helper()
	cfg := consul.Config{
		Addr:               agentURL,
		ServiceID:          "dtm-test",
		ServiceAddress:     "127.0.0.1:18080",
		CheckAddress:       "http://127.0.0.1:18080/health",
		DiscoveryURLScheme: discoveryURLScheme,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("consul 配置非法: %v", err)
	}
	consulClient, err := consul.NewClient(cfg)
	if err != nil {
		t.Fatalf("创建 consul 客户端失败: %v", err)
	}
	return NewClient(consulClient, "http://127.0.0.1:36789")
}

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

// TestAddSagaStepResolvesAndAppends 验证通过 consul 解析健康实例后成功追加分支
func TestAddSagaStepResolvesAndAppends(t *testing.T) {
	// Service.Address 为空时回退 Node.Address
	agent := newFakeConsulAgent(t,
		`[{"Node":{"Address":"10.0.0.1"},"Service":{"Address":"","Port":8080}}]`)
	defer agent.Close()

	client := newResolveClient(t, agent.URL, "http://")
	saga := dtmcli.NewSaga("http://127.0.0.1:36789", "g-1")
	err := client.AddSagaStep(context.Background(), saga, "svc", "/action", "/compensate",
		map[string]string{"name": "x"})
	if err != nil {
		t.Fatalf("AddSagaStep 失败: %v", err)
	}
	if len(saga.Steps) != 1 || len(saga.Payloads) != 1 {
		t.Fatalf("应追加 1 个分支，steps=%d payloads=%d", len(saga.Steps), len(saga.Payloads))
	}
	if got := saga.Steps[0]["action"]; got != "http://10.0.0.1:8080/action" {
		t.Fatalf("action 地址期望 http://10.0.0.1:8080/action，实际 %s", got)
	}
	if got := saga.Steps[0]["compensate"]; got != "http://10.0.0.1:8080/compensate" {
		t.Fatalf("compensate 地址期望 http://10.0.0.1:8080/compensate，实际 %s", got)
	}
}

// TestAddSagaStepRejectsUnresolvableService 验证服务无健康实例时返回错误且不追加分支
func TestAddSagaStepRejectsUnresolvableService(t *testing.T) {
	agent := newFakeConsulAgent(t, `[]`)
	defer agent.Close()

	client := newResolveClient(t, agent.URL, "http://")
	saga := dtmcli.NewSaga("http://127.0.0.1:36789", "g-1")
	err := client.AddSagaStep(context.Background(), saga, "svc", "/action", "/compensate", nil)
	if err == nil {
		t.Fatal("无健康实例应返回错误")
	}
	if len(saga.Steps) != 0 || len(saga.Payloads) != 0 {
		t.Fatalf("解析失败不应追加分支，steps=%d payloads=%d", len(saga.Steps), len(saga.Payloads))
	}
}

// TestAddSagaStepRejectsSchemelessResolve 验证未配置协议前缀时解析出的地址被拒绝
func TestAddSagaStepRejectsSchemelessResolve(t *testing.T) {
	agent := newFakeConsulAgent(t,
		`[{"Node":{"Address":"10.0.0.1"},"Service":{"Address":"","Port":8080}}]`)
	defer agent.Close()

	client := newResolveClient(t, agent.URL, "")
	saga := dtmcli.NewSaga("http://127.0.0.1:36789", "g-1")
	err := client.AddSagaStep(context.Background(), saga, "svc", "/action", "/compensate", nil)
	if err == nil || !strings.Contains(err.Error(), "action") {
		t.Fatalf("无协议地址应被 action 校验拒绝，实际: %v", err)
	}
	if len(saga.Steps) != 0 || len(saga.Payloads) != 0 {
		t.Fatalf("校验失败不应追加分支，steps=%d payloads=%d", len(saga.Steps), len(saga.Payloads))
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

// TestGenGidEmptyGid 验证服务端返回空 gid 时返回可读错误
func TestGenGidEmptyGid(t *testing.T) {
	srv := newJSONGidServer(t, http.StatusOK, `{"gid":""}`)
	defer srv.Close()

	_, err := NewClient(nil, srv.URL).NewSaga(context.Background())
	if err == nil || !strings.Contains(err.Error(), "empty gid") {
		t.Fatalf("空 gid 应返回可读错误，实际: %v", err)
	}
	if !errors.Is(err, ErrGenGidFailed) {
		t.Fatalf("应保留 ErrGenGidFailed 身份，实际: %v", err)
	}
}

// TestNewSagaNilContext 验证 ctx 为 nil 时退回 Background 不报错
func TestNewSagaNilContext(t *testing.T) {
	srv := newJSONGidServer(t, http.StatusOK, `{"gid":"g-nil"}`)
	defer srv.Close()

	saga, err := NewClient(nil, srv.URL).NewSaga(nil)
	if err != nil || saga == nil {
		t.Fatalf("nil ctx 应正常创建 Saga，saga=%v err=%v", saga, err)
	}
}

// TestTruncateBody 验证错误响应体的截断边界
func TestTruncateBody(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "短文本原样", in: "short", want: "short"},
		{name: "去除首尾空白", in: "  padded  ", want: "padded"},
		{name: "恰好上限不截断", in: strings.Repeat("汉", maxErrorBodyRunes), want: strings.Repeat("汉", maxErrorBodyRunes)},
		{name: "超限按 rune 截断", in: strings.Repeat("汉", maxErrorBodyRunes+50),
			want: strings.Repeat("汉", maxErrorBodyRunes) + "..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := truncateBody(tc.in); got != tc.want {
				t.Fatalf("truncateBody 长度期望 %d，实际 %d", len([]rune(tc.want)), len([]rune(got)))
			}
		})
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
