# yomi-boot

Go **基础设施库**（能力库 + 插件生命周期），**不含业务规则**。  
业务工程通过 `go.mod` 标准 `require gitee.com/hq_project/yomi-boot` 依赖本库，按 YAML 配置按需启用能力，可覆盖轻量单体、模块化大单体、分布式微服务等形态。

**当前底座已足够支撑绝大多数企业业务。** 默认按需裁剪（改 YAML `enabled` 等开关即可），不必一上来启用 Consul / Kafka / DTM 等分布式组件。

可通过 [https://gitee.com/hq_project/yomi-boot-template](https://gitee.com/hq_project/yomi-boot-template) 模板项目快速开始。


## 中间件推荐适配版本

以下为业务工程对接本库时的**运行侧基础设施推荐适配版本**（非 Go 客户端库版本）。按需启用，未用到的中间件无需部署。Docker 镜像 tag 均为已发布的具体版本，不要用 `latest`。

| 中间件 | 推荐适配版本 | Docker 镜像 | 说明 |
|--------|--------------|-------------|------|
| MySQL | **8.4 LTS**（`8.4.11`） | `mysql:8.4.11` | 默认关系库；兼容 8.0 – 9.7，8.0 已过扩展支持（2026-04-30 到期），只建议存量使用 |
| Redis | **7.4**（`7.4.11`） | `redis:7.4.11`（精简镜像 `redis:7.4.11-alpine`） | 缓存、令牌、分布式锁、限流等；兼容 6.2 – 8.x |
| Kafka | **Apache Kafka 4.2.2** | `apache/kafka:4.2.2`（Confluent 版 `confluentinc/cp-kafka:8.2.4`） | 可选；事件 / 访问记录等消息场景；4.x 仅 KRaft（ZooKeeper 已移除），broker 需 Java 17+ |
| Consul | **1.20.x** | — | 可选；服务注册发现 |
| DTM | **≥ 1.18**（与 `dtm-labs/client` 配套） | — | 可选；Saga 分布式事务 |
| PostgreSQL | **17**（实测 `16.15`） | `postgres:16-alpine` | 可选；非默认库，已支持驱动；兼容 12 – 18 |
| MinIO | **存量 RELEASE**（官方社区版已停止分发） | — | 可选；对象存储（亦可对接 AWS S3）。官方社区镜像已下架，存量部署可继续使用，新部署建议选 S3 兼容存储 |
| RustFS | **1.0.1** | `rustfs/rustfs:1.0.1` | 可选；S3 兼容对象存储，经 `oss` 的 `s3` provider + `use_path_style: true` 接入；本地 Docker 验证 S3 兼容性推荐 |
| Milvus | **2.6.x**（与 `milvus/client/v2` v2.6.5 配套） | — | 可选；向量库（经 `ai/vectordb`，亦可 memory） |

表内运行侧版本已按本库当前依赖实测：MySQL `8.4.11` / `9.7.2`、PostgreSQL `16.15` / `17.11`、Redis `7.4.11` / `8.10.2`、Kafka `4.2.2`、Consul `1.20`、RustFS `1.0.1`；DTM / Milvus 按客户端同版本线配套，未做运行实测。

## 许可证

本项目采用 [Apache License 2.0](./LICENSE) 开源许可。
