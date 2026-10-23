// Package dbplugin 事务执行器插件
//
// @author zwei
package dbplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/db"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const TxName = "tx"

// Tx 事务执行器插件；自包含事务执行器，依赖前序 db 插件
type Tx struct {
	plugin.PluginState

	executor db.TxExecutor
}

var _ plugin.Plugin = (*Tx)(nil)

// NewTx 创建事务执行器生命周期插件
func NewTx() *Tx {
	return &Tx{PluginState: plugin.NewPluginState(TxName)}
}

// FromTx 从插件插槽中取出事务执行器插件；未注册时返回 nil
func FromTx(slot *plugin.PluginSlot) *Tx {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(TxName)
	txPlugin, _ := p.(*Tx)
	return txPlugin
}

// Init 基于前序 db 插件创建事务执行器；数据库未启用时跳过
func (p *Tx) Init(slot *plugin.PluginSlot) {
	dbPlugin := FromDB(slot)
	if dbPlugin.DB() == nil {
		return
	}
	p.executor = db.NewTxManager(dbPlugin.DB())
	p.SetStarted()
}

// Close 事务执行器无资源需要释放
func (p *Tx) Close() {}

// Executor 返回事务执行器；未启用时返回 nil
func (p *Tx) Executor() db.TxExecutor {
	if p == nil {
		return nil
	}
	return p.executor
}
