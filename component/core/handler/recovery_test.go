package handler_test

import (
	"github.com/gin-gonic/gin"
	"github.com/hwwadmin/yomi-boot/component/core/handler"
	"github.com/hwwadmin/yomi-boot/component/core/response"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecovery_PanicUsesFailureEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response.RegisterDefaultForTest(t)
	discardRecoveryLog(t)

	engine := gin.New()
	engine.Use(handler.Recovery())
	engine.GET("/panic", func(c *gin.Context) {
		panic("secret-panic-detail")
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200，实际 %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"INTERNAL_ERROR"`) || !strings.Contains(body, `"msg":"INTERNAL_ERROR"`) || !strings.Contains(body, `"timestamp"`) {
		t.Fatalf("期望统一失败信封，实际 body=%s", body)
	}
	if strings.Contains(body, "secret-panic-detail") {
		t.Fatalf("panic 原文不应出现在响应中: %s", body)
	}
}

func TestRecovery_SkipsWhenResponseWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response.RegisterDefaultForTest(t)
	discardRecoveryLog(t)

	engine := gin.New()
	engine.Use(handler.Recovery())
	engine.GET("/partial", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": "OK", "msg": "success"})
		panic("secret-panic-detail")
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/partial", nil))

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 HTTP 200，实际 %d body=%s", rec.Code, body)
	}
	if !strings.Contains(body, `"code":"OK"`) || strings.Contains(body, "INTERNAL_ERROR") || strings.Contains(body, "secret-panic-detail") {
		t.Fatalf("已写出的响应不应再被失败信封覆盖: %s", body)
	}
}

// discardRecoveryLog 丢掉 Gin Recovery 打到 DefaultErrorWriter 的栈，避免测试输出被刷屏。
// CustomRecovery 在创建时拷贝 writer，必须在 handler.Recovery() 之前调用。
func discardRecoveryLog(t *testing.T) {
	t.Helper()
	old := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = io.Discard
	t.Cleanup(func() { gin.DefaultErrorWriter = old })
}
