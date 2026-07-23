// Package logger 提供统一日志初始化能力
//
// @author zwei
package logger

import (
	"context"
	"fmt"
	"strings"
	"yomi-boot/shared"
	"yomi-boot/traceid"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Debug 输出 Debug 级别日志，traceID 从 ctx 读取
func Debug(ctx context.Context, msg string, keysAndValues ...any) {
	write(ctx, zapcore.DebugLevel, msg, keysAndValues...)
}

// DebugWithoutCtx 输出 Debug 级别日志（无请求上下文，仅用于启动等场景）
func DebugWithoutCtx(msg string, keysAndValues ...any) {
	write(context.Background(), zapcore.DebugLevel, msg, keysAndValues...)
}

// Info 输出 Info 级别日志，traceID 从 ctx 读取
func Info(ctx context.Context, msg string, keysAndValues ...any) {
	write(ctx, zapcore.InfoLevel, msg, keysAndValues...)
}

// InfoWithoutCtx 输出 Info 级别日志（无请求上下文，仅用于启动等场景）
func InfoWithoutCtx(msg string, keysAndValues ...any) {
	write(context.Background(), zapcore.InfoLevel, msg, keysAndValues...)
}

// Warn 输出 Warn 级别日志，traceID 从 ctx 读取
func Warn(ctx context.Context, msg string, keysAndValues ...any) {
	write(ctx, zapcore.WarnLevel, msg, keysAndValues...)
}

// WarnWithoutCtx 输出 Warn 级别日志（无请求上下文，仅用于启动等场景）
func WarnWithoutCtx(msg string, keysAndValues ...any) {
	write(context.Background(), zapcore.WarnLevel, msg, keysAndValues...)
}

// Error 输出 Error 级别日志，traceID 从 ctx 读取
func Error(ctx context.Context, msg string, keysAndValues ...any) {
	write(ctx, zapcore.ErrorLevel, msg, keysAndValues...)
}

// ErrorWithoutCtx 输出 Error 级别日志（无请求上下文，仅用于启动等场景）
func ErrorWithoutCtx(msg string, keysAndValues ...any) {
	write(context.Background(), zapcore.ErrorLevel, msg, keysAndValues...)
}

// Access 写入 access 文件日志（不进入 info/error/debug 文件）
func Access(ctx context.Context, msg string, keysAndValues ...any) {
	keysAndValues = currentSensitive().ApplyKV(keysAndValues...)
	fields := injectTraceFields(ctx, withoutMetaFields(kvToFields(keysAndValues...))...)
	accessLogger().Info(msg, fields...)
}

// write 写入指定级别日志，自动注入 trace_id / span_id，并按 sensitive 脱敏业务字段
func write(ctx context.Context, level zapcore.Level, msg string, keysAndValues ...any) {
	keysAndValues = currentSensitive().ApplyKV(keysAndValues...)
	fields := injectTraceFields(ctx, withoutMetaFields(kvToFields(keysAndValues...))...)

	switch level {
	case zapcore.DebugLevel:
		debugLogger().Debug(msg, fields...)
	case zapcore.InfoLevel:
		infoLogger().Info(msg, fields...)
	case zapcore.WarnLevel:
		infoLogger().Warn(msg, fields...)
	case zapcore.ErrorLevel:
		errorLogger().Error(msg, fields...)
	default:
		infoLogger().Warn(msg, append(fields, zap.String("unexpected_level", level.String()))...)
	}
}

// injectTraceFields 从 ctx 注入 trace_id / span_id（及非空 parent_span_id）
func injectTraceFields(ctx context.Context, extra ...zap.Field) []zap.Field {
	sc := traceid.SpanFromContext(ctx)
	traceID := strings.TrimSpace(sc.TraceID)
	if traceID == "" {
		traceID = shared.MissingTraceID
	}
	spanID := strings.TrimSpace(sc.SpanID)
	if spanID == "" {
		spanID = shared.MissingSpanID
	}
	fields := []zap.Field{
		zap.String(shared.TraceID, traceID),
		zap.String(shared.SpanID, spanID),
	}
	if parent := strings.TrimSpace(sc.ParentSpanID); parent != "" {
		fields = append(fields, zap.String(shared.ParentSpanID, parent))
	}
	return append(fields, extra...)
}

// withoutMetaFields 过滤调用方误传的系统元数据字段（如 trace_id / span_id）
func withoutMetaFields(fields []zap.Field) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	filtered := make([]zap.Field, 0, len(fields))
	for _, field := range fields {
		switch field.Key {
		case shared.TraceID, shared.SpanID, shared.ParentSpanID:
			continue
		}
		filtered = append(filtered, field)
	}
	return filtered
}

// accessLogger 返回 access 文件 logger，未初始化时回退开发模式 logger
func accessLogger() *zap.Logger {
	if hub := defaultHub.Load(); hub != nil && hub.access != nil {
		return hub.access
	}
	return fallbackLogger()
}

// infoLogger 返回 info 文件 logger（含控制台 Tee）
func infoLogger() *zap.Logger {
	if hub := defaultHub.Load(); hub != nil && hub.info != nil {
		return hub.info
	}
	return fallbackLogger()
}

// errorLogger 返回 error 文件 logger（含控制台 Tee）
func errorLogger() *zap.Logger {
	if hub := defaultHub.Load(); hub != nil && hub.errorL != nil {
		return hub.errorL
	}
	return fallbackLogger()
}

// debugLogger 返回 debug logger（文件或仅控制台）
func debugLogger() *zap.Logger {
	if hub := defaultHub.Load(); hub != nil && hub.debug != nil {
		return hub.debug
	}
	return fallbackLogger()
}

// fallbackLogger 未 Init 时的开发模式兜底
func fallbackLogger() *zap.Logger {
	fallback, _ := zap.NewDevelopment()
	return fallback
}

// kvToFields 将成对 key-value 转为 zap.Field；奇数个参数记为 invalid_log_pairs
func kvToFields(keysAndValues ...any) []zap.Field {
	if len(keysAndValues) == 0 {
		return nil
	}
	if len(keysAndValues)%2 != 0 {
		return []zap.Field{zap.Any("invalid_log_pairs", keysAndValues)}
	}

	fields := make([]zap.Field, 0, len(keysAndValues)/2)
	for i := 0; i < len(keysAndValues); i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			key = fmt.Sprint(keysAndValues[i])
		}
		fields = append(fields, zap.Any(key, keysAndValues[i+1]))
	}
	return fields
}
