// Package logger access 请求/响应体与 Header 采集
//
// @author zwei
package logger

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/utils/httputil"

	"github.com/gin-gonic/gin"
)

// body_omitted_reason / 截断占位（敏感占位见 sensitive.go）
const (
	omitBinaryContent           = "binary_content"
	omitImageContent            = "image_content"
	omitFileUpload              = "file_upload"
	omitBodyTooLarge            = "body_too_large"
	omitAccessBodyDisabled      = "access_body_disabled"
	omitContentTypeNotSupported = "content_type_not_supported"
	omitCaptureFailed           = "capture_failed"
	truncatedPlaceholder        = "[BODY_TRUNCATED]"
)

// bodyCapture 表示一次 body 采集结果
type bodyCapture struct {
	Value         string // Value 是脱敏后的正文（可能截断）
	OmittedReason string // OmittedReason 是省略原因，空表示已记录正文
	ContentType   string // ContentType 是 Content-Type
	Bytes         int64  // Bytes 是原始/采集字节数
	Truncated     bool   // Truncated 表示是否因超限截断
}

// captureRequestBody 读取并还原请求体，按 content-type 与配置决定是否省略
func captureRequestBody(r *http.Request, cfg AccessConfig, sens *Sensitive) bodyCapture {
	out := bodyCapture{}
	if r == nil {
		return out
	}
	out.ContentType = r.Header.Get("Content-Type")
	out.Bytes = r.ContentLength

	if !cfg.RecordBody {
		out.OmittedReason = omitAccessBodyDisabled
		return out
	}

	ct := strings.ToLower(out.ContentType)
	if reason := omitReasonByContentType(ct); reason != "" {
		out.OmittedReason = reason
		return out
	}

	if r.Body == nil {
		return out
	}

	maxSize := cfg.MaxBodyBytes
	if maxSize <= 0 {
		maxSize = 8192
	}
	original := r.Body
	recorded, err := io.ReadAll(io.LimitReader(original, int64(maxSize)+1))
	if err != nil {
		_ = original.Close()
		r.Body = http.NoBody
		out.OmittedReason = omitCaptureFailed
		return out
	}
	httputil.ReplayRequestBody(r, original, recorded, maxSize)
	if len(recorded) == 0 {
		return out
	}
	if int64(len(recorded)) > int64(maxSize) {
		out.Truncated = true
		out.OmittedReason = omitBodyTooLarge
		out.Value = applyBodySensitive(string(recorded[:maxSize]), sens) + truncatedPlaceholder
		return out
	}
	out.Value = applyBodySensitive(string(recorded), sens)
	if out.Bytes <= 0 {
		out.Bytes = int64(len(recorded))
	}
	return out
}

// omitReasonByContentType 根据 Content-Type 返回必须省略正文的原因；可记录时返回空
func omitReasonByContentType(ct string) string {
	switch {
	case strings.HasPrefix(ct, "multipart/"):
		return omitFileUpload
	case strings.HasPrefix(ct, "image/"):
		return omitImageContent
	case strings.Contains(ct, "octet-stream"),
		strings.Contains(ct, "application/pdf"),
		strings.Contains(ct, "application/zip"),
		strings.Contains(ct, "application/vnd."):
		return omitBinaryContent
	default:
		return ""
	}
}

// applyBodySensitive 对可解析 JSON 正文按 sensitive 脱敏，非 JSON 原样返回
func applyBodySensitive(raw string, sens *Sensitive) string {
	if sens == nil {
		return raw
	}
	return sens.ApplyJSON(raw)
}

// looksLikeJSON 粗判字符串是否为 JSON 对象/数组
func looksLikeJSON(s string) bool {
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")
}

// collectHeaders 采集 HTTP Header，敏感键脱敏，超长值截断
func collectHeaders(h http.Header, cfg AccessConfig, sens *Sensitive) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for key, values := range h {
		if len(values) == 0 {
			continue
		}
		val := strings.Join(values, ",")
		if sens != nil && sens.ShouldMask(key) {
			val = sensitivePlaceholder
		} else if cfg.MaxFieldBytes > 0 && len(val) > cfg.MaxFieldBytes {
			val = val[:cfg.MaxFieldBytes] + truncatedPlaceholder
		}
		out[strings.ToLower(key)] = val
	}
	return out
}

// accessBodyWriter 包装 gin.ResponseWriter，在写回客户端的同时采集响应体
type accessBodyWriter struct {
	gin.ResponseWriter
	body      bytes.Buffer
	maxSize   int
	truncated bool
	enabled   bool
}

// newAccessBodyWriter 创建响应体采集包装器；enabled=false 时不采集正文
func newAccessBodyWriter(w gin.ResponseWriter, maxSize int, enabled bool) *accessBodyWriter {
	return &accessBodyWriter{ResponseWriter: w, maxSize: maxSize, enabled: enabled}
}

// Write 写入响应并同步采集
func (w *accessBodyWriter) Write(b []byte) (int, error) {
	w.capture(b)
	return w.ResponseWriter.Write(b)
}

// WriteString 写入响应字符串并同步采集
func (w *accessBodyWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

// capture 按 maxSize 限制追加响应体片段
func (w *accessBodyWriter) capture(b []byte) {
	if !w.enabled || w.maxSize <= 0 || w.truncated {
		return
	}
	remain := w.maxSize - w.body.Len()
	if remain <= 0 {
		w.truncated = true
		return
	}
	if len(b) > remain {
		_, _ = w.body.Write(b[:remain])
		w.truncated = true
		return
	}
	_, _ = w.body.Write(b)
}

// captureResult 根据已采集缓冲与 content-type 生成 bodyCapture
func (w *accessBodyWriter) captureResult(cfg AccessConfig, sens *Sensitive, contentType string) bodyCapture {
	out := bodyCapture{ContentType: contentType}
	if !cfg.RecordBody {
		out.OmittedReason = omitAccessBodyDisabled
		return out
	}
	ct := strings.ToLower(contentType)
	if reason := omitReasonByContentType(ct); reason != "" {
		out.OmittedReason = reason
		return out
	}
	if w.body.Len() == 0 {
		return out
	}
	raw := w.body.String()
	out.Bytes = int64(len(raw))
	if w.truncated {
		out.Truncated = true
		out.OmittedReason = omitBodyTooLarge
		out.Value = applyBodySensitive(raw, sens) + truncatedPlaceholder
		return out
	}
	out.Value = applyBodySensitive(raw, sens)
	return out
}
