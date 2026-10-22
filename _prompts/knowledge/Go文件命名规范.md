# Go文件命名规范

---

## 0. 文档信息
- **文档名称**: Go文件命名规范
- **描述**: 定义基于 gitee.com/hq_project/yomi-boot 的业务工程中 BFF 与领域服务各层源文件命名规则，含 Saga Action/Compensate 成对命名。
- **作者**: zwei
- **创建日期**: 2026-06-17

---

## 1. 文档定位

- **用途**：约束 BFF 与领域各层 `.go` 文件的标准命名，确保一致、可读、可检索。
- **边界**：目录结构与分层职责见 [Go架构规范](./Go架构规范.md)、[Go业务分层规范](./Go业务分层规范.md)。
- **适用对象**：`gitee.com/hq_project/yomi-boot/` 本库，以及基于 gitee.com/hq_project/yomi-boot 的业务工程可部署单元内业务与装配相关源文件（惯例目录示例：`services/{unit}/`）。

---

## 2. 适用范围与关联

### 2.1 适用范围

- BFF：`internal/handler/`、`bff/`、`saga/`。
- 领域：`internal/biz/handler/`、`usecase/`（平铺）、`domain/`、`repo/`。
- 装配：各服务 `internal/bootstrap/server.go`。
- `gitee.com/hq_project/yomi-boot/` 包内文件按职责自行命名，须小写+下划线或 Go 惯例。

### 2.2 关联文档

| 文档 | 关联点 |
| ---- | ------ |
| [Go业务分层规范](./Go业务分层规范.md) | 各层职责 |
| [Go架构规范](./Go架构规范.md) | 目录结构 |
| [GoBFF规范](./GoBFF规范.md) | BFF 目录语义 |

---

## 3. 技术方案

### 3.1 命名总则

| 规则 | 要求 | 示例 |
| ---- | ---- | ---- |
| 分隔符 | 下划线 `_` | `get_resource_profile.go`（示例） |
| 字母 | 全部小写 | **示例** `assign_roles.go`（动作名随业务域） |
| 语言 | 英文语义段 | `verify_handler.go` |
| 单复数 | 模块名单数 | `{resource}_handler.go`（示例） |
| 扩展名 | `.go` | — |

### 3.2 BFF handler

目录：`services/{bff}/internal/handler/`

```text
{module}_handler.go
```

- 健康检查统一使用 `gitee.com/hq_project/yomi-boot/component/common/handler.NewHealthHandler(consulPath, opsPath, appCtx)`，分别注册 Consul 探针与运维探针，在 bootstrap 注册。
- 结构体：`{Module}Handler`（**示例**：`ResourceHandler`）。
- 一个模块一个 handler 文件，包含该模块对外全部接口方法。

### 3.3 BFF bff / saga（按 usecase 平铺）

目录：`internal/bff/`、`internal/saga/`

```text
{action}_{scope}.go
```

| 文件（示例） | 说明 |
| ------------ | ---- |
| `get_resource_profile.go` | 只读聚合 |
| `update_resource_status.go` | Saga 编排 |
| `register_resource.go` | Saga 编排 |

- 包名分别为 `bff`、`saga`。
- 一文件一用例；函数名与文件名语义一致（如**示例** `GetResourceProfile`）。

### 3.4 领域 handler

目录：`services/{domain}/internal/biz/handler/`

**普通 API：**

```text
{module}_handler.go
verify_handler.go
```

- 健康检查见 §3.2（gitee.com/hq_project/yomi-boot 公共实现）。

**DTM Saga 成对入口（二选一组织方式）：**

方式 A — 独立文件：

```text
{action}_{scope}_action_handler.go
{action}_{scope}_compensate_handler.go
```

方式 B — 同文件两个方法（仍须独立路由方法名）：

```text
update_status_handler.go   # 内含 UpdateStatusAction、UpdateStatusCompensate
```

- Handler 方法命名：`{Action}{Scope}Action`、`{Action}{Scope}Compensate`。
- **禁止**单一方法内 `switch` 处理 Action 与 Compensate。

### 3.5 领域 usecase（平铺）

目录：`services/{domain}/internal/biz/usecase/`

```text
{action}_{scope}_usecase.go
```

| 文件（示例） | 说明 |
| ------------ | ---- |
| `create_resource_usecase.go` | 含 `CreateResourceUseCase`、`Execute`；Saga 时同文件含 `Compensate` |
| `update_resource_status_usecase.go` | 写用例 + 补偿 |
| `verify_credentials_usecase.go` | 只读，仅 `Execute` |
| `query_roles_by_rel_usecase.go` | 只读 |

- 结构体：`{Action}{Scope}UseCase`。
- **禁止** `usecase/{module}/` 子目录。

### 3.6 Contract / Service / Repository

