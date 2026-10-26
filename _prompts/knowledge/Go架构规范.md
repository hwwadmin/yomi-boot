# Go架构规范

---

## 0. 文档信息
- **文档名称**: Go架构规范
- **描述**: 从 gitee.com/hq_project/yomi-boot 架构视角定义基础设施能力、配置裁剪、可部署单元服务类型（入口编排 / 领域 / 可选消费者）与装配约定；业务工程在独立仓库中组织服务，不构成本规范的硬性服务名清单。gitee.com/hq_project/yomi-boot 可按配置支撑轻量单体、大单体、分布式微服务等形态。
- **作者**: zwei
- **创建日期**: 2026-06-17

---

## 1. 文档定位

- **用途**：说明 gitee.com/hq_project/yomi-boot 的能力边界、部署形态无关性与可部署单元服务类型；约定基于 gitee.com/hq_project/yomi-boot 的进程装配、中间件链与 HTTP 路由模式。
- **边界**：BFF/入口编排细则见 [GoBFF规范](./GoBFF规范.md)；领域分层见 [Go业务分层规范](./Go业务分层规范.md)；本地事务与 DTM Saga 见 [Go事务规范](./Go事务规范.md)；微服务间契约见 [微服务调用规范](./微服务调用规范.md)（**信封与幂等权威**）；**文件命名**见 [Go文件命名规范](./Go文件命名规范.md)；**数据访问**见 [Go数据库规范](./Go数据库规范.md)；**配置加载、扩展与能力裁剪**见 [Go配置规范](./Go配置规范.md)。
- **适用对象**：gitee.com/hq_project/yomi-boot 库本身，以及基于 gitee.com/hq_project/yomi-boot 的业务工程与可部署单元。业务工程为独立仓库，通过标准 `require gitee.com/hq_project/yomi-boot` 依赖本库；本库仓库内不包含业务服务代码。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于 `gitee.com/hq_project/yomi-boot/`：可复用基础设施库；各能力插件统一编排，**是否真正启动以配置段 `enabled`（及同类开关）为准**，可按需裁剪。
- 适用于基于 gitee.com/hq_project/yomi-boot 的多种部署形态（详见 §3.1.0、§3.4.5）：
  - **轻量单体**：单进程、单库；通常只开 `db`（及按需 `redis`/`token`），关闭 `consul`/`kafka`/`redis_stream`/`dtm`/`event` 等。
  - **大单体 / 模块化单体**：单部署单元内多业务模块分层；仍用同一套 boot/插件，按模块边界组织代码，不必拆多进程。
  - **分布式微服务**：多进程按服务类型独立部署；开 `consul`/`rpc`，跨域写按需开 `dtm`，异步按需开 `kafka` / `redis_stream` / `event`。
- 适用于基于 gitee.com/hq_project/yomi-boot 组装的业务工程（独立 `go.mod`，经标准 `require` 引用 `gitee.com/hq_project/yomi-boot`；本地可用 `go.work` 联调）：具体服务名与业务域随产品演进，**以服务类型为准，不以某一仓库清单为准**。
- Consul / HTTP RPC / DTM 等为**分布式路径**能力，非 gitee.com/hq_project/yomi-boot 强制依赖。
- 不适用于前端工程（前端规范仅作消费者参考，本库仓无前端代码）。

### 2.2 关联文档

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| `_prompts/knowledge/GoBFF规范.md` | 入口编排 | 跨域调用、Body 信封、Saga、令牌注入 |
| `_prompts/knowledge/Go业务分层规范.md` | 业务分层 | 入口编排与领域各层职责 |
| `_prompts/knowledge/Go选型规范.md` | 技术栈基线 | gin、gorm、Consul、DTM、HTTP 通信选型 |
| `_prompts/knowledge/Go事务规范.md` | 事务 | 本地子事务与 DTM Saga |
| `_prompts/knowledge/微服务调用规范.md` | 微服务契约 | POST/JSON、Header 链路 ID、Body 信封 bizId/bizSubId |
| `_prompts/knowledge/API编码适配规范.md` | 对外 API | Encoder 显式注册与写出适配；入口编排层可用 GET/POST |
| `_prompts/knowledge/Go数据库规范.md` | 数据访问 | repository 与 GORM 规范 |
| `_prompts/knowledge/Go配置规范.md` | 配置 | Section 注册、严格根 key、业务扩展、**能力按需裁剪（enabled）** |
| `_prompts/knowledge/Redis设计规范.md` | Redis | 令牌 Store 键设计 |
| `_prompts/knowledge/Go注释规范.md` | 注释 | 代码注释口径 |

### 2.3 阅读建议

- 先理解 gitee.com/hq_project/yomi-boot 能力边界：读 §3.1.0（部署形态无关 / 配置裁剪）与 [Go配置规范](./Go配置规范.md) 能力裁剪章节。
- 理解可部署服务类型：读 §3.1.2～§3.1.4；入口编排再读 [GoBFF规范](./GoBFF规范.md)；领域再读 [Go业务分层规范](./Go业务分层规范.md)。
- 装配与路由：读 §3.3 gitee.com/hq_project/yomi-boot 职责、§3.4 启动装配、§3.4.5 按需启用、§3.6 路由与中间件。
- 建立门禁：读 §4 检查清单与 §5 禁止项（按服务类型与形态裁剪适用）。

---

## 3. 技术方案

### 3.1 核心设计原则

#### 3.1.0 gitee.com/hq_project/yomi-boot 部署形态无关（配置裁剪）

- gitee.com/hq_project/yomi-boot 是**能力库 + 插件生命周期**，不是「必须微服务」的运行时。
- 插件在 `plugin/builtin` 根包中按固定顺序登记；**Init 是否真正启动由各自配置决定**（见 `plugin/builtin/plugins.go` 与 `plugin/builtin/{lib}`）。
- 切换形态优先改 YAML，而不是改库代码：关未用能力的 `enabled`，需要时再打开。细则见 [Go配置规范](./Go配置规范.md)「能力按需裁剪」与本文 §3.4.5。
- 业务侧如何拆进程、拆几个可部署单元，由产品与团队决定；多服务拆分是业务仓的组织方式，不是 gitee.com/hq_project/yomi-boot 的唯一用法。
- 单体形态下：可无 Consul/DTM/Kafka/Redis Stream；业务可仍按 handler → usecase → domain → repo 分层；跨模块调用走进程内依赖，不必强上入口编排层 / Saga。

#### 3.1.1 gitee.com/hq_project/yomi-boot + 业务工程

- 基础设施库为独立 module：`gitee.com/hq_project/yomi-boot`（本仓库根）；**不含业务规则**；能力是否启用由配置裁剪（§3.1.0）。
- 业务工程在独立仓库中：标准 `require gitee.com/hq_project/yomi-boot <tag>`（无需 `replace`）；本地联调可用相对路径 `replace` 或 `go.work`。
- 业务工程可聚合：可选的 **client 契约模块**（跨进程 HTTP DTO/客户端），以及一个或多个**可部署单元**（各自 `go.mod`、`cmd/main.go`、`internal/bootstrap/`、`config/`）。
- 分布式形态下：进程间通信统一 **HTTP/HTTPS**；注册发现可用 **Consul**；跨域写事务可用 **DTM Saga**（由入口编排层提交）。单体形态可不启用上述组件。
- client 契约模块依赖 gitee.com/hq_project/yomi-boot 的 Consul/RPC/DTM 等能力，供入口编排层调用下游；领域进程不反向依赖其他领域的业务包。

#### 3.1.2 可部署单元服务类型（抽象）

规范约束的是**服务类型**，不是具体服务名。具体服务名与目录落在业务工程中。

| 服务类型 | 职责 | gitee.com/hq_project/yomi-boot 典型启用 |
| -------- | ---- | ---------------- |
| **入口编排（BFF）** | 对外 API、鉴权、跨域读聚合、跨域写编排（Saga） | `token`、按需 `db`/`redis`/`consul`/`dtm`/`kafka`/`redis_stream`/`event` |
| **领域** | 单一业务边界内的读写与本地事务；对外提供内部 API | `db`、按需 `redis`（幂等/锁）、`consul`；通常关 `token` |
| **基础设施消费者（可选）** | 消费平台/异步消息并持久化，或提供对内查询 | `kafka` 或 `redisstream` + `AddCustomPlugin`；按需 `db` |

