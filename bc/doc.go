// Package bc 提供密码学基础设施能力簇（Bootstrap Crypto）。
//
// 子包一览：
//   - yomi-boot/bc/passwd：密码单向哈希与校验（bcrypt）
//   - yomi-boot/bc/random：密码学安全随机数
//   - yomi-boot/bc/md5：MD5 摘要（遗留兼容；非安全首选，禁止用于密码）
//   - yomi-boot/bc/hash：SHA-256 / SHA-512 摘要（禁止用于密码）
//   - yomi-boot/bc/hmac：HMAC-SHA256 消息认证
//   - yomi-boot/bc/aes：AES-GCM（推荐）/ AES-CBC / AES-ECB（仅遗留，新业务禁止）
//   - yomi-boot/bc/rsa：RSA PEM、OAEP 加解密、PSS/PKCS1v15 签名
//
// 红线：
//   - 不输出业务 BizCode；不承载密码强度等业务规则。
//   - 密码只用 passwd；勿用 md5/hash/hmac 存密码。
//   - 新加密优先 aes.GCM；ECB 仅对接遗留系统。
//   - 大数据勿直接 RSA 加密（应 AES 内容密钥 + RSA 封密钥）。
//   - 无 DES；不挂载到 Infra 插件链。
//
// @author zwei
package bc
