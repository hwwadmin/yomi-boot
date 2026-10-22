// Package worker 工作池入口，负责参数校验及创建工作池
//
// @author zwei
package worker

import "context"

// NewWorker 创建工作池，默认使用基于 channel 的轻量实现
func NewWorker(cfg WorkConfig, parentCtx context.Context) (WorkerService, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return newChannelWorker(cfg, parentCtx)
}

// NewAntsWorker 创建基于 ants 协程池的工作池，适用于海量微秒级短任务或需要动态扩缩容的业务场景
func NewAntsWorker(cfg WorkConfig, parentCtx context.Context) (WorkerService, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return newAntsWorker(cfg, parentCtx)
}
