# Yomi-Boot

Go **基础设施库**（能力库 + 插件生命周期），**不含业务规则**。  
业务工程通过 `go.mod` 标准 `require gitee.com/hq_project/yomi-boot` 依赖本库，按 YAML 配置按需启用能力，可覆盖轻量单体、大单体、分布式微服务等形态。

**当前底座已足够支撑绝大多数企业业务。** 默认按需裁剪（改 YAML `enabled` 等开关即可），不必一上来启用 Consul / Kafka / DTM 等分布式组件。

架构与配置细则见“_prompts”下相关文档，例如
[基础库能力索引](./_prompts/doc/基础库能力索引.md)、
[Go架构规范](./_prompts/doc/knowledge/go/Go架构规范.md)、
[Go配置规范](./_prompts/doc/knowledge/go/Go配置规范.md)。

## 文档与 AI 工作流

| 文档 | 用途                            |
|------|-------------------------------|
| [AI开发工作流说明.md](./AI开发工作流说明.md) | 基于SDD/TDD的AI 工作流说明，主要是对于业务仓使用 |
| [_prompts](./_prompts) | 文档目录                          |
