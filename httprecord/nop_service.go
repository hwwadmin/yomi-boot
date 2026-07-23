// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import "context"

// NopService 空实现：不写文件、不发 Kafka，用于默认关闭或测试装配
type NopService struct{}

var _ HttpRecordService = (*NopService)(nil)

// NewNopService 创建空 HTTP 访问记录服务
func NewNopService() *NopService {
	return &NopService{}
}

// Handle 忽略记录
func (s *NopService) Handle(_ context.Context, _ *HttpRecord) {}
