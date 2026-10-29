// Package captcha 提供图形验证码的生成与一次性校验能力（渲染基于 github.com/mojocn/base64Captcha）。
//
// 本包与业务解耦：不读 YAML、不进插件链、不做限流；Config 由调用方显式构造，
// Store 由调用方注入（memory 适用于单实例或粘性会话，Redis 适用于多实例）。
//
// # 基本用法
//
//	store, err := captcha.NewRedisStore(redisService, captcha.RedisStoreConfig{KeyPrefix: "yomi:captcha:login"})
//	svc, err := captcha.New(store, captcha.Config{Driver: captcha.DriverDigit, Length: 4, TTL: 3 * time.Minute})
//	challenge, err := svc.Generate(ctx) // 返回 challenge.ID 与 challenge.Image 给前端
//	err = svc.Verify(ctx, req.CaptchaID, req.CaptchaCode)
//
// # 安全不变量
//
//   - Generate 只返回 id 与图片，答案只写 Store，不返回给调用方；日志不得打印答案。
//   - Verify 先原子消费再比较，无论答案对错都会消费，同一个 id 至多成功一次。
//   - 过期由 Store 执行：Redis 用 PX，memory 逐条检查到期时间。
//   - Store 故障一律向上返回，绝不当作验证通过；业务应按 500 处理。
//
// # 存储选择
//
//   - memory：仅单实例或粘性会话可用；进程重启即失效；达到 MaxEntries 后拒绝新生成。
//   - Redis：多实例与滚动发布必须使用；key 为 "prefix:id"，TTL 由 Redis 保证；
//     建议按场景使用独立前缀（如 yomi:captcha:login）。
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
// Service 与内置 Store 均可并发使用；Driver 在 New 时构造一次并复用，不能每请求重建
// （会重复解析字体）。
//
// # 业务侧
//
// 验证码只是速度闸门，业务仍须对生成与校验接口做限流，并在生成响应上设置
// Cache-Control: no-store；如担心验证码被转发，应把 id 绑定到会话后校验。
//
// # 错误
//
// 包内哨兵：ErrInvalidArgument、ErrExpired、ErrMismatch、ErrGenerateFailed、ErrStoreFull。
//
// @author zwei
package captcha
