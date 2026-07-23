package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const maxInt64JSON = 9223372036854775807

// TestDoPostInternal_PreservesInt64Precision 验证大整数经 RPC 响应解析后不丢失精度
func TestDoPostInternal_PreservesInt64Precision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK","msg":"","timestamp":1,"data":{"id":9223372036854775807}}`))
	}))
	defer server.Close()

	type dataResp struct {
		ID int64 `json:"id"`
	}

	var resp dataResp
	client := &HttpRpcClient{httpClient: &http.Client{Timeout: 5 * time.Second}}
	if _, err := client.doPostInternal(context.Background(), server.URL, nil, &resp); err != nil {
		t.Fatalf("doPostInternal: %v", err)
	}
	if resp.ID != maxInt64JSON {
		t.Fatalf("id: got %d, want %d", resp.ID, maxInt64JSON)
	}
}

// TestDoPostInternal_PreservesInt64InPageResponse 验证分页响应中 data 与 total 的大整数均不丢失精度
func TestDoPostInternal_PreservesInt64InPageResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"OK","msg":"","timestamp":1,"total":9223372036854775807,"data":[{"id":9223372036854775807}]}`))
	}))
	defer server.Close()

	type item struct {
		ID int64 `json:"id"`
	}

	var items []item
	client := &HttpRpcClient{httpClient: &http.Client{Timeout: 5 * time.Second}}
	total, err := client.doPostInternal(context.Background(), server.URL, nil, &items)
	if err != nil {
		t.Fatalf("doPostInternal: %v", err)
	}
	if total != maxInt64JSON {
		t.Fatalf("total: got %d, want %d", total, maxInt64JSON)
	}
	if len(items) != 1 || items[0].ID != maxInt64JSON {
		t.Fatalf("items: got %+v, want one item with id %d", items, maxInt64JSON)
	}
}
