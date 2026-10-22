// Package worker 工作池配置参数定义与校验
//
// @author zwei
package worker

import (
	"time"

	"gitee.com/hq_project/yomi-boot/core/errors"
)

const defaultSubmitTimeout = 10 * time.Second // defaultSubmitTimeout 是任务提交等待队列空位的默认超时

// WorkConfig 工作池配置
type WorkConfig struct {
	Name          string        // Name 工作池名称，用于日志标识
	WorkNum       int           // WorkNum 并发 worker 协程数
	Size          int           // Size 任务队列缓冲容量
	SubmitTimeout time.Duration // SubmitTimeout 是入队等待超时，0 表示使用默认值
}

// validate 校验 WorkConfig 参数合法性
func (cfg *WorkConfig) validate() error {
	if cfg.Name == "" {
		return errors.New("worker.name 不能为空")
	}
	if cfg.WorkNum <= 0 {
		return errors.New("worker.work_num 不能小于等于0")
	}
	if cfg.Size <= 0 {
		return errors.New("worker.size 不能小于等于0")
	}
	if cfg.SubmitTimeout < 0 {
		return errors.New("worker.submit_timeout 不能小于 0")
	}
	return nil
}

// effectiveSubmitTimeout 返回实际生效的入队等待超时
func (cfg WorkConfig) effectiveSubmitTimeout() time.Duration {
	if cfg.SubmitTimeout > 0 {
		return cfg.SubmitTimeout
	}
	return defaultSubmitTimeout
}
