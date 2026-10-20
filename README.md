# yomi-boot

Go **基础设施库**（能力库 + 插件生命周期），**不含业务规则**。  
业务工程通过 `go.mod` 标准 `require gitee.com/hq_project/yomi-boot` 依赖本库，按 YAML 配置按需启用能力，可覆盖轻量单体、模块化大单体、分布式微服务等形态。

**当前底座已足够支撑绝大多数企业业务。** 默认按需裁剪（改 YAML `enabled` 等开关即可），不必一上来启用 Consul / Kafka / DTM 等分布式组件。

可通过 [https://gitee.com/hq_project/yomi-boot-template](https://gitee.com/hq_project/yomi-boot-template) 模板项目快速开始。


## 中间件推荐适配版本

以下为业务工程对接本库时的**运行侧基础设施推荐适配版本**（非 Go 客户端库版本）。按需启用，未用到的中间件无需部署。Docker 镜像 tag 均为已发布的具体版本，不要用 `latest`。

| 中间件 | 推荐适配版本 | Docker 镜像 |
|--------|--------------|-------------|
| MySQL | **8.4 LTS**（`8.4.11`） | `mysql:8.4.11` |
| Redis | **7.4**（`7.4.11`） | `redis:7.4.11` |
| Kafka | **Apache Kafka 4.2.2** | `apache/kafka:4.2.2`（Confluent 版 `confluentinc/cp-kafka:8.2.4`） |
| Consul | **1.20.x** | — |
| DTM | **≥ 1.18**（配套 `dtm-labs/client` v1.18.7） | — |
| PostgreSQL | **17**（实测 `16.15`） | `postgres:16-alpine` |
| MinIO | **存量 RELEASE**（官方社区版已停止分发） | — |
| RustFS | **1.0.1** | `rustfs/rustfs:1.0.1` |
| Milvus | **2.6.x**（配套 `milvus/client/v2` v2.6.5） | — |


## 许可证

本项目采用 [Apache License 2.0](./LICENSE) 开源许可。
