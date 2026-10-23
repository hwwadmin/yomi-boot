// Package eventplugin 消息队列事件插件
//
// @author zwei
package eventplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/event"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
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
	plugin.PluginState

	components *event.Components
}

var _ plugin.Plugin = (*Event)(nil)

// NewEvent 创建消息队列事件生命周期插件
func NewEvent() *Event {
	return &Event{PluginState: plugin.NewPluginState(Name)}
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
func (p *Event) Init(slot *plugin.PluginSlot) {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.AnyEnabledOrDefault() {
		return
	}
	cfg := bootCfg.ToConfig()

	app := appctxplugin.From(slot).AppCtx()
	db := dbplugin.FromDB(slot)
	tx := dbplugin.FromTx(slot)
	redis := redisplugin.From(slot)
	kafka := kafkaplugin.FromPublisher(slot)
	ids := snowflakeplugin.From(slot)

	if app == nil {
		fail("需要 app 插件")
	}
	if db.DB() == nil {
		fail("需要 db 插件（db.enabled=true）")
	}
	if db.Type() == "" {
		fail("需要 db 插件提供 DbType")
	}
	if tx.Executor() == nil {
		fail("需要 tx 插件（依赖 db）")
	}
	if ids.Generator() == nil {
		fail("需要 snowflake 插件")
	}
	if cfg.OutboxEnabled {
		if kafka.Service() == nil {
			fail("发布侧需要 kafka-publisher 插件")
		}
		if redis.DistLockFactory() == nil {
			fail("发布侧需要 redis 插件提供 DistLockFactory")
		}
	}
	if cfg.InboxEnabled && redis.DistLockFactory() == nil {
		fail("消费侧需要 redis 插件提供 DistLockFactory")
	}

	p.components = event.MustNewAndStart(event.Deps{
		DB:              db.DB(),
		TxExecutor:      tx.Executor(),
		KafkaPublisher:  kafka.Service(),
		DistLockFactory: redis.DistLockFactory(),
		IDGenerator:     ids.Generator(),
		DbType:          db.Type(),
		AppName:         app.Name(),
	}, cfg)
	p.SetStarted()
}

// Close 关闭消息队列事件资源
func (p *Event) Close() {
	if p.components == nil {
		return
	}
	_ = p.components.Close()
	p.components = nil
}

// Components 返回事件组件；未启用时返回 nil
func (p *Event) Components() *event.Components {
	if p == nil {
		return nil
	}
	return p.components
}

// fail 记录事件初始化失败并 panic
func fail(msg string) {
	logger.ErrorWithoutCtx("event 初始化失败", "error", errors.New(msg))
	panic(errors.Newf("event: %s", msg))
}
