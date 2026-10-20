// Package bc 提供密码学基础设施能力簇（Bootstrap Crypto）。
//
// 子包一览：
//   - gitee.com/hq_project/yomi-boot/bc/passwd：密码单向哈希与校验（bcrypt）
//   - gitee.com/hq_project/yomi-boot/bc/random：密码学安全随机数
//   - gitee.com/hq_project/yomi-boot/bc/hash：MD5 / SHA-1（遗留兼容）/ SHA-256 / SHA-512 摘要（禁止用于密码）
//   - gitee.com/hq_project/yomi-boot/bc/sm3：SM3 摘要（国密；禁止用于密码）
//   - gitee.com/hq_project/yomi-boot/bc/hmacsha：HMAC-SHA1（遗留）/ HMAC-SHA256 / HMAC-SHA512 消息认证
//   - gitee.com/hq_project/yomi-boot/bc/aes：AES-GCM（推荐，支持 AAD）/ AES-CBC / AES-ECB（仅遗留，新业务禁止）
//   - gitee.com/hq_project/yomi-boot/bc/sm4：SM4-GCM（推荐）/ SM4-CBC / SM4-ECB（仅遗留，国密）
//   - gitee.com/hq_project/yomi-boot/bc/rsa：RSA PEM（含证书、带密码私钥）、OAEP 加解密
//   - gitee.com/hq_project/yomi-boot/bc/sha256withrsa：SHA256withRSA（PKCS#1 v1.5 + SHA-256）签名验签
//   - gitee.com/hq_project/yomi-boot/bc/rsapss：RSASSA-PSS + SHA-256 签名验签
//   - gitee.com/hq_project/yomi-boot/bc/sm2：SM2 PEM、加解密、签名验签（国密）
//
// 红线：
//   - 不输出业务 BizCode；不承载密码强度等业务规则。
//   - 密码只用 passwd；勿用 hash/sm3/hmacsha 存密码。
//   - 新加密优先 aes.GCM；国密优先 sm4.GCM + sm2；ECB 仅对接遗留系统。
//   - 大数据勿直接 RSA/SM2 加密（应 AES/SM4 内容密钥 + RSA/SM2 封密钥）。
//   - 无 DES；不挂载到 Infra 插件链。
//
// @author zwei
package bc
