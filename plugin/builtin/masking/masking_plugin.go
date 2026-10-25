package maskingplugin

import (
	"context"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
	"gitee.com/hq_project/yomi-boot/component/core/masking"
	"gitee.com/hq_project/yomi-boot/plugin"
)

const Name = "masking"

// Masking 统一脱敏服务插件；始终启动（空规则即全透传），并注入进程级默认实例
type Masking struct {
	plugin.Base

	service   *masking.Service
	ruleCount int
}

var _ plugin.Plugin = (*Masking)(nil)
var _ plugin.ReadyHook = (*Masking)(nil)

// NewMasking 创建统一脱敏服务生命周期插件
func NewMasking() *Masking {
	return &Masking{Base: plugin.NewBase(Name)}
}

// From 从插件插槽中取出脱敏服务插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *Masking {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	maskingPlugin, _ := p.(*Masking)
	return maskingPlugin
}

// Init 构造脱敏服务并注入全局默认实例；纯函数服务，无后台任务
func (p *Masking) Init(_ context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	svc, err := masking.New(bootCfg.ToConfig())
	if err != nil {
		return err
	}
	p.service = svc
	p.ruleCount = len(bootCfg.Rules)
	masking.SetDefault(svc)
	return nil
}

// OnReady 在日志枢纽就绪后输出策略注册清单。
// 本插件 Init 先于 logger 插件执行，注册又发生在 boot.Run 之前，
// 因此注册清单统一在 Ready 阶段落日志（控制台 + info 文件）。
func (p *Masking) OnReady(context.Context) error {
	logger.InfoWithoutCtx("masking 脱敏策略注册清单",
		"strategies", strings.Join(masking.RegisteredStrategies(), ","),
		"custom", strings.Join(masking.CustomStrategyNames(), ","),
		"rules", p.ruleCount,
	)
	return nil
}

// Close 无资源需要释放
func (p *Masking) Close(context.Context) error {
	p.service = nil
	return nil
}

// Service 返回脱敏服务；未初始化时返回全透传实例
func (p *Masking) Service() *masking.Service {
	if p == nil || p.service == nil {
		return masking.Noop()
	}
	return p.service
}
