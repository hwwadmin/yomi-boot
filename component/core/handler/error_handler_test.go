package handler_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"github.com/hwwadmin/yomi-boot/component/core/handler"
	"github.com/hwwadmin/yomi-boot/component/core/logger"
	"github.com/hwwadmin/yomi-boot/component/core/masking"

	"github.com/gin-gonic/gin"
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
