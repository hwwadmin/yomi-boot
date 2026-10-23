// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/common/appctx"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/common/token"
	"gitee.com/hq_project/yomi-boot/component/core/response"
	"gitee.com/hq_project/yomi-boot/component/core/traceid"
	"gitee.com/hq_project/yomi-boot/component/core/utils/ginutil"
	"gitee.com/hq_project/yomi-boot/component/core/utils/httputil"
	"gitee.com/hq_project/yomi-boot/component/core/utils/utf8util"

	"github.com/gin-gonic/gin"
)

// truncatedSuffix 是请求体或响应体超出采集限制时的截断标记
const truncatedSuffix = "...[truncated]"

// sensitiveHeaderKeys 是需要脱敏的请求头键名集合（不区分大小写）
var sensitiveHeaderKeys = map[string]struct{}{
	"authorization": {},
	"cookie":        {},
	"set-cookie":    {},
}

// sensitiveBodyKeys 是需要脱敏的 JSON 字段键名集合（不区分大小写）
var sensitiveBodyKeys = map[string]struct{}{
	"password":      {},
	"oldpassword":   {},
	"newpassword":   {},
	"token":         {},
	"access_token":  {},
	"refresh_token": {},
}

// HttpRecordMiddleware 表示 HTTP 访问记录采集中间件实例
type HttpRecordMiddleware struct {
	svc HttpRecordService
	cfg Config
	app appctx.AppCtx
}

// Handler 返回当前中间件实例对应的 gin 中间件处理器
// 在请求前后采集访问信息，并在请求结束后调用 HttpRecordService 处理
func (m *HttpRecordMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !m.cfg.MiddlewareEnabled || m.shouldSkip(c.Request.URL.Path) {
			c.Next()
			return
		}

		start := time.Now()
		requestAt := start
		var requestBody string
		var requestBodyTruncated bool
		if m.cfg.RecordRequestBody {
			requestBody, requestBodyTruncated = readRequestBodyForRecord(c.Request, m.cfg.MaxRequestBodySizeOrDefault())
			if m.cfg.MaskSensitive && requestBody != "" {
				requestBody = maskJSONString(requestBody)
			}
		}

		blw := newBodyRecordWriter(c.Writer, m.cfg.MaxResponseBodySizeOrDefault())
		c.Writer = blw

		c.Next()

		record := m.buildRecord(c, requestAt, time.Since(start), requestBody, requestBodyTruncated, blw)
		m.safeInvoke(c.Request.Context(), record)
	}
}

// shouldSkip 判断当前请求路径是否命中跳过采集配置
func (m *HttpRecordMiddleware) shouldSkip(requestPath string) bool {
	currentPath := normalizePath(requestPath)
	for _, skipPath := range m.cfg.SkipPaths {
		if normalizePath(skipPath) == currentPath {
			return true
		}
	}
	return false
}

// safeInvoke 安全调用 HttpRecordService，捕获 panic 防止影响 HTTP 主链路
func (m *HttpRecordMiddleware) safeInvoke(ctx context.Context, record *HttpRecord) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warn(ctx, "HTTP 访问记录处理 panic", "panic", r)
		}
	}()

	if record == nil || m.svc == nil {
		return
	}

	m.svc.Handle(ctx, record)
}

// buildRecord 根据 Gin 上下文与采集结果组装访问记录快照
func (m *HttpRecordMiddleware) buildRecord(
	c *gin.Context,
	requestAt time.Time,
	duration time.Duration,
	requestBody string,
	requestBodyTruncated bool,
	blw *bodyRecordWriter,
) *HttpRecord {
	r := c.Request
	record := &HttpRecord{
		ServiceType:           m.serviceType(),
		ServiceVersion:        m.serviceVersion(),
		TraceID:               traceid.FromContext(c.Request.Context()),
		RequestAt:             requestAt,
		Duration:              duration,
		Method:                r.Method,
		Path:                  r.URL.Path,
		Route:                 c.FullPath(),
		ClientIP:              ginutil.ClientIP(c),
		UserAgent:             r.UserAgent(),
		RequestBody:           requestBody,
		RequestBodyTruncated:  requestBodyTruncated,
		StatusCode:            c.Writer.Status(),
		ResponseBodyTruncated: blw.truncated,
	}

	if r.URL.RawQuery != "" {
		query := r.URL.RawQuery
		if m.cfg.MaskSensitive {
			query = maskQueryString(query)
		}
		record.Query = query
	}

	record.RequestHeaders = collectRequestHeaders(r, m.cfg.MaskSensitive)

	if session, ok := token.SessionFromContext(r.Context()); ok && session != nil {
		record.UserID = session.UserID
		record.ClientType = session.ClientType
	}

	if m.cfg.RecordResponseBody {
		responseBody := blw.recordedBody()
		if m.cfg.MaskSensitive && responseBody != "" {
			responseBody = maskJSONString(responseBody)
		}
		record.ResponseBody = responseBody
	}

	if record.ResponseBody != "" {
		record.BizCode, record.BizMsg = parseBizResponse(record.ResponseBody)
	}

	if record.StatusCode == 0 {
		record.StatusCode = http.StatusOK
	}

	return record
}

// serviceType 返回当前服务类型
func (m *HttpRecordMiddleware) serviceType() string {
	return m.app.Name()
}

// serviceVersion 返回当前服务版本号
func (m *HttpRecordMiddleware) serviceVersion() string {
	return m.app.Version()
}

// normalizePath 规范化请求路径，便于与 skip_paths 配置比对
func normalizePath(requestPath string) string {
	currentPath := path.Clean(strings.TrimSpace(requestPath))
	if currentPath == "." || currentPath == "/" {
		return "/"
	}
	return currentPath
}