领域服务内各层命名与目录：

| 层 | 命名 | 目录 |
| -- | ---- | ---- |
| Service 接口 | `{module}_contract.go` | `biz/domain/contract/{module}/` |
| Entity | `{module}_entity.go` | 同上 |
| DTO | `{module}_dto.go` | 同上 |
| Repo 接口 | `{module}_repo_contract.go` | `biz/repo/contract/{module}/` |
| Service 实现 | `{module}_service.go` | `biz/domain/service/{module}/` |
| Repository | `{module}_repo.go` | `biz/repo/db/{module}/` |
| Model | `{module}_model.go` | 同上 |
| Mapper | `{module}_mapper.go` | 同上 |

### 3.7 bootstrap / gitee.com/hq_project/yomi-boot

| 文件 | 位置 | 说明 |
| ---- | ---- | ---- |
| `server.go` | 业务工程 `services/{unit}/internal/bootstrap/`（惯例示例） | 启动装配入口；导出 `Wire(services *plugin.Services) []router.Registrar` 供 `boot.Run` 使用；低层入口可另提供 `NewApp(configPath) *lifecycle.App` |
| `*_consumer_plugin.go` | 同上（按需） | Kafka 消费者自定义插件 |
| `boot.go` | `gitee.com/hq_project/yomi-boot/boot/` | 启动快捷入口与初始化：`Run` / `RunWithOptions` / `Options` / `MustInit` |
| `options.go` | `gitee.com/hq_project/yomi-boot/plugin/` | `PluginOptions`、`AddCustomPlugin`、`AddConfigSection` |
| `plugins.go` / `configs.go` | `gitee.com/hq_project/yomi-boot/plugin/registry/` | 内置插件编排顺序、默认配置段注册表 |
| `server.go` | `gitee.com/hq_project/yomi-boot/boot/server/` | `MustBuildApp`、`serviceName` |
| `middleware.go` | `gitee.com/hq_project/yomi-boot/boot/server/` | `DefaultMiddlewares` |
| `lifecycle.go` / `run.go` | `gitee.com/hq_project/yomi-boot/boot/lifecycle/` | `App`、`Hook`、`Run` / `RunContext`、阶段编排与停机 |
| `plugin.go` / `host.go` / `services.go` / `options.go` | `gitee.com/hq_project/yomi-boot/plugin/` | `Plugin` 接口、`PluginState`、`Runtime`、`Host`、`Services`、`PluginOptions`、`ReadyHook` / `DrainHook` |
| `{组件}_plugin.go` | `gitee.com/hq_project/yomi-boot/plugin/builtin/{lib}/`，如 `plugin/builtin/db/`、`plugin/builtin/consul/` | 各组件生命周期插件实现与 YAML 绑定（包名 `{lib}plugin`；如 `db_plugin.go`、`tx_plugin.go`、`config.go`） |

### 3.8 gitee.com/hq_project/yomi-boot handler

```text
gitee.com/hq_project/yomi-boot/component/common/handler/base_handler.go
```

- 业务 handler 通过 alias 引用：`platformhandler "gitee.com/hq_project/yomi-boot/component/common/handler"`（模块路径以实际 `go.mod` 为准）。

### 3.9 命名对照总表

| 层 | 命名格式 | 目录 |
| -- | -------- | ---- |
| BFF handler | `{module}_handler.go` | `internal/handler/` |
| BFF bff/saga | `{action}_{scope}.go` | `internal/bff/`、`internal/saga/` |
| 领域 handler | `{module}_handler.go` 或 `*_action_handler.go` | `internal/biz/handler/` |
| 领域 usecase | `{action}_{scope}_usecase.go` | `internal/biz/usecase/` |
| Service 接口 | `{module}_contract.go` | `domain/contract/{module}/` |
| Repository | `{module}_repo.go` | `repo/db/{module}/` |

---

## 4. 检查清单

- [ ] 文件名小写+下划线
- [ ] 领域 usecase 平铺于 `biz/usecase/`，无 `{module}/` 子目录
- [ ] BFF `bff/`、`saga/` 按 usecase 平铺命名
- [ ] Saga Handler 与 usecase 的 Action/Compensate 命名成对
- [ ] `internal/bootstrap/` 以 `server.go` 为装配入口；Kafka 消费者按需另增 `*_consumer_plugin.go`
- [ ] 领域 Model 不在 contract 层

---

## 5. 禁止项

- 禁止领域 usecase 使用 `usecase/{module}/` 子包目录。
- 禁止 Handler 文件命名为 `user.go`（缺层语义）。
- 禁止将 Compensate 写到独立 `compensate/` 目录。
- 禁止 `internal/bootstrap/wire/wire.go` 多套一层 `wire/` 目录。
- 禁止拼音或驼峰文件名。
