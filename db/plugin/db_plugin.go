// Package dbplugin 数据库插件
//
// @author zwei
package dbplugin

import (
	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/db"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// dbPlugin 数据库插件
type dbPlugin struct {
	plugin.PluginState
	stopIdle func()
}

const dbName = "db"

// NewDB 创建数据库生命周期插件
func NewDB() plugin.Plugin {
	return &dbPlugin{PluginState: plugin.NewPluginState(dbName)}
}

// Init 初始化数据库连接并挂到 Host
func (p *dbPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[db.Config](host.Doc, db.SectionKey)
	if !cfg.EnabledOrDefault() {
		return
	}
	host.DbType = cfg.Type
	host.DB, p.stopIdle = db.MustOpen(*cfg)
	p.SetStarted()
}

// Close 停止空闲保活并关闭数据库连接资源
func (p *dbPlugin) Close(host *plugin.Host) {
	if p.stopIdle != nil {
		p.stopIdle()
		p.stopIdle = nil
	}
	if host.DB == nil {
		return
	}
	sqlDB, err := host.DB.DB()
	if err != nil {
		return
	}
	_ = sqlDB.Close()
}
