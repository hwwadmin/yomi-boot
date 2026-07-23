// Package testclient 单元测试
//
// @author zwei
package testclient

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGet_Success 测试 GET 请求成功时返回响应体且 ok 为 true
func TestGet_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method: got %s, want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization: got %q, want %q", got, "Bearer test-token")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	body, ok := Get(server.URL+"/health", "test-token")
	if !ok {
		t.Fatalf("ok: got false, want true, body=%s", body)
	}
	if body != `{"code":"OK"}` {
		t.Fatalf("body: got %q, want %q", body, `{"code":"OK"}`)
	}
}

// TestGet_NonOKStatus 测试 HTTP 状态码非 200 时 ok 为 false 但仍返回响应体
func TestGet_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"BAD_REQUEST"}`))
	}))
	defer server.Close()

	body, ok := Get(server.URL, "")
	if ok {
		t.Fatal("ok: got true, want false")
	}
	if body != `{"code":"BAD_REQUEST"}` {
		t.Fatalf("body: got %q", body)
	}
}

// TestPost_JSONBody 测试 POST 请求自动序列化 JSON 并设置 Content-Type
func TestPost_JSONBody(t *testing.T) {
	type loginReq struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: got %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("content-type: got %q, want application/json", ct)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var req loginReq
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if req.Username != "admin" || req.Password != "123456" {
			t.Fatalf("body: got %+v", req)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":"OK","data":{"token":"abc"}}`))
	}))
	defer server.Close()

	body, ok := Post(server.URL+"/login", "", loginReq{
		Username: "admin",
		Password: "123456",
	})
	if !ok {
		t.Fatalf("ok: got false, want true, body=%s", body)
	}
	if !strings.Contains(body, `"token":"abc"`) {
		t.Fatalf("body: got %q", body)
	}
}

// TestDelete_Success 测试 DELETE 请求成功，且支持传入完整 Bearer Token
func TestDelete_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method: got %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer server.Close()

	body, ok := Delete(server.URL+"/roles/1", "Bearer raw-token")
	if !ok {
		t.Fatalf("ok: got false, want true, body=%s", body)
	}
	if body != `{"code":"OK"}` {
		t.Fatalf("body: got %q", body)
	}
}

// TestClient_EmptyURL 测试请求地址为空时返回错误信息且 ok 为 false
func TestClient_EmptyURL(t *testing.T) {
	body, ok := New().Post("  ", "token", map[string]string{"k": "v"})
	if ok {
		t.Fatal("ok: got true, want false")
	}
	if !strings.Contains(body, "请求地址不能为空") {
		t.Fatalf("body: got %q", body)
	}
}
