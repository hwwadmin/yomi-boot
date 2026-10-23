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
	plugin.PluginState

	cleaner *idempotency.StaleCleaner
}

var _ plugin.Plugin = (*Idempotency)(nil)

// NewIdempotencyStaleCleaner 创建幂等 Processing 超时清理生命周期插件
func NewIdempotencyStaleCleaner() *Idempotency {
	return &Idempotency{PluginState: plugin.NewPluginState(Name)}
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
func (p *Idempotency) Init(slot *plugin.PluginSlot) {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return
	}
	app := appctxplugin.From(slot).AppCtx()
	if app == nil {
		panic(errors.New("idempotency: 需要 app 插件"))
	}
	db := dbplugin.FromDB(slot)
	if db.DB() == nil {
		panic(errors.New("idempotency: 需要 db 插件（db.enabled=true）"))
	}
	redis := redisplugin.From(slot)
	if redis.DistLockFactory() == nil {
		panic(errors.New("idempotency: 需要 redis 插件提供 DistLockFactory"))
	}
	p.cleaner = idempotency.MustStartStaleCleanerFromConfig(context.Background(), idempotency.StaleCleanerDeps{
		Repo:            idempotency.NewRepository(db.DB()),
		DistLockFactory: redis.DistLockFactory(),
		AppName:         app.Name(),
		Config:          bootCfg.ToConfig(),
	})
	if p.cleaner != nil {
		p.SetStarted()
	}
}

// Close 关闭幂等超时清理资源
func (p *Idempotency) Close() {
	if p.cleaner == nil {
		return
	}
	_ = p.cleaner.Close()
	p.cleaner = nil
}

// Cleaner 返回幂等超时清理器；未启用时返回 nil
func (p *Idempotency) Cleaner() *idempotency.StaleCleaner {
	if p == nil {
		return nil
	}
	return p.cleaner
}
