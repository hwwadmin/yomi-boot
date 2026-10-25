// 内存 S3 兼容 fake 服务，供三个 Storage 后端的集成测试使用
//
// @author zwei
package oss

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3Object fake 中的对象内容与元数据
type fakeS3Object struct {
	data         []byte
	contentType  string
	lastModified time.Time
}

// fakeS3 最小 S3 兼容服务：支持 PUT/GET/HEAD/DELETE 的对象读写，
// fail 置位后写入类操作返回 500，用于错误路径注入
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string]fakeS3Object
	fail    bool
}

// newFakeS3 启动 fake S3 服务并返回实例与地址
func newFakeS3(t *testing.T) (*fakeS3, string) {
	t.Helper()

	f := &fakeS3{objects: make(map[string]fakeS3Object)}
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	return f, server.URL
}

// get 读取对象（测试断言用）
func (f *fakeS3) get(key string) (fakeS3Object, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	obj, ok := f.objects[key]
	return obj, ok
}

// ServeHTTP 处理 S3 风格请求（path-style：/{bucket}/{key}）
func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/")

	switch r.Method {
	case http.MethodPut:
		if f.fail {
			writeS3Error(w, http.StatusInternalServerError, "AccessDenied", "fail injected")
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeS3Error(w, http.StatusBadRequest, "InvalidRequest", err.Error())
			return
		}
		f.mu.Lock()
		f.objects[key] = fakeS3Object{
			data:         body,
			contentType:  r.Header.Get("Content-Type"),
			lastModified: time.Now(),
		}
		f.mu.Unlock()
		w.Header().Set("ETag", `"etag-`+key+`"`)
		w.WriteHeader(http.StatusOK)

	case http.MethodGet:
		f.mu.Lock()
		obj, ok := f.objects[key]
		f.mu.Unlock()
		if !ok {
			writeS3Error(w, http.StatusNotFound, "NoSuchKey", "object not found")
			return
		}
		w.Header().Set("Content-Type", obj.contentType)
		w.Header().Set("ETag", `"etag-`+key+`"`)
		w.Header().Set("Last-Modified", obj.lastModified.UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(obj.data)

	case http.MethodHead:
		f.mu.Lock()
		obj, ok := f.objects[key]
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", obj.contentType)
		w.Header().Set("ETag", `"etag-`+key+`"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(obj.data)))
		w.Header().Set("Last-Modified", obj.lastModified.UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)

	case http.MethodDelete:
		if f.fail {
			writeS3Error(w, http.StatusInternalServerError, "AccessDenied", "fail injected")
			return
		}
		f.mu.Lock()
		delete(f.objects, key)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	default:
		writeS3Error(w, http.StatusBadRequest, "NotImplemented", "unsupported method")
	}
}

// writeS3Error 输出 S3 风格错误 XML
func writeS3Error(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w,
		`<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message><RequestId>req-1</RequestId><HostId>host-1</HostId></Error>`,
		code, message)
}

// newTestRustFSStorage 基于指向 fake 的 RustFS 配置创建 Storage
func newTestRustFSStorage(t *testing.T, endpoint string) Storage {
	t.Helper()
	store, err := New(Config{
		Provider:      ProviderRustFS,
		PublicBaseURL: "https://cdn.example.com",
		RustFS: RustFSConfig{
			Endpoint:        endpoint,
			AccessKeyID:     "test-ak",
			SecretAccessKey: "test-sk",
		},
	})
	if err != nil {
		t.Fatalf("创建 RustFS 存储失败: %v", err)
	}
	return store
}

// newTestS3Storage 基于指向 fake 的 S3 配置创建 Storage
func newTestS3Storage(t *testing.T, endpoint string) Storage {
	t.Helper()
	store, err := New(Config{
		Provider:      ProviderS3,
		PublicBaseURL: "https://cdn.example.com",
		S3: S3Config{
			Region:          "us-east-1",
			AccessKeyID:     "test-ak",
			SecretAccessKey: "test-sk",
			Endpoint:        endpoint,
			UsePathStyle:    true,
		},
	})
	if err != nil {
		t.Fatalf("创建 S3 存储失败: %v", err)
	}
	return store
}

// newTestAliStorage 基于指向 fake 的 AliOSS 配置创建 Storage
func newTestAliStorage(t *testing.T, endpoint string) Storage {
	t.Helper()
	store, err := New(Config{
		Provider:      ProviderAliOSS,
		PublicBaseURL: "https://cdn.example.com",
		AliOSS: AliOSSConfig{
			Region:          "cn-hangzhou",
			AccessKeyID:     "test-ak",
			SecretAccessKey: "test-sk",
			Endpoint:        endpoint,
			UsePathStyle:    true,
		},
	})
	if err != nil {
		t.Fatalf("创建 AliOSS 存储失败: %v", err)
	}
	return store
}
