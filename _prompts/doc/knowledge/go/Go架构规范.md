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
- **边界**：BFF/入口编排细则见 [GoBFF规范](./GoBFF规范.md)；领域分层见 [Go业务分层规范](./Go业务分层规范.md)；本地事务与 DTM Saga 见 [Go事务规范](./Go事务规范.md)；微服务间契约见 [微服务调用规范](../api/微服务调用规范.md)（**信封与幂等权威**）；**文件命名**见 [Go文件命名规范](./Go文件命名规范.md)；**数据访问**见 [Go数据库规范](./Go数据库规范.md)；**配置加载、扩展与能力裁剪**见 [Go配置规范](./Go配置规范.md)。
- **适用对象**：gitee.com/hq_project/yomi-boot 库本身，以及基于 gitee.com/hq_project/yomi-boot 的业务工程与可部署单元。业务工程为独立仓库，通过标准 `require gitee.com/hq_project/yomi-boot` 依赖本库；本库仓库内不包含业务服务代码。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于 `gitee.com/hq_project/yomi-boot/`：可复用基础设施库；各能力插件统一编排，**是否真正启动以配置段 `enabled`（及同类开关）为准**，可按需裁剪。
- 适用于基于 gitee.com/hq_project/yomi-boot 的多种部署形态（详见 §3.1.0、§3.4.5）：
  - **轻量单体**：单进程、单库；通常只开 `db`（及按需 `redis`/`auth`），关闭 `consul`/`kafka`/`redis_stream`/`dtm`/`event` 等。
  - **大单体 / 模块化单体**：单部署单元内多业务模块分层；仍用同一套 bootstrap/插件，按模块边界组织代码，不必拆多进程。
  - **分布式微服务**：多进程按服务类型独立部署；开 `consul`/`rpc`，跨域写按需开 `dtm`，异步按需开 `kafka` / `redis_stream` / `event`。
- 适用于基于 gitee.com/hq_project/yomi-boot 组装的业务工程（独立 `go.mod`，经标准 `require` 引用 `gitee.com/hq_project/yomi-boot`；本地可用 `go.work` 联调）：具体服务名与业务域随产品演进，**以服务类型为准，不以某一仓库清单为准**。
- Consul / HTTP RPC / DTM 等为**分布式路径**能力，非 gitee.com/hq_project/yomi-boot 强制依赖。
- 不适用于前端工程（前端规范仅作消费者参考，本库仓无前端代码）。

### 2.2 关联文档

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| `_prompts/doc/knowledge/go/GoBFF规范.md` | 入口编排 | 跨域调用、Body 信封、Saga、令牌注入 |
| `_prompts/doc/knowledge/go/Go业务分层规范.md` | 业务分层 | 入口编排与领域各层职责 |
| `_prompts/doc/knowledge/go/Go选型规范.md` | 技术栈基线 | gin、gorm、Consul、DTM、HTTP 通信选型 |
| `_prompts/doc/knowledge/go/Go事务规范.md` | 事务 | 本地子事务与 DTM Saga |
| `_prompts/doc/knowledge/api/微服务调用规范.md` | 微服务契约 | POST/JSON、Header 链路 ID、Body 信封 bizId/bizSubId |
| `_prompts/doc/knowledge/api/API编码适配规范.md` | 对外 API | Encoder 显式注册与写出适配；入口编排层可用 GET/POST |
| `_prompts/doc/knowledge/go/Go数据库规范.md` | 数据访问 | repository 与 GORM 规范 |
| `_prompts/doc/knowledge/go/Go配置规范.md` | 配置 | Section 注册、严格根 key、业务扩展、**能力按需裁剪（enabled）** |
| `_prompts/doc/knowledge/redis/Redis设计规范.md` | Redis | 令牌 Store 键设计 |
| `_prompts/doc/knowledge/go/Go注释规范.md` | 注释 | 代码注释口径 |
| `_prompts/doc/knowledge/知识库文档规范.md` | 文档基线 | 知识文档结构 |

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
- 插件在 `bootstrap` 中按固定顺序注册；**Init 是否真正启动由各自配置决定**（见 `infra_plugins.go` 与各 `{lib}/plugin`）。
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
| **入口编排（BFF）** | 对外 API、鉴权、跨域读聚合、跨域写编排（Saga） | `auth`/`token`、按需 `db`/`redis`/`consul`/`dtm`/`kafka`/`redis_stream`/`event` |
| **领域** | 单一业务边界内的读写与本地事务；对外提供内部 API | `db`、按需 `redis`（幂等/锁）、`consul`；通常关 `auth` |
| **基础设施消费者（可选）** | 消费平台/异步消息并持久化，或提供对内查询 | `kafka` 或 `redisstream` + `AddCustomPlugin`；按需 `db` |

