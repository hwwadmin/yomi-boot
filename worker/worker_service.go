// Package worker 提供工作池能力
//
// @author zwei
package worker

import "context"

// WorkerHandler 任务处理函数，ctx 由工作池传入
type WorkerHandler func(ctx context.Context) error

// WorkerService 工作池管理接口
type WorkerService interface {
	// Start 启动工作池并开始消费任务，重复调用无效；关闭后不可再启动
	Start()
	// Submit 提交任务到工作池异步执行，须在 Start 之后、工作池关闭之前调用
	Submit(handler WorkerHandler) error
	// SubmitContext 提交任务并在入队阶段响应 ctx 取消与入队超时
	SubmitContext(ctx context.Context, handler WorkerHandler) error
	// TrySubmit 尝试提交任务，队列满时立即返回错误
	TrySubmit(handler WorkerHandler) error
	// Pending 返回当前待处理任务数
	Pending() int
	// Close 停止接收新任务并排空已排队任务
	Close() error
	// Wait 阻塞直至工作池停机完成
	Wait()
}
