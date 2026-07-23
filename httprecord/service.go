// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import (
	"context"
)

// HttpRecordService 是 HTTP 访问记录的处理门面
type HttpRecordService interface {
	// Handle 处理一条 HTTP 访问记录，应尽快返回
	Handle(ctx context.Context, record *HttpRecord)
}
