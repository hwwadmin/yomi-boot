// Package dbplugin 数据库插件
//
// @author zwei
package dbplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/db"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/shared/enums"
	"gitee.com/hq_project/yomi-boot/plugin"

	"gorm.io/gorm"
)

const DBName = "db"

// DB 数据库插件；自包含数据源及其生命周期
type DB struct {
	plugin.Base

	conn *db.Conn
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

// Init 初始化数据源
func (p *DB) Init(ctx context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return plugin.ErrDisabled
	}
	conn, err := db.Open(ctx, bootCfg.ToConfig())
	if err != nil {
		return err
	}
	p.conn = conn
	return nil
}

// Close 停止空闲保活并关闭连接池
func (p *DB) Close(context.Context) error {
	if p == nil || p.conn == nil {
		return nil
	}
	err := p.conn.Close()
	p.conn = nil
	return err
}

// DB 返回业务数据库连接；未启用时返回 nil
func (p *DB) DB() *gorm.DB {
	if p == nil {
		return nil
	}
	return p.conn.GORM()
}

// Type 返回数据库类型；未启用时返回空值
func (p *DB) Type() enums.DbType {
	if p == nil {
		return ""
	}
	return p.conn.Type()
}
