package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/handler"
	"github.com/hwwadmin/yomi-boot/component/core/logger"
	"github.com/hwwadmin/yomi-boot/component/core/masking"
	"github.com/hwwadmin/yomi-boot/component/core/response"
	sharederr "github.com/hwwadmin/yomi-boot/component/core/shared/error"
)

func TestLogRequestError_MasksQueryLikeAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("mask on", func(t *testing.T) {
		log := logRequestQuery(t, true)
		if strings.Contains(log, "secret-token") {
			t.Fatalf("query 中的 token 应被打码: %s", log)
		}
		if !strings.Contains(log, "q=keep") {
			t.Fatalf("普通 query 参数应保留: %s", log)
		}
	})

	t.Run("mask off", func(t *testing.T) {
		log := logRequestQuery(t, false)
		if !strings.Contains(log, "secret-token") {
			t.Fatalf("关闭脱敏时应保留 query 原文: %s", log)
		}
	})
}

func TestNormalizeFailureKeepsCauseOutOfWireCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if !response.Registered() {
		response.MustRegister(response.DefaultEncoder{})
	}

	t.Run("timeout", func(t *testing.T) {
		rec, log := captureFailure(t, func(c *gin.Context) {
			handler.WriteBusinessError(c, errors.Wrap(context.DeadlineExceeded, "查询订单失败"))
		})
		if !strings.Contains(rec, `"code":"REQUEST_TIMEOUT"`) {
			t.Fatalf("对外 code 应为 REQUEST_TIMEOUT，实际 %s", rec)
		}
		if strings.Contains(rec, "查询订单失败") {
			t.Fatalf("超时原因不应进入响应: %s", rec)
		}
		if !strings.Contains(log, "查询订单失败") || !strings.Contains(log, "REQUEST_TIMEOUT") {
			t.Fatalf("日志应保留收口前的错误和对外 code: %s", log)
		}
	})

	t.Run("max bytes", func(t *testing.T) {
		rec, log := captureFailure(t, func(c *gin.Context) {
			handler.WriteBusinessError(c, &http.MaxBytesError{Limit: 128})
		})
		if !strings.Contains(rec, `"code":"REQUEST_BODY_TOO_LARGE"`) {
			t.Fatalf("对外 code 应为 REQUEST_BODY_TOO_LARGE，实际 %s", rec)
		}
		if strings.Contains(rec, "request body too large") {
			t.Fatalf("超限原因不应进入响应: %s", rec)
		}
		if !strings.Contains(log, "request body too large") || !strings.Contains(log, "REQUEST_BODY_TOO_LARGE") {
			t.Fatalf("日志应保留超限原因和对外 code: %s", log)
		}
	})

	t.Run("json bind", func(t *testing.T) {
		rec, log := captureFailure(t, func(c *gin.Context) {
			c.Request = httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(`{}`))
			c.Request.Header.Set("Content-Type", "application/json")
			var input struct {
				OrderID string `json:"orderId" binding:"required"`
			}
			if handler.BindJSON(c, &input) {
				t.Fatal("缺少 orderId 时应绑定失败")
			}
		})
		if !strings.Contains(rec, `"code":"INVALID_BODY"`) {
			t.Fatalf("对外 code 应为 INVALID_BODY，实际 %s", rec)
		}
		if strings.Contains(rec, "orderId") || strings.Contains(rec, "OrderID") {
			t.Fatalf("字段路径不应进入响应: %s", rec)
		}
		if !strings.Contains(log, "orderId") && !strings.Contains(log, "OrderID") {
			t.Fatalf("日志应保留绑定字段路径: %s", log)
		}
		if !strings.Contains(log, "INVALID_BODY") {
			t.Fatalf("日志应保留对外 code: %s", log)
		}
	})

	t.Run("json bind identity", func(t *testing.T) {
		err := handler.JSONBindError(&http.MaxBytesError{Limit: 8})
		if err.Error() != sharederr.ErrRequestBodyTooLarge.Error() {
			t.Fatalf("超限绑定时对外文案应为哨兵，实际 %s", err.Error())
		}
		if !errors.Is(err, sharederr.ErrRequestBodyTooLarge) {
			t.Fatal("超限绑定应仍匹配 REQUEST_BODY_TOO_LARGE")
		}
		bindErr := handler.JSONBindError(errors.New("Key: 'Order.OrderID'"))
		if bindErr.Error() != sharederr.ErrInvalidBody.Error() {
			t.Fatalf("绑定失败对外文案应为 INVALID_BODY，实际 %s", bindErr.Error())
		}
		if !errors.Is(bindErr, sharederr.ErrInvalidBody) {
			t.Fatal("绑定失败应仍匹配 INVALID_BODY")
		}
		if !strings.Contains(errors.Stack(bindErr), "OrderID") {
			t.Fatalf("原因链应保留字段路径: %s", errors.Stack(bindErr))
		}
	})
}

