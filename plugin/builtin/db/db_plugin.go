// Package dbplugin 数据库插件
//
// @author zwei
package dbplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/db"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"
	"gitee.com/hq_project/yomi-boot/plugin"

	"gorm.io/gorm"
)

const DBName = "db"

// DB 数据库插件；自包含连接、数据库类型与空闲保活停止函数
type DB struct {
	plugin.PluginState

	db       *gorm.DB
	dbType   enums.DbType
	stopIdle func()
}

var _ plugin.Plugin = (*DB)(nil)

// NewDB 创建数据库生命周期插件
func NewDB() *DB {
	return &DB{PluginState: plugin.NewPluginState(DBName)}
}

// FromDB 从插件插槽中取出数据库插件；未注册时返回 nil
func FromDB(slot *plugin.PluginSlot) *DB {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(DBName)
	dbPlugin, _ := p.(*DB)
	return dbPlugin
}

// Init 初始化数据库连接
func (p *DB) Init(slot *plugin.PluginSlot) {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return
	}
	cfg := bootCfg.ToConfig()
	p.dbType = cfg.Type
	p.db, p.stopIdle = db.MustOpen(cfg)
	p.SetStarted()
}

// Close 停止空闲保活并关闭数据库连接资源
func (p *DB) Close() {
	if p.stopIdle != nil {
		p.stopIdle()
		p.stopIdle = nil
	}
	if p.db == nil {
		return
	}
	sqlDB, err := p.db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
	p.db = nil
}

// DB 返回业务数据库连接；未启用时返回 nil
func (p *DB) DB() *gorm.DB {
	if p == nil {
		return nil
	}
	return p.db
}

// Type 返回数据库类型；未启用时返回空值
func (p *DB) Type() enums.DbType {
	if p == nil {
		return ""
	}
	return p.dbType
}