- 同一进程可合并多种服务类型（单体）：例如入口 + 多领域模块共进程，跨模块走内存依赖。
- 分布式形态下通常一进程一主服务类型；具体叫什么、拆几个，由业务决定。

#### 3.1.3 分布式形态下的调用边界

- **领域进程之间禁止 HTTP 互调**。
- **跨域 HTTP 仅由入口编排层**经 client 契约模块发起。
- 单体形态下跨模块调用走进程内依赖，不适用本条 HTTP 互调禁令。

#### 3.1.4 gitee.com/hq_project/yomi-boot 只放基础设施

- 仓库按三层组织：`component/`（可复用库，其中 `component/core` 为内核）、`plugin/`（插件契约、内置插件、插件清单与部署 YAML 绑定）、`boot/`（启动编排：初始化、server HTTP 装配、lifecycle、banner）。
- `component/core/` 提供 appctx、bc、config、cors、errors、handler、httpguard、logger、masking、response、router、shared、traceid、utils，是内核（无业务状态、不管理外部资源；`response` 仅保留进程级 Encoder 注册）；`component/common/` 提供 snowflake、db、redis、kafka、redisstream、httprecord、token、redisratelimit、oss、pprof、idempotency、event、distlock、roundrobin、cronjob、worker、ws、testclient 等通用组件；`component/cloud/` 提供 consul、dtm、rpc；`component/ai/` 提供 llm、embedding、jev、chunking、retrieval、rag、vectordb、rerank。**上述能力按配置按需启用**，并非每个进程都必须全部启动；appctx、masking、logger、snowflake 以及 HTTP 服务本身属于强制启动项。`component/ai/*` 与 `ws` 不进默认插件链。
- `plugin/` 提供 `Plugin`、`Base`、`ErrDisabled`、`PluginSlot`、`PluginOptions`（自定义插件 / 自定义配置段）、`ReadyHook` / `DrainHook`；`PluginSlot` 除 `Config` 外只保存有序插件链与每个插件的初始化状态，插件对象自包含其创建的组件，跨插件依赖经 `slot.Plugin` / `slot.MustPlugin` 取到前序插件后调用其访问器。内置插件与各能力的 YAML 绑定统一放在 `plugin/builtin/{lib}/`（`SectionKey`、`BootConfig`、`ConfigSection`、`ToConfig`）；内置插件启动顺序与默认配置段集中在 `plugin/builtin` 根包（`DefaultPlugins()`、`DefaultConfigs()`）。`boot/` 提供启动入口 `boot`（含 `MustInit` 初始化）、server（HTTP 装配）、lifecycle、banner。
- `component/**` 不得依赖 `boot/**` 或 `plugin/**`；`plugin/**` 可依赖 `component/**`，但不得依赖 `boot/**`；`plugin` 根包只依赖 `component/core` 的 `config` / `errors` / `logger`，不得 import `plugin/builtin/**`（子包依赖 `plugin`，会成环），内置清单放 `plugin/builtin` 根包（根包可 import 全部子包，`plugin/builtin/{lib}` 子包不得 import 根包，否则成环）；`plugin/builtin/{lib}` 之间按依赖 DAG 单向 import（如 token → redis、event → db），不得成环；`component/core` 不得依赖任何兄弟组件。组件库只保留运行时 `Config`，不写 `mapstructure` 标签。`cors` / `router` 的 `BootConfig` 分别在 `plugin/builtin/cors`、`plugin/builtin/router`（仅绑定，无生命周期插件实现；`cors.enabled` 默认 true，`router` 不提供 `enabled`）；`oss` 的绑定与生命周期插件都在 `plugin/builtin/oss`（`oss.enabled=true` 时构造 `Storage` 并由插件自包含，业务经 `From(slot).Storage()` 读取）。
- `gitee.com/hq_project/yomi-boot` 禁止反向依赖任何业务可部署单元代码。
- `gitee.com/hq_project/yomi-boot/component/common/token` 提供通用令牌库（Store、Claims、中间件骨架）与 `TokenService`；需要鉴权的进程（入口编排或单体入口）直接注入使用。
- `gitee.com/hq_project/yomi-boot/component/common/redisratelimit` 提供调用式限流门面（`Allow`/`AllowN`），依赖 `RedisService`，**不进入默认中间件链**；场景配额由调用方传入 `Rule`。

### 3.2 标准目录结构（模式）

以下为**业务工程**推荐落位模式（与 gitee.com/hq_project/yomi-boot 分仓）；`{bff}`、`{domain}` 为占位符。gitee.com/hq_project/yomi-boot 本仓根目录即为 `module gitee.com/hq_project/yomi-boot`。

```text
# gitee.com/hq_project/yomi-boot 仓库（本库）
gitee.com/hq_project/yomi-boot/                               # module: gitee.com/hq_project/yomi-boot
├── boot/                        # 启动编排（入口/初始化、server、lifecycle、banner）
├── component/                   # 可复用库
│   ├── core/                    # 内核（appctx、bc、config、cors、errors、handler、httpguard、logger、response、router、shared、traceid、utils）
│   ├── common/                  # 通用组件（router、db、redis、kafka、event 等）
│   ├── cloud/                   # 分布式组件（consul、dtm、rpc）
│   └── ai/                      # AI 能力组件（不进插件链）
└── plugin/                      # 插件契约、内置插件、插件清单与部署 YAML 绑定
    └── builtin/                 # 根包：内置插件清单（DefaultPlugins/DefaultConfigs）；子目录：内置插件与 YAML 绑定（appctx、logger、db、redis、kafka、event、cors、router、oss 等）

# 业务工程仓库（示例）
{business-repo}/
├── go.work                            # 可选；本地联调 use ./services/* 与 ../gitee.com/hq_project/yomi-boot
├── client/                            # 可选；跨进程契约 module
│   └── {domain}/
└── services/
    ├── {bff}/                         # 入口编排服务类型
    └── {domain}/                      # 领域服务类型（可多个）
```

各业务服务 `go.mod`：`require gitee.com/hq_project/yomi-boot <tag>`（标准拉取，无需 `replace`）。**按服务类型套模板，不要求对齐某一固定服务名清单。**

#### 3.2.1 入口编排（BFF）目录模式

```text
services/{bff}/
├── go.mod
├── cmd/main.go
├── config/local.yml, dev.yml, prod.yml
├── test/
└── internal/
    ├── handler/                       # 对外 HTTP
    ├── bff/                           # 只读聚合
    ├── saga/                          # 跨域写 DTM Saga
    ├── bizerr/
    └── bootstrap/                     # server.go
```

#### 3.2.2 领域目录模式

```text
services/{domain}/
├── go.mod
├── cmd/main.go
├── config/local.yml, dev.yml, prod.yml
├── script/sql/
├── test/
└── internal/
    ├── bootstrap/                     # server.go；消息消费另增 *_consumer_plugin.go
    └── biz/
        ├── handler/
        ├── usecase/
        ├── domain/contract/{bound}/, domain/service/{bound}/
        ├── repo/contract/{bound}/, repo/db/{bound}/
        └── bizerr/
```

#### 3.2.3 带消息消费者的领域变体

在 §3.2.2 基础上，于 `internal/bootstrap/` 增加自定义消费者插件（经 `AddCustomPlugin` 注册）。gitee.com/hq_project/yomi-boot 只提供消费端点能力（如 `httprecord.StartKafkaRecordConsumer` 或 `RedisStreamConsumer.Start`），**业务消费逻辑留在可部署单元内**。

### 3.3 gitee.com/hq_project/yomi-boot 模块职责

归属速查：

| 目录 | 包 |
| ---- | -- |
| `component/core/` | `appctx`、`bc`、`config`、`cors`、`errors`、`handler`、`httpguard`、`logger`、`masking`、`response`、`router`、`shared`、`traceid`、`utils` |
| `boot/` | `boot`（启动快捷入口与初始化）、`server`（HTTP 装配）、`lifecycle`、`banner` |
| `plugin/` | `Plugin` 接口、`Base`、`ErrDisabled`、`PluginSlot`、`PluginOptions`、`ReadyHook` / `DrainHook` |
| `plugin/builtin/` | 根包：内置插件启动顺序 `DefaultPlugins()`、默认配置段 `DefaultConfigs()`；子目录：内置插件与各能力 YAML 绑定（`appctx`、`masking`、`logger`、`db`、`redis`、`kafka`、`redisstream`、`event` 等；包名 `{lib}plugin`） |
| `component/common/` | `snowflake`、`db`、`redis`、`kafka`、`redisstream`、`distlock`、`event`、`httprecord`、`idempotency`、`oss`、`pprof`、`redisratelimit`、`roundrobin`、`cronjob`、`token`、`worker`、`ws`、`testclient` |
| `component/cloud/` | `consul`、`dtm`、`rpc`（HTTP 传输） |
| `component/ai/` | `llm`、`embedding`、`jev`、`chunking`、`retrieval`、`rag`、`vectordb`、`rerank`、`generation`、`tool` |

