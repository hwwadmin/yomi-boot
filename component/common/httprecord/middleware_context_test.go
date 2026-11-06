// HTTP 访问记录中间件上下文传递的回归测试
//
// @author zwei
package httprecord

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/appctx"
	"github.com/hwwadmin/yomi-boot/component/core/httpguard"
	"github.com/hwwadmin/yomi-boot/component/core/router"
	"github.com/hwwadmin/yomi-boot/component/core/traceid"

	"github.com/gin-gonic/gin"
)

// ctxCaptureService 记录投递时收到的上下文状态
type ctxCaptureService struct {
	mu      sync.Mutex
	ctxErr  error
	traceID string
}

// Handle 记录 ctx 的取消状态与链路 ID
func (s *ctxCaptureService) Handle(ctx context.Context, _ *HttpRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctxErr = ctx.Err()
	s.traceID = traceid.FromContext(ctx)
}

// TestMiddlewareHandleContextAlive 验证后置采集投递使用未被后续中间件取消的请求上下文
func TestMiddlewareHandleContextAlive(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &ctxCaptureService{}
	app := appctx.NewAppCtx("svc", "dev", "v1")
	engine := gin.New()
	engine.Use(traceid.Middleware())
	engine.Use(Middleware(svc, Config{MiddlewareEnabled: true}, app, nil))
	engine.Use(httpguard.Middleware(router.Config{Port: 8080, ReadTimeoutSeconds: 10, WriteTimeoutSeconds: 15}))
	engine.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.ctxErr != nil {
		t.Fatalf("Handle 收到的 ctx 已结束: %v", svc.ctxErr)
	}
	if svc.traceID == "" {
		t.Fatal("Handle 收到的 ctx 缺少 traceparent")
	}
}
