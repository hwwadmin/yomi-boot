// Package tokenplugin 令牌服务插件
//
// @author zwei
package tokenplugin

import (
	"gitee.com/hq_project/yomi-boot/component/common/token"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/component/core/logger"
	"gitee.com/hq_project/yomi-boot/plugin"
	redisplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redis"
)

const Name = "token"

// Token 令牌服务插件；自包含令牌服务
type Token struct {
	plugin.PluginState

	service token.TokenService
}

var _ plugin.Plugin = (*Token)(nil)

// NewToken 创建令牌服务生命周期插件
func NewToken() *Token {
	return &Token{PluginState: plugin.NewPluginState(Name)}
}

// From 从插件插槽中取出令牌服务插件；未注册时返回 nil
func From(slot *plugin.PluginSlot) *Token {
	if slot == nil {
		return nil
	}
	p, _ := slot.Plugin(Name)
	tokenPlugin, _ := p.(*Token)
	return tokenPlugin
}

// Init 初始化令牌服务
func (p *Token) Init(slot *plugin.PluginSlot) {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return
	}
	redis := redisplugin.From(slot)
	if redis.Service() == nil {
		logger.ErrorWithoutCtx("token 初始化失败", "error", errors.New("redis 插件未启动"))
		panic(errors.New("token: 需要 redis 插件（redis.enabled=true）"))
	}
	if redis.DistLockFactory() == nil {
		logger.ErrorWithoutCtx("token 初始化失败", "error", errors.New("DistLockFactory 未初始化"))
		panic(errors.New("token: 需要 redis 插件提供 DistLockFactory"))
	}
	p.service = token.NewService(
		token.NewUUIDGenerator(),
		token.NewRedisStore(redis.Service()),
		bootCfg.ToConfig(),
		redis.DistLockFactory(),
	)
	p.SetStarted()
}

// Close 令牌服务无资源需要释放
func (p *Token) Close() {}

// Service 返回令牌服务；未启用时返回 nil
func (p *Token) Service() token.TokenService {
	if p == nil {
		return nil
	}
	return p.service
}