依赖方向：`component/core` 不依赖 `boot`/`plugin`/任何兄弟组件；`component/common` 依赖 `component/core`；`component/cloud`、`component/ai` 依赖 `component/core` + `component/common`；`component/ai` 内部 `retrieval` 依赖 `embedding` + `vectordb`，`rag` 依赖 `chunking`/`embedding`/`retrieval`/`rerank`/`generation`/`vectordb`，`llm`/`embedding`/`jev`/`chunking`/`rerank`/`generation`/`tool` 互不依赖；`component/common` 不依赖 `component/cloud`/`component/ai`，`component/cloud` 与 `component/ai` 互不依赖；`component/**` 不依赖 `boot/**` 或 `plugin/**`；`plugin/**` 可依赖 `component/**`，但不得依赖 `boot/**`；`plugin` 根包只依赖 `component/core` 的 `config` / `errors` / `logger`，不得 import `plugin/builtin/**`，`plugin/builtin` 根包可 import 全部子包（子包不得 import 根包）；`plugin/builtin/{lib}` 之间按依赖 DAG 单向 import；`boot` 依赖 `component/core` + `component/common` + `plugin` + `plugin/builtin` + `boot/server`；其中 `MustInit` 只负责配置加载与插件链，不涉及 HTTP 装配；`boot/server` 依赖 `boot/lifecycle`、gin、中间件与 `plugin/builtin` 的配置段与访问器。

