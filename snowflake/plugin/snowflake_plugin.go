// Package snowflakeplugin 雪花 ID 插件
//
// @author zwei
package snowflakeplugin

import (
	"yt-boot/config"
	"yt-boot/plugin"
	"yt-boot/snowflake"
)

// snowflakePlugin 雪花 ID 插件
type snowflakePlugin struct {
	plugin.PluginState
}

const snowflakeName = "snowflake"

// NewSnowflake 创建雪花 ID 生命周期插件
func NewSnowflake() plugin.Plugin {
	return &snowflakePlugin{PluginState: plugin.NewPluginState(snowflakeName)}
}

// Init 初始化 ID 生成器并挂到 Host
func (p *snowflakePlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[snowflake.Config](host.Doc, snowflake.SectionKey)
	host.IDGenerator = snowflake.MustNewGenerator(int64(*cfg.WorkerID))
	p.SetStarted()
}

// Close 关闭雪花 ID 资源
func (p *snowflakePlugin) Close(_ *plugin.Host) {}
