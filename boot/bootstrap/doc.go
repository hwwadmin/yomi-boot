// Package bootstrap 提供基础设施初始化与 HTTP 服务装配。
//
// 初始化只负责加载配置文档、按内置插件清单启动资源，业务依赖经 *plugin.Services 暴露；
// HTTP 装配负责中间件链与 gin 服务构建，不感知具体组件。
//
// @author zwei
package bootstrap
