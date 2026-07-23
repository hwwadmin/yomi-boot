// Package testclient 单元测试
//
// @author zwei
package testclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBuildQuery_Struct 测试 struct 按 json 标签生成查询参数并跳过零值字段
func TestBuildQuery_Struct(t *testing.T) {
	type queryReq struct {
		Page     int    `json:"page"`
		PageSize int    `json:"pageSize"`
		Keyword  string `json:"keyword"`
		Status   int    `json:"status"`
	}

	query, err := BuildQuery(queryReq{
		Page:     1,
		PageSize: 10,
		Keyword:  "admin",
	})
	if err != nil {
		t.Fatalf("BuildQuery: %v", err)
	}

	if !strings.Contains(query, "page=1") {
		t.Fatalf("query: got %q, want contain page=1", query)
	}
	if !strings.Contains(query, "pageSize=10") {
		t.Fatalf("query: got %q, want contain pageSize=10", query)
	}
	if !strings.Contains(query, "keyword=admin") {
		t.Fatalf("query: got %q, want contain keyword=admin", query)
	}
	if strings.Contains(query, "status=") {
		t.Fatalf("query: got %q, zero value field should be skipped", query)
	}
}

// TestBuildQuery_Map 测试 map[string]any 生成查询参数
func TestBuildQuery_Map(t *testing.T) {
	query, err := BuildQuery(map[string]any{
		"roleId": 1001,
		"name":   "管理员",
	})
	if err != nil {
		t.Fatalf("BuildQuery: %v", err)
	}
	if !strings.Contains(query, "roleId=1001") {
		t.Fatalf("query: got %q", query)
	}
	if !strings.Contains(query, "name=") {
		t.Fatalf("query: got %q", query)
	}
}

// TestAppendQuery_WithExistingQuery 测试在已有查询参数的 URL 上追加参数
func TestAppendQuery_WithExistingQuery(t *testing.T) {
	fullURL, err := AppendQuery("http://127.0.0.1:8080/api/users?scope=admin", map[string]any{
		"page": 2,
	})
	if err != nil {
		t.Fatalf("AppendQuery: %v", err)
	}
	if !strings.Contains(fullURL, "scope=admin") {
		t.Fatalf("url: got %q, want keep existing query", fullURL)
	}
	if !strings.Contains(fullURL, "page=2") {
		t.Fatalf("url: got %q, want append new query", fullURL)
	}
}

// TestGetWithQuery_Success 测试将对象解析为查询参数后发起 GET 请求
func TestGetWithQuery_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method: got %s, want GET", r.Method)
		}
		if got := r.URL.Query().Get("page"); got != "1" {
			t.Fatalf("page: got %q, want 1", got)
		}
		if got := r.URL.Query().Get("pageSize"); got != "20" {
			t.Fatalf("pageSize: got %q, want 20", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	body, ok := GetWithQuery(server.URL+"/roles", "token", map[string]any{
		"page":     1,
		"pageSize": 20,
	})
	if !ok {
		t.Fatalf("ok: got false, want true, body=%s", body)
	}
	if body != `{"code":"OK"}` {
		t.Fatalf("body: got %q", body)
	}
}

// TestBuildQuery_NilEmbeddedPointer 测试嵌入 *Foo nil 字段时不 panic
func TestBuildQuery_NilEmbeddedPointer(t *testing.T) {
	type Foo struct {
		Name string `json:"name"`
	}
	type Outer struct {
		*Foo
		Page int `json:"page"`
	}

	query, err := BuildQuery(Outer{Page: 1})
	if err != nil {
		t.Fatalf("BuildQuery: %v", err)
	}
	if !strings.Contains(query, "page=1") {
		t.Fatalf("query: got %q, want contain page=1", query)
	}
	if strings.Contains(query, "name=") {
		t.Fatalf("query: got %q, nil embedded pointer fields should be skipped", query)
	}
}

// TestBuildQuery_UnsupportedType 测试不支持的对象类型返回错误
func TestBuildQuery_UnsupportedType(t *testing.T) {
	_, err := BuildQuery([]string{"a", "b"})
	if err == nil {
		t.Fatal("BuildQuery: got nil error, want unsupported type error")
	}
}
