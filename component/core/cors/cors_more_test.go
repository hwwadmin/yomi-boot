package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidate_RejectsBadConfig(t *testing.T) {
	t.Parallel()

	if err := (Config{MaxAge: -1}).Validate(); err == nil {
		t.Fatal("负数 MaxAge 应失败")
	}
	if err := (Config{AllowOrigins: []string{"*", "https://a.example"}}).Validate(); err == nil {
		t.Fatal("* 与其他来源并存应失败")
	}
	if err := (Config{AllowOrigins: []string{"https://a.example"}}).Validate(); err != nil {
		t.Fatalf("单一来源应通过: %v", err)
	}
}

func TestMiddleware_DisabledAndOriginBranches(t *testing.T) {
	disabled := false
	hit := false
	r := newCORSEngine(Config{MiddlewareEnabled: &disabled}, &hit)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if !hit {
		t.Fatal("关闭中间件时应继续后续处理")
	}

	cred := Config{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
		AllowMethods:     []string{"GET"},
		AllowHeaders:     []string{"X-Request"},
	}
	r = newCORSEngine(cred, nil)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://app.example")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("凭证通配应回显 Origin，实际 %q", got)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("应允许凭证")
	}

	specific := Config{AllowOrigins: []string{"https://ok.example"}, MaxAge: 12}
	r = newCORSEngine(specific, nil)
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://ok.example")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://ok.example" {
		t.Fatalf("允许来源应回显，实际 %q", got)
	}
	if w.Header().Get("Access-Control-Max-Age") != "12" {
		t.Fatalf("MaxAge = %q", w.Header().Get("Access-Control-Max-Age"))
	}

	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://no.example")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("未允许来源不应回显，实际 %q", got)
	}

	if isPreflight(nil) {
		t.Fatal("nil 请求不是预检")
	}
	if isOriginAllowed("", []string{"https://ok.example"}) {
		t.Fatal("空 Origin 不应允许")
	}
	if isOriginAllowed("https://ok.example", []string{"https://other.example"}) {
		t.Fatal("不在列表中的 Origin 不应允许")
	}
}
