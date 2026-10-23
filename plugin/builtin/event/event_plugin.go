// Package eventplugin 消息队列事件插件
//
// @author zwei
package eventplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/event"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
	appctxplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/appctx"
	dbplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/db"
	kafkaplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/kafka"
	redisplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redis"
	snowflakeplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/snowflake"
)

const Name = "event"

// Event 消息队列事件插件；自包含 Outbox/Inbox 组件
type Event struct {
	plugin.Base

	components *event.Components
}

var _ plugin.Plugin = (*Event)(nil)

// NewEvent 创建消息队列事件生命周期插件
func NewEvent() *Event {
	return &Event{Base: plugin.NewBase(Name)}
}

// From 从插件插槽中取出消息队列事件插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *Event {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	eventPlugin, _ := p.(*Event)
	return eventPlugin
}

// Init 初始化消息队列事件组件
func (p *Event) Init(ctx context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.AnyEnabledOrDefault() {
		return plugin.ErrDisabled
	}
	cfg := bootCfg.ToConfig()

	app := appctxplugin.From(slot).AppCtx()
	db := dbplugin.FromDB(slot)
	tx := dbplugin.FromTx(slot)
	redis := redisplugin.From(slot)
	kafka := kafkaplugin.FromPublisher(slot)
	ids := snowflakeplugin.From(slot)

	if app == nil {
		return errors.New("event: 需要 app 插件")
	}
	if db.DB() == nil {
		return errors.New("event: 需要 db 插件（db.enabled=true）")
	}
	if db.Type() == "" {
		return errors.New("event: 需要 db 插件提供 DbType")
	}
	if tx.Executor() == nil {
		return errors.New("event: 需要 tx 插件（依赖 db）")
	}
	if ids.Generator() == nil {
		return errors.New("event: 需要 snowflake 插件")
	}
	if cfg.OutboxEnabled {
		if kafka.Service() == nil {
			return errors.New("event: 发布侧需要 kafka-publisher 插件")
		}
		if redis.DistLockFactory() == nil {
			return errors.New("event: 发布侧需要 redis 插件提供 DistLockFactory")
		}
	}
	if cfg.InboxEnabled && redis.DistLockFactory() == nil {
		return errors.New("event: 消费侧需要 redis 插件提供 DistLockFactory")
	}

	components, err := event.NewComponents(event.Deps{
		DB:              db.DB(),
		TxExecutor:      tx.Executor(),
		KafkaPublisher:  kafka.Service(),
		DistLockFactory: redis.DistLockFactory(),
		IDGenerator:     ids.Generator(),
		DbType:          db.Type(),
		AppName:         app.Name(),
	}, cfg)
	if err != nil {
		return err
	}
	if err := components.Start(ctx); err != nil {
		_ = components.Close()
		return err
	}
	p.components = components
	return nil
}

// Close 关闭消息队列事件资源
func (p *Event) Close(context.Context) error {
	if p.components == nil {
		return nil
	}
	err := p.components.Close()
	p.components = nil
	return err
}

// Components 返回事件组件；未启用时返回 nil
func (p *Event) Components() *event.Components {
	if p == nil {
		return nil
	}
	return p.components
}
