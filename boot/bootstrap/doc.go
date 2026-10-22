// Package bootstrap 提供基础设施初始化。
//
// 负责注册配置段、加载配置文档并按内置插件清单启动资源；业务依赖经 *plugin.Services 暴露。
// HTTP 服务与中间件装配在 boot/server。
//
// @author zwei
package bootstrap
