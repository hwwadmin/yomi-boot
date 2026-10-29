// Package captcha 提供图形验证码的生成与一次性校验能力（渲染基于 github.com/mojocn/base64Captcha）。
//
// 本包与业务解耦：不读 YAML、不进插件链、不做限流；Config 由调用方显式构造，
// Store 由调用方注入（本包提供 Redis 实现，也可自行实现 Store 接入其他存储）。
//
// # 基本用法
//
//	store, err := captcha.NewRedisStore(redisService, captcha.RedisStoreConfig{KeyPrefix: "yomi:captcha:login"})
//	svc, err := captcha.New(store, captcha.Config{Driver: captcha.DriverDigit, Length: 4, TTL: 3 * time.Minute})
//	challenge, err := svc.Generate(ctx) // 返回 ID / Image / ExpiresIn 给前端
//	err = svc.Verify(ctx, req.CaptchaID, req.CaptchaCode)
//
// # 安全不变量
//
//   - Generate 只返回 id 与图片；答案在服务端归一化（去首尾空白、转大写）后只以 SHA-256
//     摘要写入 Store，明文答案不落存储，日志也不得打印答案。
//   - Verify 先原子消费再比较，无论答案对错都会消费，同一个 id 至多成功一次。
//   - 过期由 Redis 保证：写入时 SET 带过期时间（整秒 TTL 走 EX），不做进程内计时。
//   - Store 故障一律向上返回，绝不当作验证通过；业务应按 500 处理。
//
// # 存储
//
//   - 本包只提供 Redis 实现，多实例与滚动发布可直接使用；key 为 "prefix:id"，默认前缀
//     yomi:captcha:default，TTL 由 Redis 保证；建议按场景改成独立前缀（如 yomi:captcha:login）。
//   - Store 是导出接口，业务可实现自己的版本，但必须满足：只保存 64 位十六进制摘要、
//     原子取出并删除、并发消费同一 id 至多命中一次；实现不得假设收到的是明文答案。
//
// # 驱动
//
// 内置 digit（默认）、string、math。digit 与 string 使用 Length；math 的答案是算式结果，
// Length 忽略。chinese 与 audio 暂不支持。
//
// # 成本
//
// base64Captcha 在包初始化时解析内嵌字体，约占用 5.9MB 堆、耗时 3.8ms，二进制约增加 7.1MB
// （本机实测，随平台与版本浮动）；240x80、4 位数字的图片 data URI 约 2.4KB，
// 生成加校验约 0.75ms。
//
// # 并发
//
// Service 与 RedisStore 均可并发使用；Driver 在 New 时构造一次并复用，不能每请求重建
// （会重复解析字体）。
//
// # 业务侧
//
// 验证码只是速度闸门，业务仍须对生成与校验接口做限流，并在生成响应上设置
// Cache-Control: no-store；如担心验证码被转发，应把 id 绑定到会话后校验。
//
// # 错误
//
// 包内哨兵：ErrInvalidArgument、ErrExpired、ErrMismatch、ErrGenerateFailed。
//
// @author zwei
package captcha
