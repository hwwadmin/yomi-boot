package token_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/response"
	"gitee.com/hq_project/yomi-boot/component/common/token"

	"github.com/gin-gonic/gin"
)

// TestAuthMiddleware_DisabledPassesThrough 验证中间件关闭时直接放行
func TestAuthMiddleware_DisabledPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := token.Config{MiddlewareEnabled: false}

	router := gin.New()
	router.Use(token.AuthMiddleware(nil, cfg))
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("中间件关闭时应直接放行, got=%d", w.Code)
	}
}

// TestAuthMiddleware_RequiresToken 验证启用时无 Token 拒绝请求
func TestAuthMiddleware_RequiresToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response.RegisterDefaultForTest(t)

	cfg := token.Config{
		MiddlewareEnabled: true,
		TokenTTLMinutes:   60,
		StorePrefix:       "test",
	}

	router := gin.New()
	router.Use(token.AuthMiddleware(nil, cfg))
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusNoContent {
		t.Fatal("中间件启用且无 Token 时不应放行")
	}
}
