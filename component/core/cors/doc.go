// Package cors 提供基于运行时配置的 CORS 中间件：来源/方法/请求头/凭证控制与预检响应。
// 只有同时带 Origin 和 Access-Control-Request-Method 的 OPTIONS 直接返回 204；其它 OPTIONS 继续向后传递。
//
// BootConfig 与 YAML 绑定在 plugin/builtin/cors，由其映射为运行时 Config。
//
// @author zwei
package cors