| 包 | 职责 |
| -- | ---- |
| `boot` | `MustInit(ctx, Options)`、`Run` / `RunWithOptions`、`Options`、`PluginSet`；`MustInit` 返回 `*plugin.PluginSlot`，配置经 `slot.Config` + `MustSection` 读取；默认插件顺序由 `plugin/builtin` 根包提供，插件扩展经 `plugin.PluginOptions`（`AddConfigSection` / `AddCustomPlugin`）；`Options.PluginSet` 非 nil 时完全使用其 `Plugins` / `Configs`，不再追加内置链与默认配置段，`PluginOptions` 自定义项被忽略；`Run` / `RunWithOptions` 编排响应编码器注册（`Options.ResponseEncoder`，为空时 `DefaultEncoder`）、初始化、wire、HTTP 装配与 lifecycle |
| `server` | `MustBuildApp`、`DefaultMiddlewares`；只处理 HTTP 侧（gin 引擎、中间件链、`http.Server`、`lifecycle.App`），不感知具体组件 |
| `plugin` | 资源生命周期框架：`Plugin` 接口（`Init(ctx) error` / `Close(ctx) error`）、`Base`、`ErrDisabled`、`PluginSlot`（Config + 有序插件链，负责 Init / 失败回滚 / hook 收集 / 逆序 Close / 错误聚合）、`PluginOptions`（自定义插件 / 自定义配置段）；`ReadyHook` / `DrainHook` 可选阶段契约 |
| `plugin/builtin`（根包） | 内置插件启动顺序（`DefaultPlugins()`）与默认配置段（`DefaultConfigs()`）；根包可 import 全部子包，子包不得 import 根包；`plugin` 根包不得 import 本包 |
| `plugin/builtin/{lib}` | 各能力的内置生命周期插件实现与 YAML 绑定：`SectionKey`、`BootConfig`（`mapstructure`）、`ConfigSection()`、`ToConfig()`；插件导出具体类型、`Name` 常量、`From` 与组件访问器，自包含其创建的组件；`cors`/`router` 仅提供 YAML 绑定；`tx` 挂在 `plugin/builtin/db`；包名 `{lib}plugin` |
| `component/core` | 内核（无业务状态、不管理外部资源）；不得 import `boot`、`plugin` 或任何兄弟组件；提供 `appctx`、`bc`、`config`、`cors`、`errors`、`handler`、`httpguard`、`logger`、`masking`、`response`、`router`、`shared`、`traceid`、`utils` |
| `consul` | 服务注册、注销、发现、HTTP URL 解析 |
| `dtm` | Saga 客户端、GORM barrier、`TxRunner`（实现 `tx.Runner`，由 `plugin/builtin/dtm` 在 db 可用时构造）、`BindAndHandleAction/Compensate` |
| `traceid` | BFF 入站中间件生成/读取 `X-Trace-ID` Header；出站 RPC 经 Header 传播 |
| `pprof` | 独立 debug 端口暴露 `net/http/pprof`；由 `plugin/builtin/pprof` 在 `MustInit` 阶段启动；进程退出时自然关闭；默认关闭 |
| `token` | Store、Claims、中间件骨架、`TokenService`（签发/校验/撤销） |
| `db` | 数据源：配置校验、DSN、连接池、最小空闲预热与保活、`Conn` 生命周期；子包 `db/orm`（`BaseModel`、`BaseRepository`、分页/LIKE/SQL 标识符校验）、`db/tx`（`Runner`/`Executor` 契约、`LocalRunner` 实现、Context 事务传递）、`db/dbtest` |
| `idempotency` | `sys_biz` 模型、仓储、`Guard`、`Execute`；三层幂等（Redis + 锁 + DB）；细则见 [微服务调用规范](./微服务调用规范.md) §3.4 |
| `event` | 可选 Outbox/Inbox 事件组件（`outbox_enabled` / `inbox_enabled` 任一为 true 时由插件启动）；发布侧 `eventId` 必须全局唯一（建议雪花），消费侧按 `(service_name, event_id)` 幂等 |
| `router` | `Registrar` 接口与路由注册 |
| `handler` | `BindAndHandle` 等协议适配 |
| `shared` | 跨服务通用 dto（含微服务请求/响应信封）、enums、常量 |
| `utils` | 通用辅助函数收口（`ginutil`、`ptrutil`、`timeutil`、`validateutil` 等子包） |
| `redis` | `RedisService` 接口与 `RedisHelper` 实现（含 `OpsRateLimit4Allow` 滑动窗口限流原语） |
| `redisratelimit` | 调用式限流门面 `LimitService`（`Allow`/`AllowN`）；依赖 `RedisService`，不进默认中间件链 |
| `bc` | 密码学基础设施能力簇（Bootstrap Crypto）；无状态按需 import，不挂 plugin 插件链 |
| `bc/passwd` | 密码单向哈希/校验（bcrypt，≤72 字节） |
| `bc/random` | 密码学安全随机（Bytes/Hex/URLSafe） |
| `bc/hash` | MD5 / SHA-1（遗留兼容）/ SHA-256 / SHA-512 摘要（禁止用于密码） |
| `bc/sm3` | SM3 摘要（国密；禁止用于密码） |
| `bc/hmacsha` | HMAC-SHA1（遗留）/ HMAC-SHA256 / HMAC-SHA512 消息认证 |
| `bc/aes` | AES-GCM（推荐，支持 AAD）/ CBC；ECB 不推荐 |
| `bc/sm4` | SM4-GCM（推荐，支持 AAD）/ CBC；ECB 不推荐（国密） |
| `bc/rsa` | RSA PEM（PKCS#1/PKCS#8、PKIX/PKCS#1 公钥、证书链、带密码私钥）、OAEP 加解密 |
| `bc/sha256withrsa` | SHA256withRSA（PKCS#1 v1.5 + SHA-256）签名验签；对应 Java、支付宝 RSA2、微信支付 v3 |
| `bc/rsapss` | RSASSA-PSS + SHA-256 签名验签（SaltLength=HashLen） |
| `bc/ecdsap256` | ECDSA P-256 + SHA-256 签名验签与密钥 PEM；ASN.1 DER（JWT ES256） |
| `bc/ecdsap384` | ECDSA P-384 + SHA-384 签名验签与密钥 PEM；ASN.1 DER（JWT ES384） |
| `bc/ed25519` | Ed25519 签名验签与密钥 PEM；64 字节原始签名（JWT EdDSA） |
| `bc/sm2` | SM2 PEM、C1C3C2/ASN.1 加解密、ASN.1 签名验签（国密） |
| `kafka` | 生产者 `NewPublisher` / `PublisherService`、消费者 `NewConsumerManager` / `ConsumerService`，两端独立构造、校验与关闭；`kafkatest` 提供独立测试入口 |
| `redisstream` | Redis Stream 发布/消费组（`PublisherService` / `ConsumerService`）；复用 `RedisClient`，不耦合 `RedisService` |
| `httprecord` | HTTP 访问记录中间件、Kafka 发布端；`StartKafkaRecordConsumer` / `MustStartKafkaRecordConsumer` 提供消费端点（反序列化 + 委托 handler），**不含**业务消费逻辑；记录内容脱敏经注入的 `masking` 服务（`http_record.mask_sensitive` 控制，默认 true） |
| `rpc` | HTTP RPC 读传输（EndpointResolver 端点解析 + POST JSON + 反序列化）；轻量重试（退避）+ 按 HTTP 状态码/尝试耗时熔断；不解析业务信封，不区分分页；跨域写走 DTM，不在此叠加写重试 |
| `ws` | 独立端口 WebSocket（coder/websocket）：Config 配 Processor/可选 Handler；默认 Handler 消费消息；ConnID 仅服务端；心跳/寿命/`MaxConnections`；可选 `MaxFramePayloadSize` 协议分片；无 yml/默认插件；小文件可走 WS，大文件走 `oss` |
| `distlock` | 基于 Redsync 的分布式锁、看门狗 |
| `errors` | 系统级错误包装与 `BizError` 业务异常类型 |
| `response` | `Encoder` 接口与进程级注册；`boot.Options.ResponseEncoder` 指定实现，为空时 `boot` 默认注册 `DefaultEncoder`；低层启动路径仍可显式 `MustRegister`（预制 ≠ 强制契约） |
| `shared/error` | 跨库通用哨兵（`NewNoStack`；返回须 `errors.Sentinel`） |
| `roundrobin` | 分布式定时轮询调度（event outbox 等内部使用） |
| `cronjob` | 业务日历定时任务（cron 表达式 + distlock 互斥）；禁止业务直引 `robfig/cron`；内部轮询仍用 `roundrobin` |
| `snowflake` | 雪花 ID 生成器 |
| `appctx` | 应用上下文（名称、环境、版本） |
| `banner` | 启动横幅打印 |
| `lifecycle` | HTTP 服务进程生命周期编排：`App`、`Run` / `RunContext`；只负责阶段顺序、信号与优雅停机，不感知具体组件 |
| `cors` | 跨域中间件 |
| `logger` | 日志初始化与配置；`MustInit` Fail Fast；字段/正文脱敏经注入的 `masking` 服务（`log.mask_sensitive` 控制，默认 true） |
| `masking` | 统一脱敏服务：字段名匹配（contains/exact）+ 策略应用（redact/keep_edges/phone/idcard/bankcard/email/name + 业务注册自定义），logger / httprecord / 业务出参共用；规则唯一来源是 `masking` 段配置，引擎零内置规则；安全不变量：命中即变、形状不符降级 placeholder；`masking.MustRegisterStrategy` 须在 boot.Run 前注册；YAML 绑定与插件在 `plugin/builtin/masking`，业务经 `maskingplugin.From(slot).Service()` 或 `masking.Current()` 调用 |
| `config` | 配置加载抽象（Section/Registry/Config）、严格根 key、环境变量展开；`LoadConfig` / `MustLoadConfig` Fail Fast；详见 [Go配置规范](./Go配置规范.md) |
| `worker` | 后台 worker 工具 |
| `testclient` | 集成测试 HTTP 客户端辅助 |
| `component/ai/llm` | 统一大模型对话门面：Chat / 流式（OpenAI / Anthropic）；`New`/`MustNew`；一实例一模型；独立哨兵 `AI_LLM_*`；不进插件链、不读 YAML、不做限流 |
| `component/ai/embedding` | 统一文本向量化门面：OpenAI Embeddings；`New`/`MustNew`；APIKey 可空；`Request.Dimensions` 每次调用必填（返回向量按它校验），`SendDimensions` 控制是否下发上游 dimensions 参数（不支持该参数的网关会 400）；独立哨兵 `AI_EMBEDDING_*`；与 llm / jev 零依赖；不进插件链、不读 YAML、不做限流 |
| `component/ai/jev` | 统一 Jev 结构化决策门面：System One；`New`/`MustNew`；无流式；独立哨兵 `AI_JEV_*`；与 llm / embedding 零依赖；不进插件链、不读 YAML、不做限流 |
| `component/ai/chunking` | 独立文本切块策略库：内置 recursive / fixed / sentence / markdown，可自定义 `Chunker`；`New` / `ChunkText`；`Normalize` 包装自定义实现，包装后的 `Chunk` 自带输出契约校验（空白块丢弃、超长块拒绝）；块长按 rune 计，单块上限 `MaxChunkSize`；独立哨兵 `AI_CHUNKING_*`；与 ai 其他组件零依赖；不进插件链、不读 YAML、不做限流 |
| `component/ai/retrieval` | 通用检索门面：查询向量化 + 检索策略 hybrid（默认，向量 + BM25 混合）/ vector（RRF 融合）+ 三层过滤（前置元数据 `Filter` 下推 / 阈值 `MinScore` / 后置 `HitFilter` 扩展点经 `RetrieveRequest.PostFilter` 注入，内置文档版本过滤器 `NewDocumentFilter`（元数据键可配置，旧块治理 + 可选内容完整性））；`New`/`MustNew`；组合 `component/ai/embedding` + `component/ai/vectordb`，不认识业务元数据语义；独立哨兵 `AI_RETRIEVAL_*`；不进插件链、不读 YAML、不做限流 |
| `component/ai/rag` | RAG 流程编排库：门面 `Engine`（`Index` 文档入库 / `Retrieve` 文档检索增强，由 `New`/`MustNew` 返回），串联 5 个门面组件（`chunking.Chunker` / `embedding.Client` / `retrieval.Retriever` / `rerank.Reranker` / `generation.Generator`）与 `vectordb.Store`，各依赖经 Config 全部注入；rag 维护块主键、保留元数据键与文档版本；集合维度按 `IndexRequest.Dimensions` 创建，每篇文档经 `vectordb.Store.ReplaceByFilter` 覆盖并清理旧块，检索经 retrieval 的文档版本过滤器过滤旧块；文档删除与集合管理使用 `vectordb.Store`；同一 Engine 可服务多个集合，集合名按调用传入；哨兵 `AI_RAG_INVALID_ARGUMENT` / `AI_RAG_BAD_RESPONSE`，组件错误原样透传；不进插件链、不读 YAML、不做限流 |
| `component/ai/vectordb` | 统一向量库门面：milvus / memory；`New`/`MustNew`；Metric 锁在 Store，向量维度按集合传入并锁定在集合上（不同集合可不同，维度一致性由 Store 校验）；动态 Create/Drop/Replace 集合与 `EnsureCollection`（带维度）；`ReplaceByFilter` 按过滤键覆盖写入并清理旧记录（同键实例内串行）；Search 按 `SearchQuery.Text` 切换纯向量 / 向量+BM25 混合检索（RRF 融合；milvus 原生全文检索要求 2.5+，memory 自带 BM25）；不进插件链、不读 YAML |
| `component/ai/rerank` | 通用精重排门面契约：`Reranker` 接口 + 请求/结果类型，无 Engine 中间层、不含模型接入；业务实现接口接入自有模型，`ValidateRequest` / `RankScores` / `Apply` 供实现与调用方复用（契约的可执行形式），内置 PassThrough（保序返回 TOP-N）占位；单次候选上限 `MaxDocs`；独立哨兵 `AI_RERANK_*`；与 ai 其他组件零依赖；不进插件链、不读 YAML、不做限流 |
| `component/ai/generation` | 提示词拼接门面契约：`Generator` 接口（把用户问题与检索到的文档块拼接为最终输入提示词）+ 请求/结果类型，无 Engine 中间层、不含提示词模板与大模型接入；业务实现接口定制模板（拼接结果可配合 `component/ai/llm` 发送），`ValidateRequest` / `ValidateResponse` 供实现与调用方复用（契约的可执行形式），内置 Basic（问题 + 编号参考资料固定模板）占位；独立哨兵 `AI_GENERATION_*`；与 ai 其他组件零依赖；不进插件链、不读 YAML、不做限流 |
| `component/ai/tool` | 协议无关的工具调用组件：`Spec`（原生 JSON Schema + `Fill` 填槽规则 + 宿主 Metadata）/ `Tool` / `Registry` / `Call` / `Result`；`NewFunc` / `NewTyped` 适配 simple tool，`NewResolved` + `InputContract` 提供 `Decode`/`Normalize`/`Validate`/`Identify` typed 入参；`Registry.Fill` 按 `Completer` 端口拼 Schema/Guidance/Example/Evidence 提示词、抽取 JSON、按契约校验并返回 `Filled`（Arguments/Input/Missing/Complete/Raw），缺槽不重试、非法 JSON/类型错误可配置重试，`IdentifierKeys` 做证据门闩；`Invoke` / `InvokeFilled` / `FillAndInvoke` / `InvokeAll` 执行，参数只做对象形态与运行时必填校验，执行层统一 panic 恢复、参数/结果上限（默认 8 MiB，可配置）、防御性拷贝与批量固定 worker（默认顺序、不 fail-fast）；对外错误统一为 `*Error`（Code/Message/Tool/CallID/Missing，不实现 Unwrap，底层原因只经 Cause 供进程内日志），工具可实现 `SafeError` 提供安全文案；`AI_TOOL_*` 独立错误码；与 ai 其他组件零依赖，不 import llm；不进插件链、不读 YAML、不做限流 |
| `oss` | 统一对象存储抽象（RustFS / AWS S3 / 阿里云 OSS 等）：大文件上传/下载；**勿**用 `ws` 整包传大文件；YAML 绑定与插件在 `plugin/builtin/oss`，`oss.enabled=true` 时构造 `Storage` 并由插件自包含，业务经 `ossplugin.From(slot).Storage()` 读取 |

