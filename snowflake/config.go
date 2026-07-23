// Package snowflake 提供雪花 ID 生成器配置
//
// @author zwei
package snowflake

import (
	"yt-boot/config"
	"yt-boot/errors"
)

const (
	// SectionKey YAML 根 key
	SectionKey = "snowflake"

	// maxSnowflakeWorkerID 是雪花算法工作节点 ID 的最大值
	maxSnowflakeWorkerID = 1023
)

// Config 表示雪花 ID 生成器配置
type Config struct {
	WorkerID *int `mapstructure:"worker_id"` // WorkerID 是工作节点 ID（0~1023），多实例部署时须保证唯一
}

// Validate 校验雪花 ID 配置
func (c Config) Validate() error {
	if c.WorkerID == nil {
		return errors.New("snowflake.worker_id 不能为空")
	}
	if *c.WorkerID < 0 || *c.WorkerID > maxSnowflakeWorkerID {
		return errors.New("snowflake.worker_id 须在 0~1023 之间")
	}
	return nil
}

// ConfigSection 返回 snowflake 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
