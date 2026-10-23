// Package snowflakeplugin 雪花 ID 插件
//
// @author zwei
package snowflakeplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/snowflake"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const Name = "snowflake"

// Snowflake 雪花 ID 插件；自包含 ID 生成器
type Snowflake struct {
	plugin.Base

	generator snowflake.Generator
}

var _ plugin.Plugin = (*Snowflake)(nil)

// NewSnowflake 创建雪花 ID 生命周期插件
func NewSnowflake() *Snowflake {
	return &Snowflake{Base: plugin.NewBase(Name)}
}

// From 从插件插槽中取出雪花 ID 插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *Snowflake {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	snowflakePlugin, _ := p.(*Snowflake)
	return snowflakePlugin
}

// Init 初始化 ID 生成器
func (p *Snowflake) Init(_ context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	cfg := bootCfg.ToConfig()
	generator, err := snowflake.NewGenerator(int64(cfg.WorkerID))
	if err != nil {
		return err
	}
	p.generator = generator
	return nil
}

// Close 雪花 ID 生成器无资源需要释放
func (p *Snowflake) Close(context.Context) error { return nil }

// Generator 返回 ID 生成器；未初始化时返回 nil
func (p *Snowflake) Generator() snowflake.Generator {
	if p == nil {
		return nil
	}
	return p.generator
}