**平台共享 DDL**（统一维护在根目录 `gitee.com/hq_project/yomi-boot/sql/` 下，按数据库分 `mysql/` 与 `pgsql/` 两个目录，同名文件字段口径一致，修改时须同步两份）：

| 文件 | 表名 | 说明 |
| ---- | ---- | ---- |
| `gitee.com/hq_project/yomi-boot/sql/mysql/sys_biz.sql` | `sys_biz` | 业务幂等表 |
| `gitee.com/hq_project/yomi-boot/sql/mysql/barrier.sql` | `sys_barrier` | DTM 子事务屏障表 |
| `gitee.com/hq_project/yomi-boot/sql/mysql/sys_event.sql` | Outbox/Inbox 相关 | 可选，`event.outbox_enabled` / `event.inbox_enabled` 启用时使用 |

**gitee.com/hq_project/yomi-boot 明确不包含**：业务 handler/usecase、各领域 `bizerr` 哨兵。

### 3.4 启动与装配

#### 3.4.1 Fail Fast 约定

启动 / 运行 / 关闭三阶段异常策略以 [Go异常规范](./Go异常规范.md) §3.5 为准。本文仅强调装配侧：

- 配置加载、插件 `Init`、HTTP 装配失败走启动 Fail Fast（log + panic）；插件 `Init` 失败时由 `PluginSlot` 逆序回滚已初始化插件，再由 `boot.MustInit` panic。
- 各 gitee.com/hq_project/yomi-boot 库对外提供 `Must*` 包装（如 `config.MustLoadConfig`）；构造函数内部假定入参已校验。
- HTTP 请求路径的 `error` 传播与 Recovery 见 Go异常规范。

#### 3.4.2 启动链路

```text
cmd/main.go
  └─ boot.Run(envPrefix, bootstrap.Wire)                  # 推荐入口：默认配置路径 + 默认中间件链
       └─ boot.RunWithOptions(bootOpts, wire)
            └─ lifecycle.Run(factory)
                 ├─ boot.MustInit(ctx, bootOpts)
                 │     ├─ PluginSet 为 nil：config.NewRegistry + builtin.DefaultConfigs + bootOpts.Plugin.CustomConfigs() + MustLoadConfig
                 │     ├─ PluginSet 非 nil：仅注册 PluginSet.Configs + MustLoadConfig
                 │     └─ plugin.NewPluginSlot(cfg, plugins); slot.Init(ctx)  # 默认链追加 CustomPlugins；PluginSet 模式完全使用 Plugins；未启用插件返回 ErrDisabled
                 ├─ wire(slot) → []router.Registrar                    # 手写 wiring，在 internal/bootstrap
                 └─ server.MustBuildApp(slot, registrars, middlewares) → *lifecycle.App
```

标准入口是根包 `boot`：

- `boot.Run(envPrefix, wire)`：默认配置路径与默认中间件链；服务名来自 `app.name`；`wire` 通常是业务 `internal/bootstrap.Wire`。
- `boot.RunWithOptions(bootOpts, wire)`：自定义 `ConfigPath`、`PluginOptions`、`PluginSet`（完全自定义插件链与配置段）、`InitTimeout`，并支持自定义中间件工厂。

两者内部完成 `boot.MustInit → wire → server.MustBuildApp → lifecycle.Run`。

需要完全自控启动时使用低层入口：`internal/bootstrap.NewApp(configPath) *lifecycle.App` + `lifecycle.Run(...)`。`lifecycle` 只负责 `Start → Ready → Serve → Drain → Stop → Release`，测试或嵌入式场景可用 `lifecycle.RunContext` 返回 error 而不是 panic。两种入口共用同一套 `wire`。

`lifecycle` 的阶段为 `Start → Ready → Serve → Drain → Stop → Release`：先 `net.Listen`，再执行 `Ready` 钩子（如 Consul 注册），收到退出请求后先逆序执行 `Drain` 钩子（如 Consul 注销），再 `HTTP Shutdown`，最后 `Release` 逆序关闭插件。

#### 3.4.3 bootstrap 约定

- 每个可部署单元 `internal/bootstrap/` 下以 **`server.go`** 为装配入口；需要消息消费者时另增 `*_consumer_plugin.go`。
- `server.go` 导出 `Wire(slot *plugin.PluginSlot) []router.Registrar` 作为手写组合根，通过各插件的 `From(slot)` 与组件访问器构造依赖链；`boot.Run(envPrefix, bootstrap.Wire)` 直接使用。
- 需要自定义配置路径或完全自控启动时，可另提供 `NewApp(configPath string) *lifecycle.App` 并调用 `lifecycle.Run`；两种入口共用同一套 wire。
- `main.go` 只调用 `boot.Run` / `boot.RunWithOptions`（或低层 `lifecycle.Run`）；`internal/bootstrap/server.go` 只组装与 wiring。
- `server.MustBuildApp` 返回的 `lifecycle.App.Release` 绑定 `slot.Close(ctx)`；停机顺序为 `OnDraining` → HTTP `Shutdown` → 插件逆序 `Close`，插件释放使用独立的 `ReleaseTimeout` 预算。

#### 3.4.4 插件生命周期

**接口**（`gitee.com/hq_project/yomi-boot/plugin`；内置实现见 `plugin/builtin/{lib}`，如 `plugin/builtin/logger`、`plugin/builtin/db`、`plugin/builtin/consul`）：

```go
type Plugin interface {
    Name() string
    Init(ctx context.Context, slot *PluginSlot) error // 失败返回 error；未启用返回 ErrDisabled
    Close(ctx context.Context) error                  // 释放自己持有的资源；须尊重 ctx
}
```

`PluginSlot` 是唯一的插件容器：

```go
type PluginSlot struct {
    Config *config.Config
    // 其余字段为有序插件链、名称索引与每个插件的初始化状态
}

func NewPluginSlot(cfg *config.Config, plugins []Plugin) *PluginSlot
func (s *PluginSlot) Init(ctx context.Context) error
func (s *PluginSlot) Plugin(name string) (Plugin, bool)
func (s *PluginSlot) MustPlugin(name string) Plugin
func (s *PluginSlot) Started(name string) bool
func (s *PluginSlot) ReadyHooks() []func(context.Context) error
func (s *PluginSlot) DrainHooks() []func(context.Context) error
func (s *PluginSlot) Close(ctx context.Context) error
```