func TestWriteBusinessError_NilBizAndFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if !response.Registered() {
		response.MustRegister(response.DefaultEncoder{})
	}

	t.Run("nil request error", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		handler.LogRequestError(c, nil)
	})

	t.Run("nil err", func(t *testing.T) {
		rec, log := captureFailure(t, func(c *gin.Context) {
			handler.WriteBusinessError(c, nil)
		})
		if !strings.Contains(rec, `"code":"INTERNAL_ERROR"`) {
			t.Fatalf("空错误应落成 INTERNAL_ERROR: %s", rec)
		}
		if !strings.Contains(log, "INTERNAL_ERROR") {
			t.Fatalf("日志应记录 INTERNAL_ERROR: %s", log)
		}
	})

	t.Run("nil err with fallback", func(t *testing.T) {
		rec, _ := captureFailure(t, func(c *gin.Context) {
			handler.WriteBusinessErrorWithFallback(c, nil, sharederr.ErrRequestTimeout)
		})
		if !strings.Contains(rec, `"code":"REQUEST_TIMEOUT"`) {
			t.Fatalf("空错误应改用 fallback: %s", rec)
		}
	})

	t.Run("biz", func(t *testing.T) {
		rec, log := captureFailure(t, func(c *gin.Context) {
			handler.WriteBusinessError(c, errors.NewBiz("BIZ_CODE", "业务失败"))
		})
		if !strings.Contains(rec, "BIZ_CODE") || !strings.Contains(rec, "业务失败") {
			t.Fatalf("业务错误应原样输出: %s", rec)
		}
		if !strings.Contains(log, "BIZ_CODE") || !strings.Contains(log, "业务失败") {
			t.Fatalf("日志应带上业务码和消息: %s", log)
		}
	})

	t.Run("fallback wraps unknown", func(t *testing.T) {
		rec, log := captureFailure(t, func(c *gin.Context) {
			handler.WriteBusinessErrorWithFallback(c, errors.New("db down"), sharederr.ErrInternal)
		})
		if !strings.Contains(rec, `"code":"INTERNAL_ERROR"`) {
			t.Fatalf("未知错误应改用 fallback: %s", rec)
		}
		if !strings.Contains(log, "db down") {
			t.Fatalf("日志应保留原始原因: %s", log)
		}
	})
}

func captureFailure(t *testing.T, write func(c *gin.Context)) (body, log string) {
	t.Helper()
	dir := t.TempDir()
	console := false
	async := false
	closeFn, err := logger.InitLogger("test-svc", "test", logger.Config{
		Dir:            dir,
		Level:          "info",
		ConsoleEnabled: &console,
		AsyncEnabled:   &async,
		Rotation: logger.RotationConfig{
			MaxSizeMB:     50,
			RetentionDays: 3,
			MaxBackups:    30,
			MaxTotalMB:    2048,
		},
	}, nil)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	t.Cleanup(func() { _ = closeFn() })

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	write(c)
	if err := logger.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "error", "error.log"))
	if err != nil {
		t.Fatalf("read error log: %v", err)
	}
	return rec.Body.String(), string(b)
}

func logRequestQuery(t *testing.T, maskSensitive bool) string {
	t.Helper()
	dir := t.TempDir()
	maskSvc, err := masking.New(masking.Config{Rules: []masking.RuleConfig{
		{Keys: []string{"token", "password"}, Strategy: masking.StrategyRedact},
	}})
	if err != nil {
		t.Fatalf("masking.New: %v", err)
	}
	closeFn, err := logger.InitLogger("test-svc", "test", logger.Config{
		Dir:           dir,
		Level:         "info",
		MaskSensitive: maskSensitive,
		Rotation: logger.RotationConfig{
			MaxSizeMB:     50,
			RetentionDays: 3,
			MaxBackups:    30,
			MaxTotalMB:    2048,
		},
	}, maskSvc)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	t.Cleanup(func() { _ = closeFn() })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/login?token=secret-token&q=keep", nil)
	handler.LogRequestError(c, errors.New("TOKEN_REQUIRED"))
	if err := logger.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(dir, "error", "error.log"))
	if err != nil {
		t.Fatalf("read error log: %v", err)
	}
	return string(b)
}
