// Package eventplugin 消息队列事件插件
//
// @author zwei
package eventplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/event"
	"gitee.com/hq_project/yomi-boot/component/common/logger"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
)

// eventPlugin 消息队列事件插件
type eventPlugin struct {
	plugin.PluginState
	components *event.Components
}

const eventName = "event"

// NewEvent 创建消息队列事件生命周期插件
func NewEvent() plugin.Plugin {
	return &eventPlugin{PluginState: plugin.NewPluginState(eventName)}
}

// Init 初始化消息队列事件组件并挂到 Host
func (p *eventPlugin) Init(host *plugin.Host) {
	bootCfg := config.MustSection[BootConfig](host.Config, SectionKey)
	if !bootCfg.AnyEnabledOrDefault() {
		return
	}
	cfg := bootCfg.ToConfig()
	if host.AppCtx == nil {
		logger.ErrorWithoutCtx("event 初始化失败", "error", errors.New("AppCtx 未初始化"))
		panic(errors.New("event: 需要 AppCtx"))
	}
	if host.DB == nil {
		logger.ErrorWithoutCtx("event 初始化失败", "error", errors.New("DB 未初始化"))
		panic(errors.New("event: 需要 DB"))
	}
	if host.DbType == "" {
		logger.ErrorWithoutCtx("event 初始化失败", "error", errors.New("DbType 未初始化"))
		panic(errors.New("event: 需要 DbType"))
	}
	if host.TxExecutor == nil {
		logger.ErrorWithoutCtx("event 初始化失败", "error", errors.New("TxExecutor 未初始化"))
		panic(errors.New("event: 需要 TxExecutor"))
	}
	if host.IDGenerator == nil {
		logger.ErrorWithoutCtx("event 初始化失败", "error", errors.New("IDGenerator 未初始化"))
		panic(errors.New("event: 需要 IDGenerator"))
	}
	if cfg.OutboxEnabled {
		if host.KafkaPublisher == nil {
			logger.ErrorWithoutCtx("event 初始化失败", "error", errors.New("KafkaPublisher 未初始化"))
			panic(errors.New("event: 发布侧需要 KafkaPublisher"))
		}
		if host.DistLockFactory == nil {
			logger.ErrorWithoutCtx("event 初始化失败", "error", errors.New("DistLockFactory 未初始化"))
			panic(errors.New("event: 发布侧需要 DistLockFactory"))
		}
	}
	if cfg.InboxEnabled && host.DistLockFactory == nil {
		logger.ErrorWithoutCtx("event 初始化失败", "error", errors.New("DistLockFactory 未初始化"))
		panic(errors.New("event: 消费侧需要 DistLockFactory"))
	}
	p.components = event.MustNewAndStart(event.Deps{
		DB:              host.DB,
		TxExecutor:      host.TxExecutor,
		KafkaPublisher:  host.KafkaPublisher,
		DistLockFactory: host.DistLockFactory,
		IDGenerator:     host.IDGenerator,
		DbType:          host.DbType,
		AppName:         host.AppCtx.Name(),
	}, cfg)
	host.EventComponents = p.components
	p.SetStarted()
}

// Close 关闭消息队列事件资源
func (p *eventPlugin) Close(_ *plugin.Host) {
	if p.components == nil {
		return
	}
	_ = p.components.Close()
}
