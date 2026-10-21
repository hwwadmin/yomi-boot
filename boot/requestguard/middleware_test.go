package requestguard

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/handler"
	"gitee.com/hq_project/yomi-boot/boot/response"
	"gitee.com/hq_project/yomi-boot/boot/router"

	"github.com/gin-gonic/gin"
)

// TestMiddleware_RequestTimeout 验证写超时后返回 REQUEST_TIMEOUT
func TestMiddleware_RequestTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response.RegisterDefaultForTest(t)

	engine := gin.New()
	engine.Use(Middleware(router.Config{WriteTimeoutSeconds: 3}))
	engine.GET("/slow", func(c *gin.Context) {
		select {
		case <-c.Request.Context().Done():
			return
		case <-time.After(2 * time.Second):
			c.Status(http.StatusOK)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200，实际 %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "REQUEST_TIMEOUT") {
		t.Fatalf("期望返回 REQUEST_TIMEOUT，实际 body=%s", rec.Body.String())
	}
}

// TestMiddleware_MaxBodyBytes 验证请求体超限返回 REQUEST_BODY_TOO_LARGE
func TestMiddleware_MaxBodyBytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response.RegisterDefaultForTest(t)

	engine := gin.New()
	engine.Use(Middleware(router.Config{
		WriteTimeoutSeconds: 30,
		MaxRequestBodyBytes: 8,
	}))
	engine.POST("/echo", func(c *gin.Context) {
		handler.BindAndHandle(c, func(ctx context.Context, input *struct {
			Data string `json:"data"`
		},
		) (struct{}, error) {
			return struct{}{}, nil
		})
	})

	body := bytes.NewBufferString(`{"data":"0123456789"}`)
	req := httptest.NewRequest(http.MethodPost, "/echo", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200，实际 %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "REQUEST_BODY_TOO_LARGE") {
		t.Fatalf("期望返回 REQUEST_BODY_TOO_LARGE，实际 body=%s", rec.Body.String())
	}
}

// TestMiddleware_AllowsSmallBody 验证小请求体正常通过
func TestMiddleware_AllowsSmallBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response.RegisterDefaultForTest(t)

	engine := gin.New()
	engine.Use(Middleware(router.Config{
		WriteTimeoutSeconds: 30,
		MaxRequestBodyBytes: 64,
	}))
	engine.POST("/echo", func(c *gin.Context) {
		handler.BindAndHandle(c, func(ctx context.Context, input *struct {
			Data string `json:"data"`
		}) (struct {
			Data string `json:"data"`
		}, error,
		) {
			return struct {
				Data string `json:"data"`
			}{Data: input.Data}, nil
		})
	})

	body := bytes.NewBufferString(`{"data":"ok"}`)
	req := httptest.NewRequest(http.MethodPost, "/echo", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200，实际 %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"OK"`) {
		t.Fatalf("期望成功响应，实际 body=%s", rec.Body.String())
	}
}

// TestMiddleware_InjectsDeadline 验证中间件注入请求 deadline
func TestMiddleware_InjectsDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotDeadline time.Time
	engine := gin.New()
	engine.Use(Middleware(router.Config{WriteTimeoutSeconds: 5}))
	engine.GET("/deadline", func(c *gin.Context) {
		deadline, ok := c.Request.Context().Deadline()
		if !ok {
			t.Error("期望 context 带 deadline")
			return
		}
		gotDeadline = deadline
	})

	req := httptest.NewRequest(http.MethodGet, "/deadline", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200，实际 %d", rec.Code)
	}
	remaining := time.Until(gotDeadline)
	if remaining <= 0 || remaining > 3*time.Second {
		t.Fatalf("deadline 剩余时间异常: %v", remaining)
	}
}

// TestMiddleware_UsesDefaultBodyLimit 验证默认请求体大小限制生效
func TestMiddleware_UsesDefaultBodyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response.RegisterDefaultForTest(t)

	engine := gin.New()
	engine.Use(Middleware(router.Config{WriteTimeoutSeconds: 30}))
	engine.POST("/echo", func(c *gin.Context) {
		handler.BindAndHandle(c, func(ctx context.Context, input *struct {
			Data string `json:"data"`
		},
		) (struct{}, error) {
			return struct{}{}, nil
		})
	})

	limit := router.Config{}.MaxRequestBodyBytesOrDefault()
	body := bytes.NewBufferString(`{"data":"` + strings.Repeat("a", int(limit)+16) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/echo", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "REQUEST_BODY_TOO_LARGE") {
		t.Fatalf("期望默认 body 限制生效，实际 body=%s", rec.Body.String())
	}
}
