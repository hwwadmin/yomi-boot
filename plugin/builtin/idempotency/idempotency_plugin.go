// Package idempotencyplugin 幂等 Processing 超时清理插件
//
// @author zwei
package idempotencyplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/idempotency"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
	dbplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/db"
	redisplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redis"
)

const Name = "idempotency"

// Idempotency 幂等 Processing 超时清理插件；自包含清理器
type Idempotency struct {
	plugin.Base

	cleaner *idempotency.StaleCleaner
}

var _ plugin.Plugin = (*Idempotency)(nil)

// NewIdempotency 创建幂等 Processing 超时清理生命周期插件
func NewIdempotency() *Idempotency {
	return &Idempotency{Base: plugin.NewBase(Name)}
}

// From 从插件插槽中取出幂等清理插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *Idempotency {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	idempotencyPlugin, _ := p.(*Idempotency)
	return idempotencyPlugin
}

// Init 初始化幂等超时清理器
func (p *Idempotency) Init(ctx context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return plugin.ErrDisabled
	}
	app := appctxplugin.From(slot).AppCtx()
	if app == nil {
		return errors.New("idempotency: 需要 app 插件")
	}
	db := dbplugin.FromDB(slot)
	if db.DB() == nil {
		return errors.New("idempotency: 需要 db 插件（db.enabled=true）")
	}
	redis := redisplugin.From(slot)
	if redis.DistLockFactory() == nil {
		return errors.New("idempotency: 需要 redis 插件提供 DistLockFactory")
	}
	cleaner := idempotency.NewStaleCleaner(
		idempotency.NewRepository(db.DB()),
		idempotency.StaleCleanerConfigFrom(bootCfg.ToConfig(), app.Name()),
	)
	if err := cleaner.Start(ctx, redis.DistLockFactory()); err != nil {
		return err
	}
	p.cleaner = cleaner
	return nil
}

// Close 关闭幂等超时清理资源
func (p *Idempotency) Close(context.Context) error {
	if p.cleaner == nil {
		return nil
	}
	err := p.cleaner.Close()
	p.cleaner = nil
	return err
}

// Cleaner 返回幂等超时清理器；未启用时返回 nil
func (p *Idempotency) Cleaner() *idempotency.StaleCleaner {
	if p == nil {
		return nil
	}
	return p.cleaner
}
