// Package tokenplugin 令牌服务插件
//
// @author zwei
package tokenplugin

import (
	"yomi-boot/config"
	"yomi-boot/errors"
	"yomi-boot/logger"
	"yomi-boot/plugin"
	"yomi-boot/token"
)

// tokenPlugin 令牌服务插件
type tokenPlugin struct {
	plugin.PluginState
}

const tokenName = "token"

// NewToken 创建令牌服务生命周期插件
func NewToken() plugin.Plugin {
	return &tokenPlugin{PluginState: plugin.NewPluginState(tokenName)}
}

// Init 初始化令牌服务并挂到 Host
func (p *tokenPlugin) Init(host *plugin.Host) {
	cfg := config.MustSection[token.Config](host.Doc, token.SectionKey)
	if !cfg.EnabledOrDefault() {
		return
	}
	if host.RedisService == nil {
		logger.ErrorWithoutCtx("token 初始化失败", "error", errors.New("RedisService 未初始化"))
		panic(errors.New("token: 需要 RedisService"))
	}
	if host.DistLockFactory == nil {
		logger.ErrorWithoutCtx("token 初始化失败", "error", errors.New("DistLockFactory 未初始化"))
		panic(errors.New("token: 需要 DistLockFactory"))
	}
	host.TokenService = token.NewService(
		token.NewUUIDGenerator(),
		token.NewRedisStore(host.RedisService),
		*cfg,
		host.DistLockFactory,
	)
	p.SetStarted()
}

// Close 关闭令牌服务资源
func (p *tokenPlugin) Close(_ *plugin.Host) {}