- 插件对象自包含其创建的组件；跨插件依赖在 `Init` 中经 `slot.MustPlugin(name)`（或各包的 `From(slot)`）取到前序插件，再调用其组件访问器。
- 插件名称非空且唯一，重复注册在 `NewPluginSlot` 构造期 panic；`Init` 只能执行一次。
- `Init` 按序执行；插件返回 error 或 panic 时，逆序回滚已初始化插件，返回聚合错误。`Init` 返回 `ErrDisabled` 的插件记为跳过，不参与 hook 与 `Close`。
- `Init` 入参 ctx 只约束启动操作；需要长期运行的后台任务使用 `slot.RuntimeContext()`，该 ctx 在 `Close` 或 Init 失败回滚时取消。
- `Close` 逆序关闭已初始化插件，使用调用方 ctx（无 deadline 时默认 10 秒）；单个插件超时或 panic 后继续关闭剩余插件，返回聚合错误。
- 内置插件导出具体类型、`Name` 常量、`From` helper 与只读组件访问器；`From` 返回 nil 时访问器仍安全，依赖方按组件是否为 nil 做 Fail Fast。

需要参与进程阶段（注册发现、缓存预热/落盘等）的插件按需实现两个可选接口：

```go
type ReadyHook interface {
    OnReady(ctx context.Context) error
}

type DrainHook interface {
    OnDraining(ctx context.Context) error
}
```

- 只有已初始化插件会被收集；`Init` 返回 `ErrDisabled` 的插件会被跳过，不会误注册。
- `server.MustBuildApp` 将 `slot.ReadyHooks()` / `slot.DrainHooks()` 搬运到 `lifecycle.App`；`lifecycle` 只按阶段调用函数，不认识任何具体组件。
- `Ready` 按 Init 顺序执行，失败会导致启动失败；`Drain` 按 Init 逆序执行，失败只记录日志。
- `Drain` 可能在 `Ready` 未完整执行时被调用（启动失败回滚），因此必须幂等。
- 组件示例：Consul 插件在 `Init` 只创建 client，`OnReady` 注册服务，`OnDraining` 注销服务；业务 custom 插件可用同一契约做缓存预热/落盘。
- 业务 custom 插件的 `Init(ctx, slot)` 可经 `kafkaplugin.FromConsumer(slot)`、`eventplugin.From(slot)` 等访问器读取前序插件组件，再把需要的依赖保存在自身字段中。

服务自定义插件 embed 导出的 `Base`，与内置插件写法一致：

```go
type myConsumerPlugin struct {
    plugin.Base
    consumerID string
}

func (p *myConsumerPlugin) Init(ctx context.Context, slot *plugin.PluginSlot) error { ... }
func (p *myConsumerPlugin) Close(ctx context.Context) error { ... }
```

**内置插件启动顺序**（不可随意调整；后序 `Init` 通过 `PluginSlot` 查找前序插件并读取其组件）：

| 序号 | 插件 | 启动条件 |
| ---- | ---- | -------- |
| 1 | app | 始终（提供 `AppCtx`） |
| 2 | logger | 始终（依赖 app 插件） |
| 3 | pprof | `pprof.enabled=true` 时启动（默认 false） |
| 4 | db | `db.enabled=true`（提供 `DB` 与 `DbType`） |
| 5 | redis | `redis.enabled=true`（提供 `Client` / `RedisService` / `DistLockFactory`） |
| 6 | redisratelimit | 始终启动（未启用时 `Limiter` 为 Noop） |
| 7 | kafka-publisher | `kafka.publisher_enabled=true` |
| 8 | kafka-consumer | `kafka.consumer_enabled=true` |
| 9 | redisstream | `redis_stream.enabled=true`（依赖 redis 插件 `Client`） |
| 10 | snowflake | 始终 |
| 11 | oss | `oss.enabled=true` |
| 12 | tx | db 插件已启用 |
| 13 | consul | `consul.enabled=true`（Ready 阶段注册、Drain 阶段注销） |
| 14 | dtm | 始终构造客户端（注入 consul 插件 `Client`）；`dtm.enabled=true` 时探活；db 可用时构造 `tx.Runner`（供 Saga 分支 usecase 注入） |
| 15 | idempotency | `idempotency.stale_cleaner_enabled=true`（依赖 app/db/redis 插件） |
| 16 | token | `token.enabled=true`（依赖 redis 插件） |
| 17 | httprecord | `http_record.enabled=true` 时按 provider 装配；未启用时不创建服务（中间件短路） |
| 18 | event | `outbox_enabled` / `inbox_enabled` 任一为 true（按侧组装 Publisher/Receiver；须在依赖 Inbox 的消费者之前） |
| 19 | custom | `PluginOptions.AddCustomPlugin` 注册的服务自定义插件（可经 slot 依赖内置插件；可实现 ReadyHook / DrainHook；Close 时先于 event 停止） |

**插件 Close 语义**：

| 需优雅 Close | noop / 尽力 |
| ------------ | ----------- |
| event、kafka-publisher、kafka-consumer、redisstream、logger、consul、idempotency stale cleaner | db、redis、snowflake、oss、tx、token、httprecord（发布端）、dtm、pprof |
| 服务自定义插件（如 Stop 消息消费者） | — |

`Close` 应尽快返回；框架会按上限等待，超时视为该插件未关干净（记 Warn 后继续关闭前序插件），但不会中断仍在执行的 `Close`。

**消息消费者自定义插件约定**：

- gitee.com/hq_project/yomi-boot 消费端点（如 `httprecord.StartKafkaRecordConsumer`）只负责参数校验、反序列化、委托业务 `Handler`。
- 业务 handler、是否启用、消费者标识 / 组名等由**可部署单元内**自定义插件实现（可使用 `KafkaConsumer` 或 `RedisStreamConsumer`）。
- 通过 `pluginOpts.AddCustomPlugin(...)` 插入启动流程，**禁止**在 `wireHandlers` 或 gitee.com/hq_project/yomi-boot 内置插件中硬编码业务消费逻辑。

#### 3.4.5 能力按需启用

| 服务类型 | 典型做法 | 说明 |
| -------- | -------- | ---- |
| 入口编排（BFF） | `PluginOptions{}` + 按需 `AddConfigSection`；YAML 开 db/redis/token/consul（及按需 kafka/redis_stream/event/dtm） | 本进程独立库仅基础设施表；Redis 用于令牌 Store；禁止连领域库 |
| 领域 | `PluginOptions{}`；YAML 开 db，按需 redis/kafka/redis_stream/consul，通常关 token | Redis 供幂等/锁等；业务层不直接访问 Redis 细节 |
| 带消息消费的领域 | 上栏 + `AddCustomPlugin(消费者插件)` | 消费启停由自定义插件与配置控制 |

各组件是否启动**一律以对应配置段为准**（如 `db.enabled`、`token.enabled`、`event.outbox_enabled` / `event.inbox_enabled`）；缺依赖时由插件 Init / 构造函数 Fail Fast。

**形态裁剪参考（改配置即可，不必改 gitee.com/hq_project/yomi-boot 源码）**：

| 形态 | 建议启用 | 建议关闭 / 暂缓 |
| ---- | -------- | --------------- |
| 轻量单体 | `db`；按需 `redis`、`token`、`pprof` | `consul`、`kafka`、`redis_stream`、`dtm`、`event`、`http_record` |
| 大单体 | 上栏 + 按需 `redis` 幂等/锁、`cronjob` | 无跨进程时不必开 `consul`/`dtm`；异步消息有需求再开 `kafka` 或 `redis_stream` |
| 分布式微服务 | `db`/`redis`/`consul`；入口编排开 `token`；跨域写开 `dtm`；异步按需 `kafka`/`redis_stream`/`event` | 按服务类型裁剪（领域通常关 `token`） |

说明：开关名以各配置段为准（如 `db.enabled`、`token.enabled`、`consul.enabled`、`kafka.publisher_enabled` / `kafka.consumer_enabled`、`redis_stream.enabled`、`dtm.enabled`、`event.outbox_enabled` / `event.inbox_enabled`、`pprof.enabled`、`http_record.enabled`）。能力裁剪权威说明见 [Go配置规范](./Go配置规范.md)。

### 3.5 业务工程依赖 gitee.com/hq_project/yomi-boot

各可部署单元 `go.mod` 标准依赖：

```go
require gitee.com/hq_project/yomi-boot v1.0.0
```

`import` 路径为 `gitee.com/hq_project/yomi-boot/...`。本地联调可用相对路径 `replace` 或根目录 `go.work`。

