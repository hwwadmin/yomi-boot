# yomi-boot

Go **基础设施库**（能力库 + 插件生命周期），**不含业务规则**。  
业务工程通过 `go.mod` 标准 `require gitee.com/hq_project/yomi-boot` 依赖本库，按 YAML 配置按需启用能力，可覆盖轻量单体、模块化大单体、分布式微服务等形态。

**当前底座已足够支撑绝大多数企业业务。** 默认按需裁剪（改 YAML `enabled` 等开关即可），不必一上来启用 Consul / Kafka / DTM 等分布式组件。

可通过 [https://gitee.com/hq_project/yomi-boot-template](https://gitee.com/hq_project/yomi-boot-template) 模板项目快速开始。

## 仓库目录

| 路径 | 说明 |
|------|------|
| `bootstrap/`、`plugin/`、`config/`、`runner/`、`appctx/`、`banner/` | 生命周期与装配 |
| `router/`、`handler/`、`response/`、`cors/`、`requestguard/`、`traceid/`、`httprecord/`、`logger/` | HTTP / 路由 / 协议适配 |
| `db/`、`idempotency/`、`dtm/`、`event/` | 数据访问与事务 |
| `redis/`、`distlock/`、`ratelimit/`、`roundrobin/`、`cronjob/`、`worker/`、`snowflake/` | 缓存、锁、限流与调度 |
| `consul/`、`rpc/`、`kafka/` | 服务发现、RPC 与消息 |
| `token/`、`bc/` | 安全、令牌与密码学 |
| `oss/`、`ai/`、`vectordb/`、`pprof/` | 对象存储、AI 组件（llm / embedding / jev / rag）、向量库与可观测 |
| `errors/`、`shared/`、`utils/`、`testclient/` | 错误、共享模型与工具 |
| `_prompts/` | 指令（commands） / Skill / 知识库与规范文档 |
| `LICENSE` | Apache License 2.0 全文 |

能力职责与落位速查见 [Go架构规范](./_prompts/knowledge/Go架构规范.md) §3.3。

## 中间件推荐适配版本

以下为业务工程对接本库时的**运行侧基础设施推荐适配版本**（非 Go 客户端库版本）。按需启用，未用到的中间件无需部署。

| 中间件 | 推荐适配版本 | 说明 |
|--------|--------------|------|
| MySQL | **8.0.x**（推荐 `8.0.46`） | 默认关系库 |
| Redis | **7.x**（推荐 `7.4.x`） | 缓存、令牌、分布式锁、限流等 |
| Kafka | **3.6.x**（推荐 `3.6.2`） | 可选；事件 / 访问记录等消息场景 |
| Consul | **1.20.x** | 可选；服务注册发现 |
| DTM | **≥ 1.18**（与 `dtm-labs/client` 配套） | 可选；Saga 分布式事务 |
| PostgreSQL | **16.x** | 可选；非默认库，已支持驱动 |
| MinIO | **当前稳定版**（RELEASE 系列） | 可选；对象存储（亦可对接 AWS S3） |
| Milvus | **2.6.x**（与 `milvus/client/v2` v2.6.5 配套） | 可选；向量库（经 `vectordb`，亦可 memory） |

## 许可证

本项目采用 [Apache License 2.0](./LICENSE) 开源许可。