- 同一进程可合并多种服务类型（单体）：例如入口 + 多领域模块共进程，跨模块走内存依赖。
- 分布式形态下通常一进程一主服务类型；具体叫什么、拆几个，由业务决定。

#### 3.1.3 分布式形态下的调用边界

- **领域进程之间禁止 HTTP 互调**。
- **跨域 HTTP 仅由入口编排层**经 client 契约模块发起。
- 单体形态下跨模块调用走进程内依赖，不适用本条 HTTP 互调禁令。

#### 3.1.4 gitee.com/hq_project/yomi-boot 只放基础设施

- `gitee.com/hq_project/yomi-boot/` 提供 config、plugin（生命周期框架）、各库 `*/plugin` 内置生命周期实现、logger、db、redis、router、runner、consul、dtm、rpc、distlock、errors、traceid、token、kafka、**redisstream**、httprecord、snowflake、**pprof**、**idempotency**、**ratelimit**、**bc**、**event**、**roundrobin**、**cronjob**、**appctx**、**banner**、**worker**、**testclient** 等能力；**上述能力按配置按需启用**，并非每个进程都必须全部启动。
- `gitee.com/hq_project/yomi-boot` 禁止反向依赖任何业务可部署单元代码。
- `gitee.com/hq_project/yomi-boot/token` 提供通用令牌库（Store、Claims、中间件骨架）与 `TokenService`；需要鉴权的进程（入口编排或单体入口）直接注入使用。
- `gitee.com/hq_project/yomi-boot/ratelimit` 提供调用式限流门面（`Allow`/`AllowN`），依赖 `RedisService`，**不进入默认中间件链**；场景配额由调用方传入 `Rule`。

### 3.2 标准目录结构（模式）

以下为**业务工程**推荐落位模式（与 gitee.com/hq_project/yomi-boot 分仓）；`{bff}`、`{domain}` 为占位符。gitee.com/hq_project/yomi-boot 本仓根目录即为 `module gitee.com/hq_project/yomi-boot`。

