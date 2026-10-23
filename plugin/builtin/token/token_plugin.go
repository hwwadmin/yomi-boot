// Package tokenplugin 令牌服务插件
//
// @author zwei
package tokenplugin

import (
	"context"

	"gitee.com/hq_project/yomi-boot/component/common/token"
	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
	"gitee.com/hq_project/yomi-boot/plugin"
	redisplugin "gitee.com/hq_project/yomi-boot/plugin/builtin/redis"
)

const Name = "token"

// Token 令牌服务插件；自包含令牌服务
type Token struct {
	plugin.Base

	service token.TokenService
}

var _ plugin.Plugin = (*Token)(nil)

// NewToken 创建令牌服务生命周期插件
func NewToken() *Token {
	return &Token{Base: plugin.NewBase(Name)}
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
func (p *Token) Init(_ context.Context, slot *plugin.PluginSlot) error {
	bootCfg := config.MustSection[BootConfig](slot.Config, SectionKey)
	if !bootCfg.EnabledOrDefault() {
		return plugin.ErrDisabled
	}
	redis := redisplugin.From(slot)
	if redis.Service() == nil {
		return errors.New("token: 需要 redis 插件（redis.enabled=true）")
	}
	if redis.DistLockFactory() == nil {
		return errors.New("token: 需要 redis 插件提供 DistLockFactory")
	}
	p.service = token.NewService(
		token.NewUUIDGenerator(),
		token.NewRedisStore(redis.Service()),
		bootCfg.ToConfig(),
		redis.DistLockFactory(),
	)
	return nil
}

// Close 令牌服务无资源需要释放
func (p *Token) Close(context.Context) error { return nil }

// Service 返回令牌服务；未启用时返回 nil
func (p *Token) Service() token.TokenService {
	if p == nil {
		return nil
	}
	return p.service
}