// readRequestBodyForRecord 读取并还原请求体，返回用于记录输出的内容与是否截断标记
// maxSize == utf8util.Unlimited 时读取完整 body；否则仅读取 maxSize+1 字节用于判断是否截断
func readRequestBodyForRecord(r *http.Request, maxSize int) (string, bool) {
	if r == nil || r.Body == nil {
		return "", false
	}
	if maxSize == 0 {
		return "", false
	}

	original := r.Body
	var recorded []byte
	var err error
	replayMax := maxSize
	if maxSize < 0 {
		recorded, err = io.ReadAll(original)
		replayMax = len(recorded)
	} else {
		recorded, err = io.ReadAll(io.LimitReader(original, int64(maxSize)+1))
	}
	if err != nil {
		_ = original.Close()
		r.Body = http.NoBody
		return "", false
	}
	httputil.ReplayRequestBody(r, original, recorded, replayMax)

	if len(recorded) == 0 {
		return "", false
	}

	cut, truncated := utf8util.Cut(recorded, maxSize)
	if truncated {
		return string(cut) + truncatedSuffix, true
	}
	return string(cut), false
}

// bodyRecordWriter 包装 gin.ResponseWriter，在写回客户端的同时采集响应体
type bodyRecordWriter struct {
	gin.ResponseWriter
	body      bytes.Buffer // body 是已采集的响应体内容
	maxSize   int          // maxSize 是响应体记录最大采集字节数；<0 表示不截断
	truncated bool         // truncated 表示响应体是否因超出限制被截断
}

// newBodyRecordWriter 创建带响应体采集能力的 ResponseWriter 包装器
func newBodyRecordWriter(writer gin.ResponseWriter, maxSize int) *bodyRecordWriter {
	return &bodyRecordWriter{
		ResponseWriter: writer,
		maxSize:        maxSize,
	}
}

// Write 写入响应字节并同步采集记录内容
func (w *bodyRecordWriter) Write(b []byte) (int, error) {
	w.capture(b)
	return w.ResponseWriter.Write(b)
}

// WriteString 写入响应字符串并同步采集记录内容
func (w *bodyRecordWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

// capture 按 maxSize 限制采集响应体片段；maxSize < 0 表示不截断
func (w *bodyRecordWriter) capture(b []byte) {
	if w.truncated {
		return
	}
	if w.maxSize < 0 {
		_, _ = w.body.Write(b)
		return
	}
	if w.maxSize == 0 {
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

// recordedBody 返回已采集响应体的合法 UTF-8 快照（超限时回退到字符边界）
func (w *bodyRecordWriter) recordedBody() string {
	if w == nil || w.maxSize == 0 {
		return ""
	}
	cut, _ := utf8util.Cut(w.body.Bytes(), w.maxSize)
	return string(cut)
}

// collectRequestHeaders 收集请求头，并按需对敏感头脱敏
func collectRequestHeaders(r *http.Request, maskSensitive bool) map[string]string {
	if r == nil || len(r.Header) == 0 {
		return nil
	}

	headers := make(map[string]string, len(r.Header))
	for key, values := range r.Header {
		if len(values) == 0 {
			continue
		}
		value := strings.Join(values, ",")
		if maskSensitive && isSensitiveHeader(key) {
			value = "***"
		}
		headers[key] = value
	}
	return headers
}

// isSensitiveHeader 判断请求头键名是否属于敏感字段
func isSensitiveHeader(key string) bool {
	_, ok := sensitiveHeaderKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

// maskJSONString 对 JSON 字符串中的敏感字段进行脱敏
func maskJSONString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}

	// UseNumber 避免大整数（如雪花 ID）经 float64 往返时精度丢失
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return raw
	}

	masked := maskJSONValue(payload)
	data, err := json.Marshal(masked)
	if err != nil {
		return raw
	}
	return string(data)
}

// maskJSONValue 递归脱敏 JSON 值中的敏感字段
func maskJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		masked := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveBodyKey(key) {
				masked[key] = "***"
				continue
			}
			masked[key] = maskJSONValue(item)
		}
		return masked
	case []any:
		masked := make([]any, len(typed))
		for i, item := range typed {
			masked[i] = maskJSONValue(item)
		}
		return masked
	default:
		return value
	}
}

// isSensitiveBodyKey 判断 JSON 字段键名是否属于敏感字段
func isSensitiveBodyKey(key string) bool {
	_, ok := sensitiveBodyKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

// maskQueryString 对 query string 中的敏感参数进行脱敏
func maskQueryString(rawQuery string) string {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}

	for key, items := range values {
		if !isSensitiveBodyKey(key) {
			continue
		}
		masked := make([]string, len(items))
		for i := range items {
			masked[i] = "***"
		}
		values[key] = masked
	}

	return values.Encode()
}

// parseBizResponse 从响应 JSON 中解析业务 code 与 msg（经已注册 Encoder.Parse）
func parseBizResponse(body string) (code, msg string) {
	if !response.Registered() {
		return "", ""
	}
	parsed, ok := response.Enc().Parse([]byte(body))
	if !ok {
		return "", ""
	}
	return parsed.Code, parsed.Msg
}

// Middleware 返回基于 HttpRecordService 与配置注入的 gin 中间件（gin.HandlerFunc）
// svc 为可选依赖（为 nil 时跳过记录处理），app 为必填依赖缺失时启动期 fail-fast
func Middleware(svc HttpRecordService, cfg Config, app appctx.AppCtx) gin.HandlerFunc {
	if app == nil {
		panic("httprecord: Middleware app 不能为 nil")
	}
	return (&HttpRecordMiddleware{
		svc: svc,
		cfg: cfg,
		app: app,
	}).Handler()
}
