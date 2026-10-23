// Package dbplugin 数据库插件
//
// @author zwei
package dbplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/db"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"
	"gitee.com/hq_project/yomi-boot/plugin"

	"gorm.io/gorm"
)

const DBName = "db"

// DB 数据库插件；自包含连接、数据库类型与空闲保活停止函数
type DB struct {
	plugin.Base

	db       *gorm.DB
	dbType   enums.DbType
	stopIdle func()
}

var _ plugin.Plugin = (*DB)(nil)

// NewDB 创建数据库生命周期插件
func NewDB() *DB {
	return &DB{Base: plugin.NewBase(DBName)}
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
func (p *DB) Init(_ context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return plugin.ErrDisabled
	}
	cfg := bootCfg.ToConfig()
	conn, stopIdle, err := db.Open(cfg)
	if err != nil {
		return err
	}
	p.db = conn
	p.dbType = cfg.Type
	p.stopIdle = stopIdle
	return nil
}

// Close 停止空闲保活并关闭数据库连接资源
func (p *DB) Close(context.Context) error {
	if p.stopIdle != nil {
		p.stopIdle()
		p.stopIdle = nil
	}
	if p.db == nil {
		return nil
	}
	sqlDB, err := p.db.DB()
	p.db = nil
	if err != nil {
		return errors.Wrap(err, "获取底层 sql.DB 失败")
	}
	return sqlDB.Close()
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
