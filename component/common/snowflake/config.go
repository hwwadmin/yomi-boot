// Package snowflake 提供雪花 ID 生成器配置
//
// @author zwei
package snowflake

import "gitee.com/hq_project/yomi-boot/component/core/errors"

// maxSnowflakeWorkerID 是雪花算法工作节点 ID 的最大值
const maxSnowflakeWorkerID = 1023

// Config 表示雪花 ID 生成器配置
type Config struct {
	WorkerID int // WorkerID 是工作节点 ID（0~1023），多实例部署时须保证唯一
}

// Validate 校验雪花 ID 配置
func (c Config) Validate() error {
	if c.WorkerID < 0 || c.WorkerID > maxSnowflakeWorkerID {
		return errors.New("worker_id 须在 0~1023 之间")
	}
	return nil
}
