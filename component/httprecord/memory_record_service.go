// Package httprecord 提供 HTTP 访问记录采集与处理能力
//
// @author zwei
package httprecord

import (
	"context"
	"sync"
	"sync/atomic"

	"gitee.com/hq_project/yomi-boot/core/logger"
)

// MemoryHttpRecordService 通过进程内内存队列异步处理 HTTP 访问记录
type MemoryHttpRecordService struct {
	queue  chan *HttpRecord
	ctx    context.Context
	cancel context.CancelFunc

	consumerStarted atomic.Bool
	consumerMu      sync.Mutex
	consumerCancel  context.CancelFunc
}

var _ HttpRecordService = (*MemoryHttpRecordService)(nil)

// NewMemoryHttpRecordService 创建内存队列 HTTP 访问记录服务
// queueSize 为必填容量，须大于 0，缺失时启动期 fail-fast
func NewMemoryHttpRecordService(queueSize int) *MemoryHttpRecordService {
	if queueSize <= 0 {
		panic("httprecord: NewMemoryHttpRecordService queueSize 必须大于 0")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &MemoryHttpRecordService{
		queue:  make(chan *HttpRecord, queueSize),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Handle 将访问记录非阻塞写入内存队列；队列满或已关闭时直接丢弃并打 Warn
func (s *MemoryHttpRecordService) Handle(ctx context.Context, record *HttpRecord) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warn(ctx, "HTTP 访问记录内存队列入队 panic", "panic", r)
		}
	}()

	if s == nil || record == nil {
		return
	}

	select {
	case <-s.ctx.Done():
		logger.Warn(ctx, "HTTP 访问记录内存队列已关闭，已丢弃",
			"traceId", record.TraceID,
			"path", record.Path,
			"method", record.Method,
		)
		return
	default:
	}

	select {
	case s.queue <- record:
	default:
		logger.Warn(ctx, "HTTP 访问记录内存队列已满，已丢弃",
			"traceId", record.TraceID,
			"path", record.Path,
			"method", record.Method,
			"queueCap", cap(s.queue),
		)
	}
}

// Close 停止接收新记录并取消消费侧；不等待、不排空队列
func (s *MemoryHttpRecordService) Close() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.consumerMu.Lock()
	cancel := s.consumerCancel
	s.consumerMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
