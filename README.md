# Yomi-Boot

Go **基础设施库**（能力库 + 插件生命周期），**不含业务规则**。  
业务工程通过 `go.mod` 标准 `require gitee.com/hq_project/yomi-boot` 依赖本库，按 YAML 配置按需启用能力，可覆盖轻量单体、模块化大单体、分布式微服务等形态。

**当前底座已足够支撑绝大多数企业业务。** 默认按需裁剪（改 YAML `enabled` 等开关即可），不必一上来启用 Consul / Kafka / DTM 等分布式组件。

可通过 [https://gitee.com/hq_project/yomi-boot-template](https://gitee.com/hq_project/yomi-boot-template) 模板项目快速开始。

# 仓库目录

| 路径 | 说明 |
|------|------|
| `bootstrap/`、`plugin/`、`config/`、`runner/`、`appctx/`、`banner/` | 生命周期与装配 |
| `router/`、`handler/`、`response/`、`cors/`、`requestguard/`、`traceid/`、`httprecord/`、`logger/` | HTTP / 路由 / 协议适配 |
| `db/`、`idempotency/`、`dtm/`、`event/` | 数据访问与事务 |
| `redis/`、`distlock/`、`ratelimit/`、`roundrobin/`、`cronjob/`、`worker/`、`snowflake/` | 缓存、锁、限流与调度 |
| `consul/`、`rpc/`、`kafka/` | 服务发现、RPC 与消息 |
| `token/`、`bc/` | 安全、令牌与密码学 |
| `filestorage/`、`pprof/` | 对象存储与可观测 |
| `errors/`、`shared/`、`utils/`、`testclient/` | 错误、共享模型与工具 |
| `_prompts/` | Agent / Skill / 知识库与规范文档 |
| `LICENSE` | Apache License 2.0 全文 |

能力职责与落位速查见 [基础库能力索引](./_prompts/doc/基础库能力索引.md)。


# 来源与许可证

| 项 | 说明 |
|----|------|
| 仓库 | [https://gitee.com/hq_project/yomi-boot](https://gitee.com/hq_project/yomi-boot) |
| Go module | `gitee.com/hq_project/yomi-boot` |
| 版权 | Copyright 2026 zwei |
| 许可证 | [Apache License 2.0](./LICENSE) |

本仓库以 Apache License 2.0 授权。使用、复制、修改与分发时请遵守该许可证条款；完整条文见根目录 [LICENSE](./LICENSE)。第三方依赖各自遵循其原许可证。