### 3.6 HTTP 路由约定

| 类型 | 路径前缀 | 方法 | 调用方 |
| ---- | -------- | ---- | ------ |
| 入口编排对外 | `router_prefix` + 相对路径（`router_prefix` 按端在配置中设置；**示例**：如管理端/App 的前缀 + `/v1/{resource}/profile`） | **GET / POST** | 前端 / 外部客户端 |
| 领域读 | `/internal/v1/{resource}/...` | **POST only** | 入口编排（经 client） |
| 领域写 Action | `/internal/v1/{resource}/{action}` | **POST only** | DTM Server |
| 领域写 Compensate | `.../compensate` | **POST only** | DTM Server |
| 入口编排 Consul 探针 | `router_prefix` + `/v1/health` | GET | Consul HTTP Check |
| 入口编排运维探针 | `router_prefix` + `/v1/health/ops` | GET | 运维 / 监控 |
| 领域 Consul 探针 | `/internal/v1/{resource}/health` | GET | Consul HTTP Check |
| 领域运维探针 | `/internal/v1/{resource}/health/ops` | GET | 运维 / 监控 |

- 入口编排 handler 注册**相对路径**，完整 URL = `http.router_prefix` + 相对路径。
- **入口编排对外主输入**：默认推荐 **POST + JSON Body**；允许 GET（CDN/SEO）。若 GET 仅用 Query 承载主输入，须在该接口契约中**显式约定**（非默认）。
- 领域**写**接口仅 Action/Compensate 成对；入口编排 **不得**直调写路径。详见 [微服务调用规范](./微服务调用规范.md) §3.1.1。
- 链路 ID 经 HTTP Header `X-Trace-ID` 传播（`gitee.com/hq_project/yomi-boot/component/core/traceid`），**不在** JSON Body 信封中传递。
- 微服务间请求/响应契约见 [微服务调用规范](./微服务调用规范.md) §3（权威）。
- 对外出口编码适配见 [API编码适配规范](./API编码适配规范.md)（**不**规定 HTTP 方法；方法以本节约定为准）。
- 路径中资源段使用**单数名词**，禁止无意义复数。

### 3.7 中间件链

`server.DefaultMiddlewares` 统一装配下列链；`cors.Middleware` / `token.Middleware` / `httprecord.Middleware` / `logger.Middleware` 均在中间件内按各自 `enabled` 决定是否生效。

```text
CORS → traceid → httprecord.Middleware → logger.Middleware → httpguard → Recovery → token.Middleware → handler
```

中间件始终挂载；`cors.enabled` 由 BootConfig 映射为 `cors.Config.MiddlewareEnabled`，`http_record.enabled` / `token.enabled` 由插件映射为运行时 `MiddlewareEnabled`；关闭时由中间件在组件内短路放行，不在 `boot` 层动态增删中间件。

- **顺序约束**：`httprecord.Middleware` 与 `logger.Middleware` 必须位于 `httpguard` **外侧**。`httpguard` 在 `c.Next()` 返回后才可能写出 `REQUEST_TIMEOUT`；若采集层更靠内，会在超时信封写出前落盘，造成审计/access 的 `StatusCode`/`BizCode` 失真。
- `token.enabled=true`（入口编排）时校验 Bearer Token；未配置或 `false`（领域）时中间件短路放行（仍保留在链上）。
- `traceid` 中间件从 Header `X-Trace-ID` 读取或生成 trace ID，写入 Context；出站 `gitee.com/hq_project/yomi-boot/component/cloud/rpc` 自动回填 Header。
- 领域进程鉴权关闭后不校验令牌；身份字段由入口编排按接口 DTO 显式传入 Body（单体共进程时可按模块边界自行约定）。
- 入口编排**对外**业务响应：经已注册 Encoder 写出；HTTP 状态与 JSON 形状由 Encoder 决定，见 [API编码适配规范](./API编码适配规范.md)。
- 领域**读**（微服务间）：对内信封与 HTTP 状态见 [微服务调用规范](./微服务调用规范.md) §3.3.1。
- 领域 **DTM Action/Compensate** 回调契约见 [微服务调用规范](./微服务调用规范.md) §3.3.2（`gitee.com/hq_project/yomi-boot/component/cloud/dtm`）。

### 3.8 pprof 排障

- 配置项：`pprof.enabled`（默认 `false`）、`pprof.addr`（独立 debug 端口，由各可部署单元自行配置，避免端口冲突）。
- 由 `plugin/builtin/pprof` 在 `MustInit` 阶段按配置启动；进程退出时随进程自然关闭，无需显式释放。
- 常用命令：
  - `go tool pprof http://{host}:{pprof_port}/debug/pprof/profile?seconds=30`
  - `go tool pprof http://{host}:{pprof_port}/debug/pprof/heap`
  - `go tool pprof http://{host}:{pprof_port}/debug/pprof/goroutine`

### 3.9 配置约定

环境文件命名统一为 `config/local.yml`、`config/dev.yml`、`config/prod.yml`。`gitee.com/hq_project/yomi-boot/component/core/config.ResolvePath()` 默认从 `config/` 目录加载，根据 `YOMI_CONFIG`（`local`/`dev`/`prod`，**默认 `dev`**）选择文件。

`dev.yml` / `prod.yml` 中中间件连接参数（host、port、user、password 等）使用 `${YOMI_*:默认值}` 占位符；`gitee.com/hq_project/yomi-boot/component/core/config.LoadConfig` 加载时展开，未设置的环境变量取冒号后默认值。`local.yml` 仍可直接写本地默认值。

```yaml
# 入口编排 config/local.yml 示意
consul:
  addr: "127.0.0.1:8500"
  service_name: "{bff}"

dtm:
  server: "http://localhost:36789/api/dtmsvr"
  barrier_table: "sys_barrier"

token:
  store_prefix: "yomi:token:{bff}"

redis: ...
kafka: ...
```

```yaml
# 领域 config/local.yml 示意
consul:
  service_name: "{domain}"
db: ...
redis: ...
kafka: ...
# 通常无 token、无下游 client
```

### 3.10 质量门禁（depguard）

- `gitee.com/hq_project/yomi-boot` 不得 import 业务可部署单元代码。
- client 契约模块不得 import 可部署单元 `internal`，也不得向 `gitee.com/hq_project/yomi-boot` 注入业务代码。
- 领域 `internal/biz` 不得 import 其他领域可部署单元代码；不得 import 入口编排层。
- 入口编排通过 client 契约模块调用领域，不得 import 领域 `internal/biz`。
- 分层 depguard 细则见 [Go业务分层规范](./Go业务分层规范.md) §3.8。

---

### 3.11 基础库优先使用（强制）

凡涉及 **Go 基础设施能力**（装配、HTTP、DB、缓存、锁、限流、RPC、消息、鉴权、加解密、对象存储、ID、错误模型、定时任务、测试辅助等）的设计、实现、改造、巡检或方案评审，**必须优先复用本仓已有能力包**（§3.3）。禁止在能力已覆盖且能满足需求时，另行引入同类第三方库或在业务仓/本仓平行实现。

**决策流程**：

```text
阅读 §3.3 模块职责
  → 判断是否存在相同或相近能力
    → 有候选：阅读对应包源码（含 plugin / 导出 API / 配置段）
      → 能满足需求：必须使用该包（或在其扩展点上扩展）
      → 不能满足：说明缺口后，强制选择新增方式（见下），再实现
    → 无候选：强制选择新增方式（见下），再实现
```

1. **有候选时必须阅读源码**：读对应包下的公开 API、包注释、`plugin`（若有）及关键配置结构，判断功能边界、错误语义、配置开关、生命周期与扩展点；仅凭包名或一行描述判定「不能用」禁止。
2. **满足则强制使用**：含经配置启用、实现接口、注册插件、组合已有 API 等方式；禁止在业务仓再封装同职责组件、禁止重复引入已封装能力的第三方库、定制优先走扩展点而不是复制源码。
3. **有候选但确认不满足**：先列出已对照的包路径与缺口清单，再进入新增方式选择。
4. **无候选 / 需新增时**：禁止直接写代码或默认落业务仓；必须先向开发者列出下列 A/B/C 并**等待其明确选择**后，方可继续设计与实现。

