// Package masking 提供统一脱敏服务：字段名匹配 + 策略应用，供日志、访问记录与业务出参共用。
//
// 设计约定：
//   - 规则唯一来源是配置（Config.Rules），引擎不携带任何默认敏感字段清单；
//     规则为空即全链路透传。
//   - 内置策略（phone/idcard/bankcard/email/name）与业务自定义策略共用同一注册表；
//     自定义策略须在 boot.Run 之前调用 MustRegisterStrategy 完成注册。
//   - 安全不变量：字段命中规则后绝不原文返回；策略形状不符时降级为 placeholder。
//   - redact / keep_edges 为保留策略名，由引擎内建解析，不可注册覆盖：
//     redact 直接替换为 placeholder，keep_edges 依规则内 keep_head/keep_tail 保留头尾。
//
// 消费方式：logger / httprecord 等组件通过注入 *Service 调用 Mask* 系列方法
// （nil 接收者等价于透传）；业务代码可直接使用 Current() 返回的全局默认实例
// （由 masking 插件在 Init 阶段注入）。
//
// @author zwei
package masking
