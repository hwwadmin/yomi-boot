// 令牌鉴权中间件的端到端补充测试：真实令牌服务 + 内存 Redis
//
// @author zwei
package token_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	yomiredis "gitee.com/hq_project/yomi-boot/component/common/redis"
	"gitee.com/hq_project/yomi-boot/component/common/redis/redistest"
	"gitee.com/hq_project/yomi-boot/component/common/token"
	"gitee.com/hq_project/yomi-boot/component/core/response"
)

// newE2ETokenService 构建基于内存 Redis 的真实令牌服务（多端登录模式，无需分布式锁）
func newE2ETokenService(t *testing.T) token.TokenService {
	t.Helper()

	client, cleanup := redistest.NewClient(t)
	t.Cleanup(cleanup)
	store := token.NewRedisStore(yomiredis.NewRedisHelper(client))
	return token.NewService(token.NewUUIDGenerator(), store, token.Config{
		TokenTTLMinutes: 30,
		AllowMultiLogin: true,
		StorePrefix:     "yomi:test:token:e2e",
	}, nil)
}

// newAuthRouter 构建带鉴权中间件的测试路由，业务 handler 透出会话信息
func newAuthRouter(t *testing.T, svc token.TokenService, cfg token.Config) (*gin.Engine, *int64) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	response.RegisterDefaultForTest(t)

	var operatorID int64
	router := gin.New()
	router.Use(token.Middleware(svc, cfg))
	router.GET("/ping", func(c *gin.Context) {
		if session, ok := token.SessionFromContext(c.Request.Context()); ok && session != nil {
			operatorID = session.UserID
		}
		if tokenValue, ok := token.TokenValueFromContext(c.Request.Context()); ok && tokenValue == "" {
			t.Error("上下文中的令牌值不应为空")
		}
		c.Status(http.StatusNoContent)
	})
	return router, &operatorID
}

// TestMiddlewareValidateSuccess 验证携带有效令牌时请求放行且会话注入上下文
func TestMiddlewareValidateSuccess(t *testing.T) {
	svc := newE2ETokenService(t)

	issued, err := svc.Issue(context.Background(), &token.TokenSession{UserID: 8101})
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}

	cfg := token.Config{MiddlewareEnabled: true, TokenTTLMinutes: 30, StorePrefix: "yomi:test:token:e2e"}
	router, operatorID := newAuthRouter(t, svc, cfg)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Authorization", "Bearer "+issued)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("有效令牌应放行，实际 %d, body=%s", w.Code, w.Body.String())
	}
	if *operatorID != 8101 {
		t.Fatalf("业务 handler 应能读取会话用户 ID，实际 %d", *operatorID)
	}
}

// TestMiddlewareValidateFailure 验证无效令牌被拒绝且返回统一业务错误码
func TestMiddlewareValidateFailure(t *testing.T) {
	svc := newE2ETokenService(t)
	cfg := token.Config{MiddlewareEnabled: true, TokenTTLMinutes: 30, StorePrefix: "yomi:test:token:e2e"}
	router, _ := newAuthRouter(t, svc, cfg)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "TOKEN_VALIDATE_FAILED") {
		t.Fatalf("无效令牌应返回 TOKEN_VALIDATE_FAILED，实际 %s", w.Body.String())
	}
}

// TestMiddlewareSkipsValidatePath 验证跳过校验路径无需令牌即可访问
func TestMiddlewareSkipsValidatePath(t *testing.T) {
	svc := newE2ETokenService(t)
	cfg := token.Config{
		MiddlewareEnabled: true,
		TokenTTLMinutes:   30,
		StorePrefix:       "yomi:test:token:e2e",
		SkipValidatePaths: []string{"/public"},
	}
	router, _ := newAuthRouter(t, svc, cfg)
	router.GET("/public", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/public", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("跳过路径应放行，实际 %d, body=%s", w.Code, w.Body.String())
	}
}