```text
# gitee.com/hq_project/yomi-boot 仓库（本库）
gitee.com/hq_project/yomi-boot/                               # module: gitee.com/hq_project/yomi-boot
├── bootstrap/, plugin/, config/, ...
└── （能力包见 §3.3）

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

| 包 | 职责 |
| -- | ---- |
| `bootstrap` | `MustInitInfra`、`MustBuildServer`、`DefaultMiddlewares`；`Infra` 仅聚合运行时依赖，配置经 `Infra.Doc` + `MustSection` 读取；经 `infra_plugins.go` 编排内置插件顺序；`InfraOptions` 扩展（`AddConfigSection` / `AddCustomPlugin`） |
| `plugin` | 生命周期框架：`Plugin` 接口、`PluginState`、`Host`、`Runtime`（与 `config` 同层） |
| `{lib}/plugin` | 各库内置生命周期插件实现（如 `db/plugin`、`redis/plugin`；`tx` 挂在 `db/plugin`）；包名 `{lib}plugin` |
| `consul` | 服务注册、注销、发现、HTTP URL 解析 |
| `dtm` | Saga 客户端、GORM barrier、`TxRunner`、`BindAndHandleAction/Compensate` |
| `traceid` | BFF 入站中间件生成/读取 `X-Trace-ID` Header；出站 RPC 经 Header 传播 |
| `pprof` | 独立 debug 端口暴露 `net/http/pprof`；由 `pprof/plugin` 在 `MustInitInfra` 阶段启动；进程退出时自然关闭；默认关闭 |
| `token` | Store、Claims、中间件骨架、`TokenService`（签发/校验/撤销） |
| `db` | GORM 打开、`TxExecutor`、`BaseRepository`、`BaseModel` |
| `idempotency` | `sys_biz` 模型、仓储、`Guard`、`Execute`；三层幂等（Redis + 锁 + DB）；细则见 [微服务调用规范](../api/微服务调用规范.md) §3.4 |
| `event` | 可选 Outbox/Inbox 事件组件（`publisher_enabled` / `consumer_enabled` 任一为 true 时由插件启动） |
| `router` | `Registrar` 接口与路由注册 |
| `handler` | `BindAndHandle` 等协议适配 |
| `shared` | 跨服务通用 dto（含微服务请求/响应信封）、enums、常量 |
| `utils` | 通用辅助函数收口（`ginutil`、`ptrutil`、`timeutil`、`validateutil` 等子包） |
| `redis` | `RedisService` 接口与 `RedisHelper` 实现（含 `OpsRateLimit4Allow` 滑动窗口限流原语） |
| `ratelimit` | 调用式限流门面 `LimitService`（`Allow`/`AllowN`）；依赖 `RedisService`，不进默认中间件链 |
| `bc` | 密码学基础设施能力簇（Bootstrap Crypto）；无状态按需 import，不挂 Infra |
| `bc/passwd` | 密码单向哈希/校验（bcrypt） |
| `bc/random` | 密码学安全随机（Bytes/Hex/URLSafe） |
| `bc/md5` | MD5 摘要（禁止用于密码） |
| `bc/hash` | SHA-256 / SHA-512 摘要（禁止用于密码） |
| `bc/hmac` | HMAC-SHA256 消息认证 |
| `bc/aes` | AES-GCM（推荐）/ CBC；ECB 不推荐 |
| `bc/rsa` | RSA PEM、OAEP 加解密、PSS/PKCS1v15 签名 |
| `kafka` | 生产者 `PublisherService`、消费者 `ConsumerService`、`kafkatest` 测试封装 |
| `redisstream` | Redis Stream 发布/消费组（`PublisherService` / `ConsumerService`）；复用 `RedisClient`，不耦合 `RedisService` |
| `httprecord` | HTTP 访问记录中间件、Kafka 发布端；`StartKafkaRecordConsumer` / `MustStartKafkaRecordConsumer` 提供消费端点（反序列化 + 委托 handler），**不含**业务消费逻辑 |
| `rpc` | HTTP RPC 读客户端（Consul 发现 + POST JSON）；轻量重试（退避）+ 按下游服务熔断；跨域写走 DTM，不在此叠加写重试 |
| `ws` | WebSocket 高级会话门面：Config 配 Processor/可选 Handler；默认 Handler 消费消息；ConnID 仅服务端；组装 `wsbase` |
| `ws/wsbase` | WebSocket 传输原语（gws）：独立端口 `Server`、出站 `Client`、共用 `Conn`/`Handler`、可配置分片 `WriteReader`、`Native()`；无 yml/默认插件 |
| `distlock` | 基于 Redsync 的分布式锁、看门狗 |
| `errors` | 系统级错误包装与 `BizError` 业务异常类型 |
| `response` | `Encoder` 接口、显式 `MustRegister`；可提供预制实现供业务显式选用（预制 ≠ 强制契约） |
| `shared/error` | 跨库通用哨兵（`NewNoStack`；返回须 `errors.Sentinel`） |
| `roundrobin` | 分布式定时轮询调度（event outbox 等内部使用） |
| `cronjob` | 业务日历定时任务（cron 表达式 + distlock 互斥）；禁止业务直引 `robfig/cron`；内部轮询仍用 `roundrobin` |
| `snowflake` | 雪花 ID 生成器 |
| `appctx` | 应用上下文（名称、环境、版本） |
| `banner` | 启动横幅打印 |
| `runner` | 服务启动/关闭生命周期管理（`Run` / `RunServer`） |
| `cors` | 跨域中间件 |
| `logger` | 日志初始化与配置；`MustInit` Fail Fast |
| `config` | 配置加载抽象（Section/Registry/Document）、严格根 key、环境变量展开；`MustLoad` / `MustLoadDocument` Fail Fast；详见 [Go配置规范](./Go配置规范.md) |
| `worker` | 后台 worker 工具 |
| `testclient` | 集成测试 HTTP 客户端辅助 |

**平台共享 DDL**（与封装同包维护，**无**独立 `gitee.com/hq_project/yomi-boot/sql/` 目录）：

| 文件 | 表名 | 说明 |
| ---- | ---- | ---- |
| `gitee.com/hq_project/yomi-boot/idempotency/sys_biz.sql` | `sys_biz` | 业务幂等表 |
| `gitee.com/hq_project/yomi-boot/dtm/barrier.sql` | `sys_barrier` | DTM 子事务屏障表 |
| `gitee.com/hq_project/yomi-boot/event/sql/sys_event.sql` | Outbox/Inbox 相关 | 可选，`event.publisher_enabled` / `event.consumer_enabled` 启用时使用 |

**gitee.com/hq_project/yomi-boot 明确不包含**：业务 handler/usecase、各领域 `bizerr` 哨兵。

### 3.4 启动与装配

#### 3.4.1 Fail Fast 约定

启动 / 运行 / 关闭三阶段异常策略以 [Go异常规范](./Go异常规范.md) §3.5 为准。本文仅强调装配侧：

- 配置加载、插件 `Init`、HTTP 装配失败走启动 Fail Fast（log + panic）；**不**在插件 `Init` 失败时回滚已启动插件。
- 各 gitee.com/hq_project/yomi-boot 库对外提供 `Must*` 包装（如 `config.MustLoad`）；构造函数内部假定入参已校验。
- HTTP 请求路径的 `error` 传播与 Recovery 见 Go异常规范。

#### 3.4.2 启动链路

```text
cmd/main.go
  └─ runner.Run(name, bootstrap.NewServer)
       └─ services/*/internal/bootstrap/server.go
            ├─ InfraOptions（按需 AddConfigSection / AddCustomPlugin）
            ├─ gitee.com/hq_project/yomi-boot/bootstrap.MustInitInfra(configPath, envPrefix, opts)
            │     ├─ DefaultConfigRegistry + MustLoadDocument
            │     └─ plugin.Run(host, buildInfraPlugins(opts))   # 按序 Init；是否启动看各段 enabled
            ├─ wireHandlers(infra) → []router.Registrar         # 手写 wiring，在 server.go 内
            └─ gitee.com/hq_project/yomi-boot/bootstrap.MustBuildServer(infra, registrars, middlewares)
```

#### 3.4.3 bootstrap 约定

- 每个可部署单元 `internal/bootstrap/` 下以 **`server.go`** 为装配入口；需要消息消费者时另增 `*_consumer_plugin.go`。
- `server.go` 内的 `NewServer(configPath string) *runner.ServerBundle` 手写组合根，通过 `wireHandlers(infra)` 显式 `NewXxx` 构造依赖链。
- `main.go` 只调用 `runner.Run`；`internal/bootstrap/server.go` 只组装与 wiring。
- `MustBuildServer` 返回的 `runner.ServerBundle.Cleanup` 绑定 `infra.Cleanup()`，停机时先 HTTP `Shutdown`，再逆序执行插件 `Close`。

#### 3.4.4 插件生命周期

**接口**（`gitee.com/hq_project/yomi-boot/plugin`；内置实现见各库 `gitee.com/hq_project/yomi-boot/{lib}/plugin`）：

```go
type Plugin interface {
    Name() string
    Started() bool
    Init(host *Host)   // 失败 panic；按需启动由配置 enabled 控制
    Close(host *Host)  // 无返回值；尽力释放，失败静默
}
```

服务自定义插件 embed 导出的 `PluginState`，与内置插件写法一致：

```go
type myConsumerPlugin struct {
    plugin.PluginState
    consumerID string
}
```

**内置插件启动顺序**（不可随意调整；后序 `Init` 依赖前序写入 `Host` 的字段）：

| 序号 | 插件 | 启动条件 |
| ---- | ---- | -------- |
| 1 | app | 始终（写入 `Host.AppCtx`） |
| 2 | logger | 始终 |
| 3 | pprof | `pprof.enabled=true` 时启动（默认 false） |
| 4 | db | `db.enabled=true` |
| 5 | redis | `redis.enabled=true` |
| 6 | ratelimit | 始终写入 RateLimiter（未启用时为 Noop） |
| 7 | kafka | `kafka.enabled=true` |
| 8 | redisstream | `redis_stream.enabled=true`（依赖 `RedisClient`） |
| 9 | snowflake | 始终 |
| 10 | tx | `host.DB` 存在 |
| 11 | dtm | 写入 DtmServer；`dtm.enabled=true` 时探活 |
| 12 | idempotency | `idempotency.stale_cleaner_enabled=true` |
| 13 | token | `auth.enabled=true` |
| 14 | httprecord | `http_record.enabled=true` 时按 provider 装配，否则 Nop |
| 15 | consul | `consul.enabled=true` |
| 16 | event | `publisher_enabled` / `consumer_enabled` 任一为 true（按侧组装 Publisher/Receiver；须在依赖 Inbox 的消费者之前） |
| 17 | custom | `InfraOptions.AddCustomPlugin` 注册的服务自定义插件（可依赖 KafkaConsumer / RedisStreamConsumer / Event.Receiver；Close 时先于 event 停止） |

**插件 Close 语义**：

| 需优雅 Close | noop / 尽力 |
| ------------ | ----------- |
| event、kafka、redisstream、logger、consul、idempotency stale cleaner | db、redis、snowflake、tx、token、httprecord（发布端）、dtm、pprof |
| 服务自定义插件（如 Stop 消息消费者） | — |

**消息消费者自定义插件约定**：

- gitee.com/hq_project/yomi-boot 消费端点（如 `httprecord.StartKafkaRecordConsumer`）只负责参数校验、反序列化、委托业务 `Handler`。
- 业务 handler、是否启用、消费者标识 / 组名等由**可部署单元内**自定义插件实现（可使用 `KafkaConsumer` 或 `RedisStreamConsumer`）。
- 通过 `opts.AddCustomPlugin(...)` 插入启动流程，**禁止**在 `wireHandlers` 或 gitee.com/hq_project/yomi-boot 内置插件中硬编码业务消费逻辑。

#### 3.4.5 Infra 组件按需启用

| 服务类型 | 典型做法 | 说明 |
| -------- | -------- | ---- |
| 入口编排（BFF） | `InfraOptions{}` + 按需 `AddConfigSection`；YAML 开 db/redis/auth/consul（及按需 kafka/redis_stream/event/dtm） | 本进程独立库仅基础设施表；Redis 用于令牌 Store；禁止连领域库 |
| 领域 | `InfraOptions{}`；YAML 开 db，按需 redis/kafka/redis_stream/consul，通常关 auth | Redis 供幂等/锁等；业务层不直接访问 Redis 细节 |
| 带消息消费的领域 | 上栏 + `AddCustomPlugin(消费者插件)` | 消费启停由自定义插件与配置控制 |

各组件是否启动**一律以对应配置段为准**（如 `db.enabled`、`auth.enabled`、`event.publisher_enabled` / `event.consumer_enabled`）；缺依赖时由插件 Init / 构造函数 Fail Fast。

**形态裁剪参考（改配置即可，不必改 gitee.com/hq_project/yomi-boot 源码）**：

| 形态 | 建议启用 | 建议关闭 / 暂缓 |
| ---- | -------- | --------------- |
| 轻量单体 | `db`；按需 `redis`、`auth`、`pprof` | `consul`、`kafka`、`redis_stream`、`dtm`、`event`、`http_record` |
| 大单体 | 上栏 + 按需 `redis` 幂等/锁、`cronjob` | 无跨进程时不必开 `consul`/`dtm`；异步消息有需求再开 `kafka` 或 `redis_stream` |
| 分布式微服务 | `db`/`redis`/`consul`；入口编排开 `auth`；跨域写开 `dtm`；异步按需 `kafka`/`redis_stream`/`event` | 按服务类型裁剪（领域通常关 `auth`） |

说明：开关名以各配置段为准（如 `db.enabled`、`auth.enabled`、`consul.enabled`、`kafka.enabled`、`redis_stream.enabled`、`dtm.enabled`、`event.publisher_enabled` / `event.consumer_enabled`、`pprof.enabled`、`http_record.enabled`）。能力裁剪权威说明见 [Go配置规范](./Go配置规范.md)。

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
- 领域**写**接口仅 Action/Compensate 成对；入口编排 **不得**直调写路径。详见 [微服务调用规范](../api/微服务调用规范.md) §3.1.1。
- 链路 ID 经 HTTP Header `X-Trace-ID` 传播（`gitee.com/hq_project/yomi-boot/traceid`），**不在** JSON Body 信封中传递。
- 微服务间请求/响应契约见 [微服务调用规范](../api/微服务调用规范.md) §3（权威）。
- 对外出口编码适配见 [API编码适配规范](../api/API编码适配规范.md)（**不**规定 HTTP 方法；方法以本节约定为准）。
- 路径中资源段使用**单数名词**，禁止无意义复数。

### 3.7 中间件链

`DefaultMiddlewares` 统一装配下列链；`token.AuthMiddleware` / `httprecord` / access 均在中间件内按各自 `enabled` 决定是否生效。

```text
CORS → traceid → httprecord → AccessMiddleware → requestguard → Recovery → token.AuthMiddleware → handler
```

- **顺序约束**：`httprecord` 与文件 `AccessMiddleware` 必须位于 `requestguard` **外侧**。`requestguard` 在 `c.Next()` 返回后才可能写出 `REQUEST_TIMEOUT`；若采集层更靠内，会在超时信封写出前落盘，造成审计/access 的 `StatusCode`/`BizCode` 失真。
- `auth.enabled=true`（入口编排）时校验 Bearer Token；未配置或 `false`（领域）时直接放行。
- `traceid` 中间件从 Header `X-Trace-ID` 读取或生成 trace ID，写入 Context；出站 `gitee.com/hq_project/yomi-boot/rpc` 自动回填 Header。
- 领域进程鉴权关闭后不校验令牌；身份字段由入口编排按接口 DTO 显式传入 Body（单体共进程时可按模块边界自行约定）。
- 入口编排**对外**业务响应：经已注册 Encoder 写出；HTTP 状态与 JSON 形状由 Encoder 决定，见 [API编码适配规范](../api/API编码适配规范.md)。
- 领域**读**（微服务间）：对内信封与 HTTP 状态见 [微服务调用规范](../api/微服务调用规范.md) §3.3.1。
- 领域 **DTM Action/Compensate** 回调契约见 [微服务调用规范](../api/微服务调用规范.md) §3.3.2（`gitee.com/hq_project/yomi-boot/dtm`）。

### 3.8 pprof 排障

- 配置项：`pprof.enabled`（默认 `false`）、`pprof.addr`（独立 debug 端口，由各可部署单元自行配置，避免端口冲突）。
- 由 `pprof/plugin` 在 `MustInitInfra` 阶段按配置启动；进程退出时随进程自然关闭，无需显式释放。
- 常用命令：
  - `go tool pprof http://{host}:{pprof_port}/debug/pprof/profile?seconds=30`
  - `go tool pprof http://{host}:{pprof_port}/debug/pprof/heap`
  - `go tool pprof http://{host}:{pprof_port}/debug/pprof/goroutine`

### 3.9 配置约定

环境文件命名统一为 `config/local.yml`、`config/dev.yml`、`config/prod.yml`。`gitee.com/hq_project/yomi-boot/config.ResolvePath()` 默认从 `config/` 目录加载，根据 `YOMI_CONFIG`（`local`/`dev`/`prod`，**默认 `dev`**）选择文件。

`dev.yml` / `prod.yml` 中中间件连接参数（host、port、user、password 等）使用 `${YOMI_*:默认值}` 占位符；`gitee.com/hq_project/yomi-boot/config.Load` 加载时展开，未设置的环境变量取冒号后默认值。`local.yml` 仍可直接写本地默认值。

```yaml
# 入口编排 config/local.yml 示意
consul:
  addr: "127.0.0.1:8500"
  service_name: "{bff}"

dtm:
  server: "http://localhost:36789/api/dtmsvr"

auth:
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
# 通常无 auth、无下游 client
```

### 3.10 质量门禁（depguard）

- `gitee.com/hq_project/yomi-boot` 不得 import 业务可部署单元代码。
- client 契约模块不得 import 可部署单元 `internal`，也不得向 `gitee.com/hq_project/yomi-boot` 注入业务代码。
- 领域 `internal/biz` 不得 import 其他领域可部署单元代码；不得 import 入口编排层。
- 入口编排通过 client 契约模块调用领域，不得 import 领域 `internal/biz`。
- 分层 depguard 细则见 [Go业务分层规范](./Go业务分层规范.md) §3.8。

---

## 4. 检查清单

按形态裁剪适用项：单体不强制拆多进程或启用 Consul/DTM；分布式按服务类型勾选。

- [ ] 业务可部署单元经标准 `require gitee.com/hq_project/yomi-boot`（本地可用 `go.work` / `replace`）引用本库；`import` 为 `gitee.com/hq_project/yomi-boot/...`
- [ ] `gitee.com/hq_project/yomi-boot` 不依赖任何业务可部署单元代码
- [ ] 每个可部署单元 `internal/bootstrap/` 以 `server.go` 为装配入口；消息消费者插件单独文件
- [ ] 启动链路使用 `MustInitInfra` / `MustBuildServer` / `runner.Run`；启动失败 Fail Fast（panic）
- [ ] 未使用的基础设施能力已在 YAML 中关闭对应 `enabled`
- [ ] 消息/平台消费的业务逻辑在可部署单元自定义插件中实现；gitee.com/hq_project/yomi-boot 仅提供消费端点
- [ ] 分布式形态：领域进程无跨域 HTTP、无 token 中间件
- [ ] 入口编排含 `handler/`、`bff/`、`saga/`（若属该服务类型）
- [ ] 跨进程契约符合 [微服务调用规范](../api/微服务调用规范.md) §3（若启用）
- [ ] 链路 ID 经 Header `X-Trace-ID` 传播，非 Body 字段
- [ ] 启用 DTM 时 Action/Compensate 路由成对，补偿路由为正向路由 + `/compensate`
- [ ] 业务分层符合 [Go业务分层规范](./Go业务分层规范.md)

---

## 5. 禁止项

- 业务以独立工程 + 可部署单元组织；禁止在业务工程根目录放置业务 `internal/biz` 或业务装配入口（应落在各可部署单元内）。基于 gitee.com/hq_project/yomi-boot 的单进程部署仍属合法形态（见 §3.1.0）。
- 禁止领域进程之间 HTTP 互调（分布式形态；单体进程内模块调用不受此限）。
- 禁止把业务签发/校验策略放入 `gitee.com/hq_project/yomi-boot/token`（gitee.com/hq_project/yomi-boot 仅提供库；鉴权方在 config 中配置 `auth` 段）。
- 禁止服务间默认使用 gRPC 等其他 RPC 协议替代 HTTP（分布式形态）。
- 禁止在 JSON Body 中传递 `traceId`（统一使用 Header `X-Trace-ID`）。
- 禁止在领域 handler/usecase 中读取 token 或 `SessionFromContext`（分布式形态下的领域服务类型）。
- 禁止在 `wireHandlers` 或 gitee.com/hq_project/yomi-boot 内置插件中编写业务消费逻辑（应通过 `AddCustomPlugin` 注册）。
- 禁止在 `internal/bootstrap/` 下存在 `wire/` 子目录或独立 `wire.go`。
