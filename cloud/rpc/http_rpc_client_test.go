package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const maxInt64JSON = 9223372036854775807

// TestDefaultRequestTimeout 验证默认请求超时为 3 秒
func TestDefaultRequestTimeout(t *testing.T) {
	if DefaultRequestTimeout != 3*time.Second {
		t.Fatalf("DefaultRequestTimeout: got %v want 3s", DefaultRequestTimeout)
	}
}

// TestInvoke_PreservesInt64Precision 验证大整数经 RPC 响应解析后不丢失精度
func TestInvoke_PreservesInt64Precision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK","msg":"","data":{"id":9223372036854775807}}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL)
	var env testEnvelope
	if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var resp struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &resp); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if resp.ID != maxInt64JSON {
		t.Fatalf("id: got %d, want %d", resp.ID, maxInt64JSON)
	}
}

// TestInvoke_PreservesInt64InPageResponse 验证分页响应中 total 与 data 的大整数均不丢失精度
func TestInvoke_PreservesInt64InPageResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK","msg":"","total":9223372036854775807,"data":[{"id":9223372036854775807}]}`))
	}))
	defer server.Close()

	client := newTestRPCClient(server.URL)
	var env testEnvelope
	if err := invokeTest(client, context.Background(), "/x", nil, &env, checkTestEnvelope); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if env.Total != maxInt64JSON {
		t.Fatalf("total: got %d, want %d", env.Total, maxInt64JSON)
	}

	var items []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &items); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(items) != 1 || items[0].ID != maxInt64JSON {
		t.Fatalf("items: got %+v, want one item with id %d", items, maxInt64JSON)
	}
}