| 选项 | 含义 | 适用倾向 |
| ---- | ---- | -------- |
| **A. 扩展既有能力包** | 在已有包内增 API / 配置 / plugin 行为，不新建顶层包 | 与某现有包职责相邻、可自然收口 |
| **B. 新增顶层能力包** | 在本仓新建独立能力包（及按需 `*/plugin`） | 独立基础设施能力、可被多业务仓复用 |
| **C. 业务仓特例实现** | 仅在当前业务仓实现，不进入本仓 | **仅当**能力强绑定单一业务域、确认无跨仓复用价值；须写明理由与回迁条件 |

选择优先级默认 **A → B → C**；选择输出格式（强制，字段齐全）：

```text
【基础库新增方式选择】
- 索引结论：无候选 / 有候选但不满足（列出已对照路径与缺口）
- 请选择其一：A 扩展既有包 | B 新增顶层包 | C 业务仓特例
- 已确认选择：（待用户确认 / A|B|C）
- 落位说明：（包名或业务仓路径；选 A/B 时同步更新 §3.3 模块职责表的计划）
```

**禁止行为**：

| 禁止 | 说明 |
| ---- | ---- |
| 凭印象否定 | 未读候选包源码就认定「不好用 / 不够用」并平行实现 |
| 跳过新增方式选择 | 无候选或需新增时未经 A/B/C 明确选择就开始实现 |
| 默认落业务仓 | 无候选时未经选择、擅自在业务仓实现可复用基础设施 |
| 业务仓造轮子 | 本仓已满足需求时，在业务仓再写一套 DB/锁/限流/幂等/RPC/存储等 |
| 重复依赖 | 引入与本仓已封装能力重复的第三方库替代本仓能力 |
| 复制粘贴库代码 | 把本仓源码拷到业务仓改一改当「自有实现」 |

**能力场景速查**：

| 你需要… | 优先看 |
| -------- | ------ |
| 进程启动、插件装配、中间件链 | `boot`、`plugin`、`lifecycle`、`config` |
| HTTP 绑定/响应/路由 | `handler`、`response`、`router` |
| 数据源 / 本地事务 / Repository 基类 | `db` + `db/orm` + `db/tx` |
| 业务幂等 | `idempotency` |
| 跨域 Saga | `dtm` |
| Redis / 分布式锁 / 调用式限流 | `redis`、`distlock`、`redisratelimit` |
| 定时任务 | `cronjob`（内部轮询用 `roundrobin`） |
| 服务发现与读 RPC | `consul`、`rpc` |
| 异步消息 | 按需选用 `kafka` 或 `redisstream`；可靠领域事件另看 `event` |
| WebSocket 长连接 | `ws`（独立端口 Server/Client；ConnID 仅服务端）；小文件可走 WS，大文件走 `oss` |
| 鉴权令牌 | `token` |
| 加解密 / 哈希 / 密码 | `bc/*` |
| 对象存储 | `oss`（大文件上传/下载；勿用 `ws` 整包） |
| 调用大模型 Chat / 流式 | `component/ai/llm`（`New`/`MustNew`；OpenAI / Anthropic；业务自管配置；一实例一模型；勿直引官方 SDK；不做限流） |
| 文本向量化 Embedding | `component/ai/embedding`（`New`/`MustNew`；OpenAI Embeddings；APIKey 可空；勿直引官方 SDK；不做限流） |
| Jev 结构化决策 | `component/ai/jev`（`New`/`MustNew`；System One；业务自管配置；一实例一模型；勿直引官方 SDK；不做限流） |
| 文本切块 | `component/ai/chunking`（`New`/`ChunkText`；内置 recursive / fixed / sentence / markdown，可注入自定义 `Chunker`；与 ai 其他组件零依赖；不读 YAML） |
| 通用向量 / BM25 检索 | `component/ai/retrieval`（`New`/`MustNew`；hybrid 默认 / vector；对任意 vectordb 集合做通用检索，不认识业务元数据；不读 YAML） |
| RAG 入库 / 检索 / 删除 | `component/ai/rag`（`New`/`MustNew`；同一 Engine 可服务多个集合，集合名按调用传入；切块经 `component/ai/chunking`，可注入自定义 `Chunker`；检索经 `component/ai/retrieval` 并叠加旧块过滤；基于 `component/ai/embedding` + `component/ai/vectordb`；不含问答；不读 YAML） |
| 向量写入 / ANN 检索 / 动态索引 | `component/ai/vectordb`（`New`/`MustNew`；milvus 或 memory；Dimension/Metric 锁在 Store；勿直引 Milvus SDK；不读 YAML） |
| 精重排 Rerank | `component/ai/rerank`（实现 `Reranker` 接口接入自有模型，`ValidateRequest` / `RankScores` 供实现复用；PassThrough 保序 TOP-N 占位；与 ai 其他组件零依赖；不读 YAML） |
| 工具定义 / 填槽 / 执行 | `component/ai/tool`（`Spec` 原生 JSON Schema + `Fill`；`Registry` 不可变；`NewFunc` / `NewTyped` / `NewResolved`；`Fill` / `InvokeFilled` / `FillAndInvoke` / `Invoke` / `InvokeAll`；`Completer` 模型端口；缺槽 `Missing`；对外错误统一 `*Error`，底层原因只经 Cause 进内部日志；与 ai 其他组件零依赖；不读 YAML） |
| 雪花 ID | `snowflake` |
| 错误码 / 信封 DTO | `errors`、`shared/error`、`shared/dto` |
| 集成测试 HTTP 调用 | `testclient` |

**维护约定**：新增或下线能力包时，**同步更新 §3.3 模块职责表**（必要时更新本节场景速查）；配置段、启用开关、装配顺序等细则以对应知识文档为准。

---

## 4. 检查清单

按形态裁剪适用项：单体不强制拆多进程或启用 Consul/DTM；分布式按服务类型勾选。

- [ ] 业务可部署单元经标准 `require gitee.com/hq_project/yomi-boot`（本地可用 `go.work` / `replace`）引用本库；`import` 为 `gitee.com/hq_project/yomi-boot/...`
- [ ] `gitee.com/hq_project/yomi-boot` 不依赖任何业务可部署单元代码
- [ ] 每个可部署单元 `internal/bootstrap/` 以 `server.go` 为装配入口；消息消费者插件单独文件
- [ ] 启动链路使用 `boot.Run` / `boot.RunWithOptions`（或低层 `lifecycle.Run`）；启动失败 Fail Fast（panic）
- [ ] 未使用的基础设施能力已在 YAML 中关闭对应 `enabled`
- [ ] 消息/平台消费的业务逻辑在可部署单元自定义插件中实现；gitee.com/hq_project/yomi-boot 仅提供消费端点
- [ ] 分布式形态：领域进程无跨域 HTTP、无 token 中间件
- [ ] 入口编排含 `handler/`、`bff/`、`saga/`（若属该服务类型）
- [ ] 跨进程契约符合 [微服务调用规范](./微服务调用规范.md) §3（若启用）
- [ ] 链路 ID 经 Header `X-Trace-ID` 传播，非 Body 字段
- [ ] 启用 DTM 时 Action/Compensate 路由成对，补偿路由为正向路由 + `/compensate`
- [ ] 业务分层符合 [Go业务分层规范](./Go业务分层规范.md)

---

## 5. 禁止项

- 业务以独立工程 + 可部署单元组织；禁止在业务工程根目录放置业务 `internal/biz` 或业务装配入口（应落在各可部署单元内）。基于 gitee.com/hq_project/yomi-boot 的单进程部署仍属合法形态（见 §3.1.0）。
- 禁止领域进程之间 HTTP 互调（分布式形态；单体进程内模块调用不受此限）。
- 禁止把业务签发/校验策略放入 `gitee.com/hq_project/yomi-boot/component/common/token`（gitee.com/hq_project/yomi-boot 仅提供库；鉴权方在 config 中配置 `token` 段）。
- 禁止服务间默认使用 gRPC 等其他 RPC 协议替代 HTTP（分布式形态）。
- 禁止在 JSON Body 中传递 `traceId`（统一使用 Header `X-Trace-ID`）。
- 禁止在领域 handler/usecase 中读取 token 或 `SessionFromContext`（分布式形态下的领域服务类型）。
- 禁止在 `wireHandlers` 或 gitee.com/hq_project/yomi-boot 内置插件中编写业务消费逻辑（应通过 `AddCustomPlugin` 注册）。
- 禁止在 `internal/bootstrap/` 下存在 `wire/` 子目录或独立 `wire.go`。
