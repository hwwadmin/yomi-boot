// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import "time"

// HttpRecord 表示一次 HTTP 请求的访问记录快照
// 由中间件在请求结束后组装，并传递给 HttpRecordService 处理
type HttpRecord struct {
	ServiceType    string        // ServiceType 是来源服务类型，如 admin-bff、user-service
	ServiceVersion string        // ServiceVersion 是来源服务版本号，如 v1
	TraceID        string        // TraceID 是请求链路追踪标识
	RequestAt      time.Time     // RequestAt 是请求到达时间
	Duration       time.Duration // Duration 是请求处理耗时

	Method         string            // Method 是 HTTP 方法，如 GET、POST
	Path           string            // Path 是实际请求路径，如 /admin/api/v1/user
	Route          string            // Route 是 Gin 路由模板，如 /admin/api/v1/user/:id
	Query          string            // Query 是脱敏后的 URL query string
	ClientIP       string            // ClientIP 是客户端 IP
	UserAgent      string            // UserAgent 是客户端 User-Agent
	RequestHeaders map[string]string // RequestHeaders 是脱敏后的请求头集合
	RequestBody    string            // RequestBody 是脱敏后的请求体内容

	UserID     int64  // UserID 是当前登录用户标识，未登录时为 0
	ClientType string // ClientType 是端类型，如 admin、app

	StatusCode            int    // StatusCode 是 HTTP 状态码
	ResponseBody          string // ResponseBody 是脱敏后的响应体内容
	BizCode               string // BizCode 是标准响应体中的业务 code（OK 表示成功）
	BizMsg                string // BizMsg 是标准响应体中的业务 msg
	RequestBodyTruncated  bool   // RequestBodyTruncated 表示请求体是否因超出限制被截断
	ResponseBodyTruncated bool   // ResponseBodyTruncated 表示响应体是否因超出限制被截断
}
