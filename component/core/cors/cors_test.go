package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func testCORSConfig() Config {
	return Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST"},
		AllowHeaders: []string{"Content-Type"},
	}
}

func newCORSEngine(cfg Config, hit *bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(cfg))
	r.Any("/x", func(c *gin.Context) {
		if hit != nil {
			*hit = true
		}
		c.Status(http.StatusOK)
	})
	return r
}

func TestMiddleware_PreflightOptionsReturns204WithCORSHeaders(t *testing.T) {
	hit := false
	r := newCORSEngine(testCORSConfig(), &hit)

	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("预检状态 = %d，期望 204", w.Code)
	}
	if hit {
		t.Fatal("预检不应进入后续处理")
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST" {
		t.Fatalf("Allow-Methods = %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type" {
		t.Fatalf("Allow-Headers = %q", got)
	}
}

func TestMiddleware_PlainOptionsReachesHandler(t *testing.T) {
	hit := false
	r := newCORSEngine(testCORSConfig(), &hit)

	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !hit {
		t.Fatal("普通 OPTIONS 应进入后续处理")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("普通 OPTIONS 状态 = %d，期望后续处理的 200", w.Code)
	}
}

func TestMiddleware_OptionsWithoutOriginReachesHandler(t *testing.T) {
	hit := false
	r := newCORSEngine(testCORSConfig(), &hit)

	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !hit || w.Code != http.StatusOK {
		t.Fatalf("没有 Origin 的 OPTIONS 不应当作预检: hit=%v code=%d", hit, w.Code)
	}
}
