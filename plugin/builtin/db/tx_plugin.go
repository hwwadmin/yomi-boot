// Package dbplugin 事务执行器插件
//
// @author zwei
package dbplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/db"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// txPlugin 事务执行器插件
type txPlugin struct {
	plugin.PluginState
}

const txName = "tx"

// NewTx 创建事务执行器生命周期插件
func NewTx() plugin.Plugin {
	return &txPlugin{PluginState: plugin.NewPluginState(txName)}
}

// Init 初始化事务执行器并挂到 Host
func (p *txPlugin) Init(host *plugin.Host) {
	if host.DB == nil {
		return
	}
	host.TxExecutor = db.NewTxManager(host.DB)
	p.SetStarted()
}

// Close 关闭事务执行器资源
func (p *txPlugin) Close(_ *plugin.Host) {}
