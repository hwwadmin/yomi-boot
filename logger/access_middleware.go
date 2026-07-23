// Package logger HTTP 文件 access 中间件
//
// 同一次入站 HTTP 产生成对 access 日志（类 AOP around）：
//   - 请求侧字段先缓冲，handler 结束后再决定是否落盘
//   - 落盘时成对写入：direction=inbound + direction=outbound
//   - sample_rate 默认 1（全量）；仅约束成功请求；status>=400 必采
//
// @author zwei
package logger

import (
	"context"
	"math/rand"
	"path"
	"strings"
	"time"
	"yomi-boot/shared"
	"yomi-boot/traceid"
	"yomi-boot/utils/ginutil"

	"github.com/gin-gonic/gin"
)

// accessRandFloat64 采样用随机源，单测可替换
var accessRandFloat64 = rand.Float64

// AccessMiddleware 返回成对写入 access 文件的 Gin 中间件（入站 / 出站各一条）
func AccessMiddleware(cfg AccessConfig) gin.HandlerFunc {
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 8192
	}
	if cfg.MaxFieldBytes <= 0 {
		cfg.MaxFieldBytes = 1024
	}
	sampleRate := cfg.SampleRateOrDefault()
	skip := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skip[normalizePath(p)] = struct{}{}
	}
	return func(c *gin.Context) {
		if !cfg.EnabledOrDefault() {
			c.Next()
			return
		}
		reqPath := ""
		if c.Request != nil && c.Request.URL != nil {
			reqPath = c.Request.URL.Path
		}
		if _, ok := skip[normalizePath(reqPath)]; ok {
			c.Next()
			return
		}

		sens := currentSensitive()
		ctx := requestContext(c)
		requestID := traceid.FromContext(ctx)
		if strings.TrimSpace(requestID) == "" {
			requestID = shared.MissingTraceID
		}
		method := ""
		var ua, query string
		if c.Request != nil {
			method = c.Request.Method
			ua = c.Request.UserAgent()
			if c.Request.URL != nil {
				query = sens.ApplyQuery(c.Request.URL.RawQuery)
			}
		}
		clientIP := ginutil.ClientIP(c)

		reqBody := captureRequestBody(c.Request, cfg, sens)
		var reqHeaders map[string]string
		if c.Request != nil {
			reqHeaders = collectHeaders(c.Request.Header, cfg, sens)
		}

		// —— 入站快照：先缓冲，不落盘 ——
		inboundKV := []any{
			"direction", DirectionInbound,
			"request_id", requestID,
			"method", method,
			"path", reqPath,
			"client_ip", clientIP,
		}
		if ua != "" {
			inboundKV = append(inboundKV, "user_agent", ua)
		}
		if query != "" {
			inboundKV = append(inboundKV, "query", query)
		}
		if reqBody.ContentType != "" {
			inboundKV = append(inboundKV, "request_content_type", reqBody.ContentType)
		}
		if reqBody.Bytes > 0 {
			inboundKV = append(inboundKV, "request_bytes", reqBody.Bytes)
		}
		if len(reqHeaders) > 0 {
			inboundKV = append(inboundKV, "request_headers", reqHeaders)
		}
		if reqBody.Value != "" {
			inboundKV = append(inboundKV, "request_body", reqBody.Value)
		}
		if reqBody.OmittedReason != "" {
			inboundKV = append(inboundKV, "body_omitted_reason", reqBody.OmittedReason)
		}

		blw := newAccessBodyWriter(c.Writer, cfg.MaxBodyBytes, cfg.RecordBody)
		c.Writer = blw

		start := time.Now()
		c.Next()

		// —— 出站快照 + 采样判定后成对落盘 ——
		defer func() { _ = recover() }()

		route := c.FullPath()
		if route == "" {
			route = reqPath
		}
		status := 0
		if c.Writer != nil {
			status = c.Writer.Status()
		}
		if !shouldKeepAccess(status, sampleRate) {
			return
		}

		respCT := ""
		if c.Writer != nil {
			respCT = c.Writer.Header().Get("Content-Type")
		}
		respBody := blw.captureResult(cfg, sens, respCT)
		respHeaders := collectHeaders(c.Writer.Header(), cfg, sens)

		outboundKV := []any{
			"direction", DirectionOutbound,
			"request_id", requestID,
			"method", method,
			"path", reqPath,
			"route", route,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"client_ip", clientIP,
		}
		if ua != "" {
			outboundKV = append(outboundKV, "user_agent", ua)
		}
		if query != "" {
			outboundKV = append(outboundKV, "query", query)
		}
		if respCT != "" {
			outboundKV = append(outboundKV, "response_content_type", respCT)
		}
		if respBody.Bytes > 0 {
			outboundKV = append(outboundKV, "response_bytes", respBody.Bytes)
		} else if c.Writer != nil && c.Writer.Size() > 0 {
			outboundKV = append(outboundKV, "response_bytes", c.Writer.Size())
		}
		if len(respHeaders) > 0 {
			outboundKV = append(outboundKV, "response_headers", respHeaders)
		}
		if respBody.Value != "" {
			outboundKV = append(outboundKV, "response_body", respBody.Value)
		}
		if respBody.OmittedReason != "" {
			outboundKV = append(outboundKV, "body_omitted_reason", respBody.OmittedReason)
		}

		Access(ctx, "http inbound", inboundKV...)
		Access(requestContext(c), "http outbound", outboundKV...)
	}
}

// requestContext 从 Gin 上下文取出请求 context，缺失时返回 Background
func requestContext(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}

// normalizePath 规范化路径便于与 skip_paths 比对
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	cleaned := path.Clean(p)
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

// shouldKeepAccess 决定是否落盘：status>=400 必采；否则按 sample_rate
func shouldKeepAccess(status int, rate float64) bool {
	if status >= 400 {
		return true
	}
	return shouldSample(rate)
}

// shouldSample 按采样率决定是否记录成功请求；rate>=1 全量
func shouldSample(rate float64) bool {
	if rate >= 1 {
		return true
	}
	if rate <= 0 {
		return true
	}
	return accessRandFloat64() < rate
}
