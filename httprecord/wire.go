// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import "time"

// wireRecord 是 Kafka 消息序列化结构
type wireRecord struct {
	TraceID               string            `json:"traceId"`
	ServiceType           string            `json:"serviceType"`
	ServiceVersion        string            `json:"serviceVersion"`
	RequestAt             int64             `json:"requestAt"`
	DurationMs            int64             `json:"durationMs"`
	Method                string            `json:"method"`
	Path                  string            `json:"path"`
	Route                 string            `json:"route,omitempty"`
	Query                 string            `json:"query,omitempty"`
	ClientIP              string            `json:"clientIp,omitempty"`
	UserAgent             string            `json:"userAgent,omitempty"`
	RequestHeaders        map[string]string `json:"requestHeaders,omitempty"`
	RequestBody           string            `json:"requestBody,omitempty"`
	RequestBodyTruncated  bool              `json:"requestBodyTruncated"`
	UserID                int64             `json:"userId"`
	ClientType            string            `json:"clientType,omitempty"`
	StatusCode            int               `json:"statusCode"`
	ResponseBody          string            `json:"responseBody,omitempty"`
	ResponseBodyTruncated bool              `json:"responseBodyTruncated"`
	BizCode               string            `json:"bizCode,omitempty"`
	BizMsg                string            `json:"bizMsg,omitempty"`
}

// toWireRecord 将访问记录转换为 Kafka 消息结构
func toWireRecord(record *HttpRecord) wireRecord {
	if record == nil {
		return wireRecord{}
	}
	return wireRecord{
		TraceID:               record.TraceID,
		ServiceType:           record.ServiceType,
		ServiceVersion:        record.ServiceVersion,
		RequestAt:             record.RequestAt.UnixMilli(),
		DurationMs:            record.Duration.Milliseconds(),
		Method:                record.Method,
		Path:                  record.Path,
		Route:                 record.Route,
		Query:                 record.Query,
		ClientIP:              record.ClientIP,
		UserAgent:             record.UserAgent,
		RequestHeaders:        record.RequestHeaders,
		RequestBody:           record.RequestBody,
		RequestBodyTruncated:  record.RequestBodyTruncated,
		UserID:                record.UserID,
		ClientType:            record.ClientType,
		StatusCode:            record.StatusCode,
		ResponseBody:          record.ResponseBody,
		ResponseBodyTruncated: record.ResponseBodyTruncated,
		BizCode:               record.BizCode,
		BizMsg:                record.BizMsg,
	}
}

// fromWireRecord 将 Kafka 消息结构转换为访问记录
func fromWireRecord(wire wireRecord) *HttpRecord {
	if wire.Method == "" && wire.Path == "" && wire.TraceID == "" {
		return nil
	}
	return &HttpRecord{
		TraceID:               wire.TraceID,
		ServiceType:           wire.ServiceType,
		ServiceVersion:        wire.ServiceVersion,
		RequestAt:             time.UnixMilli(wire.RequestAt),
		Duration:              time.Duration(wire.DurationMs) * time.Millisecond,
		Method:                wire.Method,
		Path:                  wire.Path,
		Route:                 wire.Route,
		Query:                 wire.Query,
		ClientIP:              wire.ClientIP,
		UserAgent:             wire.UserAgent,
		RequestHeaders:        wire.RequestHeaders,
		RequestBody:           wire.RequestBody,
		RequestBodyTruncated:  wire.RequestBodyTruncated,
		UserID:                wire.UserID,
		ClientType:            wire.ClientType,
		StatusCode:            wire.StatusCode,
		ResponseBody:          wire.ResponseBody,
		ResponseBodyTruncated: wire.ResponseBodyTruncated,
		BizCode:               wire.BizCode,
		BizMsg:                wire.BizMsg,
	}
}
