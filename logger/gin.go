// Package logger 提供统一日志初始化能力
//
// @author zwei
package logger

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// SetupGin 将 Gin 启动与调试输出重定向到 logger，须在 InitLogger 之后、创建 Engine 之前调用
func SetupGin() {
	gin.DefaultWriter = &ginLogWriter{write: DebugWithoutCtx}
	gin.DefaultErrorWriter = &ginLogWriter{write: WarnWithoutCtx}
}

// ginLogWriter 将 Gin 框架的调试输出重定向到统一日志
type ginLogWriter struct {
	write func(msg string, keysAndValues ...any)
}

// Write 实现 io.Writer 接口，将 Gin 输出写入 logger
func (w *ginLogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		w.write(msg)
	}
	return len(p), nil
}
