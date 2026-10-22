package token_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gitee.com/hq_project/yomi-boot/component/common/response"
	"gitee.com/hq_project/yomi-boot/component/common/token"

	"github.com/gin-gonic/gin"
)

// TestAuthMiddleware_DisabledPassesThrough 验证 auth.enabled=false 时中间件直接放行
func TestAuthMiddleware_DisabledPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := token.Config{Enabled: false}

	router := gin.New()
	router.Use(token.AuthMiddleware(nil, cfg))
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("auth.enabled=false 时应直接放行, got=%d", w.Code)
	}
}

// TestAuthMiddleware_EnabledRequiresToken 验证 auth.enabled=true 且无 Token 时拒绝请求
func TestAuthMiddleware_EnabledRequiresToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response.RegisterDefaultForTest(t)

	cfg := token.Config{
		Enabled:         true,
		TokenTTLMinutes: 60,
		StorePrefix:     "test",
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
		t.Fatal("auth.enabled=true 且无 Token 时不应放行")
	}
}
