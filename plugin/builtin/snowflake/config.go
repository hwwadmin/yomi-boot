package snowflakeplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/snowflake"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// SectionKey YAML 根 key
const SectionKey = "snowflake"

// BootConfig 表示 snowflake 配置段的 YAML 绑定
type BootConfig struct {
	WorkerID *int `mapstructure:"worker_id"` // WorkerID 是工作节点 ID（0~1023），多实例部署时须保证唯一
}

// ToConfig 转换为运行时配置
func (c BootConfig) ToConfig() snowflake.Config {
	workerID := 0
	if c.WorkerID != nil {
		workerID = *c.WorkerID
	}
	return snowflake.Config{WorkerID: workerID}
}

// Validate 校验 snowflake 配置段语义
func (c BootConfig) Validate() error {
	if c.WorkerID == nil {
		return errors.New("snowflake.worker_id 不能为空")
	}
	if err := c.ToConfig().Validate(); err != nil {
		return errors.Wrap(err, "%s", SectionKey)
	}
	return nil
}

// ConfigSection 返回 snowflake 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &BootConfig{}, func(c *BootConfig, _ *config.Document) error {
		return c.Validate()
	})
}
