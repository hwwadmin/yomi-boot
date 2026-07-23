# Go选型规范

---

## 0. 文档信息
- **文档名称**: Go选型规范
- **描述**: 定义基于 gitee.com/hq_project/yomi-boot 的 Go 后端默认技术栈基线，覆盖 HTTP、Consul、DTM、配置、日志、数据库、缓存与依赖注入等基础技术决策。
- **作者**: zwei
- **创建日期**: 2026-05-28

---

## 1. 文档定位

- **用途**：作为**基于 gitee.com/hq_project/yomi-boot 的业务工程** Go 后端技术选型基线，统一 HTTP、配置、日志、持久化、缓存、消息、性能剖析、测试和依赖注入等基础技术决策。
- **边界**：不替代模块目录划分、数据库表结构和 API 出口编码适配等专题规范；相关内容应分别参考对应知识文档。
- **适用对象**：基于 gitee.com/hq_project/yomi-boot 的 Go 后端可部署单元、基础设施适配代码、技术预研和代码评审场景。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于**基于 gitee.com/hq_project/yomi-boot 的业务工程**中新建 Go 后端服务的 HTTP、配置、日志、数据库、缓存、消息和性能剖析等基础技术栈选型。
- 适用于 HTTP 服务、定时任务、消费程序、命令行进程等服务类型的基础技术栈选型。
- 适用于既有 Go 服务按本规范收敛技术栈的场景。
- 不适用于前端工程、Java 工程或与 Go 后端无关的工具脚本场景。

### 2.2 关联文档

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| `_prompts/doc/knowledge/知识库文档规范.md` | 文档基线 | 约束本规范及后续 Go 知识文档的结构与索引登记方式 |

### 2.3 阅读建议

- 新建 Go 服务前，先阅读“选型原则”和“默认技术栈总览”，先确定默认栈，再决定是否需要少量例外。
- 设计 HTTP 服务时，重点阅读“HTTP 服务”“配置”“日志与性能剖析”章节。
- 设计数据访问、消息适配和外部调用时，重点阅读“数据库与缓存”“外部调用、序列化与消息”“不作为默认选型的技术”章节，避免误引入第二套 ORM 和大框架。

---

## 3. 技术方案

### 3.1 选型原则

#### 3.1.1 原生优先

- 能用标准库解决的问题，默认不引入第三方库。
- 标准库已经覆盖的能力，例如 HTTP Client、JSON、日志、上下文、并发控制，应优先使用原生实现。
- 当标准库能力不足以支撑生产级需求（如 HTTP 路由分组、中间件链、参数自动绑定、ORM 模型映射等）时，引入社区成熟、生态稳定的第三方库。
- 第三方库只用于标准库明显缺失、且收益稳定清晰的场景。

#### 3.1.2 轻量框架优先，避免全家桶

- 默认选择职责单一、可独立替换的轻量框架或库，不选择把路由、配置、ORM、日志、依赖注入全部绑在一起的全家桶大框架。
- gin 和 gorm 各自只覆盖一个领域（HTTP / 持久化），不互相绑架，符合可组合的选型原则。
- 选型目标是让代码能被普通 Go 开发者直接读懂，而不是依赖框架约定才能运行。

#### 3.1.3 显式优先

- 配置来源、超时、重试、SQL、依赖注入、序列化规则都应显式写出，不依赖隐式约定或全局自动装配。
- 不使用反射式 DI 容器；依赖装配保持显式可读。

#### 3.1.4 低魔法与低耦合

- 不默认引入第二套 ORM、第二套 Web 框架、第二套配置库——每种能力只保留一个默认选型。
- gorm 的 tag 映射属于 ORM 必要的声明式元数据，不视为过度魔法。
- 选型的基础库应保持可替换，不因选型锁死架构。

### 3.2 默认技术栈总览

| 类别 | 默认选型 | 选型理由 | 不作为默认选型 |
| ---- | -------- | -------- | -------------- |
| Go 版本 | `Go 1.26+` | 统一团队运行时基线，兼容当前默认技术栈与标准库能力 | 低于 `Go 1.26` 的运行时基线 |
| HTTP 服务 | `github.com/gin-gonic/gin` | 社区最广泛使用的 Go HTTP 框架，路由、中间件、参数绑定、响应输出等能力成熟完善 | `echo`、`fiber`、`net/http` 作为默认入口框架 |
| 配置 | `github.com/spf13/viper` | 统一管理 yml、环境变量和命令行覆盖，并保留强类型配置结构与显式初始化流程 | `koanf`、`envconfig` 或多套配置库并存 |
| 日志 | `go.uber.org/zap` | 高性能结构化日志，支持双路输出（控制台 + JSON 文件）、独立级别控制与调用点定位 | `log/slog`、`logrus` 作为默认日志入口 |
| JSON | `encoding/json` | 标准库、兼容性稳定、行为可预期 | `jsoniter`、`sonic` 作为默认 JSON 库 |
| HTTP Client | `net/http.Client` + 自定义 `Transport` | 原生、显式控制超时、连接池、重试策略 | `resty` 作为默认 HTTP 调用库 |
| 数据库 | MySQL | 社区成熟、生态稳定、事务与索引能力完整，适合作为默认关系型数据库 | PostgreSQL、SQLite 等作为默认数据库基线 |
| SQL 访问 | `gorm.io/gorm` + `gorm.io/driver/mysql` | 社区最广泛使用的 Go ORM，提供模型映射、自动迁移、链式查询、事务、预加载等完善能力 | `sqlx`、`xorm` 等作为默认持久化方案 |
| 本地事务控制 | `gorm.io/gorm` 的 `Transaction` 方法 | 与默认 SQL 访问栈保持一致，闭包式事务边界清晰，自动回滚 | 额外事务管理框架或隐藏式事务装饰器 |
| Redis | `github.com/redis/go-redis/v9` | 事实标准客户端，能力完整，社区稳定 | 二次包装缓存框架作为默认选型 |
| MQ | Kafka + `github.com/segmentio/kafka-go` | Kafka 生态成熟，适合事件流、异步解耦和消费组模型；`kafka-go` 轻量直接、易于与当前分层整合 | RabbitMQ、NATS、Pulsar 作为默认 MQ 基线 |
| 分布式锁 | `github.com/go-redsync/redsync/v4`（经 `gitee.com/hq_project/yomi-boot/distlock` 封装） | 基于 Redis 的分布式锁，支持看门狗、超时、可选 Key 命名空间 | 其他分布式锁实现 |
| 业务日历定时任务 | `github.com/robfig/cron/v3`（经 `gitee.com/hq_project/yomi-boot/cronjob` 封装） | cron 表达式触发 + distlock 集群互斥；与 runner/`ctx` 同生命周期 | 业务模块直接依赖 `robfig/cron`；用 `roundrobin` 冒充日历任务 |
| HTTP RPC | `gitee.com/hq_project/yomi-boot/rpc`（`gitee.com/hq_project/yomi-boot/rpc` 包） | 轻量 HTTP RPC 读封装：Consul 发现、JSON 信封、传输层短退避重试与 per-service 熔断；跨域写重试由 DTM 承担 | 直接使用 `net/http` 作为默认 RPC 入口；读写混用一套复杂韧性框架 |
| 并发控制 | `context` + `sync` + `golang.org/x/sync/errgroup` | 生命周期和错误传播清晰，接近原生模型 | 自定义协程框架 |
| 日志 | `go.uber.org/zap` | 结构化日志，控制台 + JSON 文件双路输出 | `log/slog`、`logrus` 作为默认入口 |
| 性能剖析 | `net/http/pprof`（经 `gitee.com/hq_project/yomi-boot/pprof` 封装） | 独立 debug 端口，CPU/内存/goroutine 排障 | OpenTelemetry、`prometheus/client_golang`、黑盒观测 SDK |
| 测试 | `testing` + `httptest` + `github.com/google/go-cmp/cmp` + `testcontainers-go` | 标准库优先，最小辅助库补齐对比和集成测试 | 大量 mock 框架作为默认测试方案 |
| DI | 手写 Wiring，各服务 `internal/bootstrap/server.go` 组合根 | 依赖关系显式、调试简单、与 Go 风格一致 | `fx`、`dig`、`github.com/google/wire` 等 DI 框架 |
| 服务注册发现 | Consul | 微服务注册、健康检查、HTTP 服务发现 | Eureka、Nacos 作为默认方案 |
| 分布式事务 | DTM（Saga） | BFF 编排跨域写，领域本地子事务 + barrier | 2PC、TCC、Outbox/Inbox 作为默认方案 |
| 服务间通信 | HTTP/HTTPS + `net/http` | 兼容性好、调试简单、与 BFF/client 分层一致 | gRPC 作为服务间默认协议 |
| 参数校验 | 显式代码校验，必要时局部补充轻量校验库 | 规则位置清楚，不依赖 tag 魔法驱动业务 | 大范围 tag 反射校验框架作为默认方案 |
| 精度小数 | `github.com/shopspring/decimal` | 金额等需精确表示的数值场景必须使用 Decimal，避免浮点精度丢失 | `float32`、`float64` 用于金额/数量等精确计算场景 |

### 3.3 默认选型细则

以下对 §3.2 中的关键选型补充选型理由、配套说明及选型边界，不涉及具体编码方式与分层架构。

#### 3.3.1 HTTP 服务

- 默认选型：`github.com/gin-gonic/gin`。
- 选型理由：社区最广泛使用的 Go HTTP 框架，路由、中间件、参数绑定、响应输出能力成熟，且不绑架配置、ORM 和日志体系。
- 配套驱动：无额外驱动依赖，gin 自身即完整 HTTP 框架。
- 不作为默认：`echo`、`fiber`、`net/http`（避免多套 HTTP 框架并存）。

#### 3.3.2 配置

- 默认选型：`github.com/spf13/viper`。
- 选型理由：统一管理 yml、环境变量和命令行覆盖，支持强类型配置结构。
- 配套约束：配置项必须在 yml 中显式声明默认值；环境变量和命令行参数仅用于覆盖，不作为新增配置项的唯一来源。
- 不作为默认：`koanf`、`envconfig`（避免多套配置源口径不一致）。

#### 3.3.3 日志与性能剖析

- 日志默认选型：`go.uber.org/zap`。
- 性能剖析默认选型：`net/http/pprof`（经 `gitee.com/hq_project/yomi-boot/pprof` 提供独立 debug 端口）。
- 选型理由：
  - `zap` 性能高、结构化能力强，适合控制台与 JSON 文件双路输出场景。
  - `pprof` 为标准库能力，零额外依赖，适合 CPU、堆内存、goroutine 泄漏等排障场景。
- 配套约束：
  - 各服务通过 `pprof.enabled` / `pprof.addr` 暴露独立 debug 端口；默认关闭；由 `pprof/plugin` 在 `MustInitInfra` 阶段启动。
  - pprof 端口仅供内网排障，不注册 Consul、不挂业务网关。
  - 跨服务请求关联依赖 Header `X-Trace-ID`（`gitee.com/hq_project/yomi-boot/traceid`）+ 结构化日志，不引入分布式 tracing 平台。
- 不作为默认：`log/slog`、`logrus`；OpenTelemetry、`prometheus/client_golang`、Grafana/Jaeger 等观测平台（当前阶段不建设）。

#### 3.3.4 数据库与缓存

- 数据库引擎：MySQL。
- SQL 访问默认选型：`gorm.io/gorm` + `gorm.io/driver/mysql`。
- 选型理由：社区最广泛使用的 Go ORM，提供模型映射、链式查询、事务、预加载等能力。
- 本地事务：使用 `gorm.DB.Transaction`，与 SQL 访问栈一致。
- Redis 默认选型：`github.com/redis/go-redis/v9`。
- 不作为默认：PostgreSQL（数据库引擎）；`sqlx`、`xorm`（SQL 访问）；额外事务框架。

#### 3.3.5 外部调用、服务发现与分布式事务

- **服务间通信**：HTTP/HTTPS only；BFF 经**领域 client**（业务工程独立模块，路径惯例 `{client}/{domain}`，基于 `gitee.com/hq_project/yomi-boot/rpc`）调用领域服务，**默认不使用 gRPC**。
- HTTP Client：`gitee.com/hq_project/yomi-boot/rpc` 封装 POST JSON + Consul 发现；由领域 client 使用，信封口径见 [微服务调用规范](../api/微服务调用规范.md) §3.2。
- JSON：`encoding/json`（标准库）。
- **服务注册发现**：Consul；各服务启动注册，BFF 按服务名发现下游。
- **分布式事务**：DTM Saga；BFF 提交 Saga，领域服务在 barrier 下执行本地子事务。
- MQ：Kafka + `github.com/segmentio/kafka-go` 保留在 `gitee.com/hq_project/yomi-boot/kafka/` 作为可选基础设施，**非默认业务通信方式**；单元测试通过 `gitee.com/hq_project/yomi-boot/kafka/kafkatest` 封装（底层 `franz-go/kfake` 启动内存 broker），**franz-go 不得进入生产代码路径**。
- 不作为默认：`resty`（HTTP Client）；`jsoniter`/`sonic`（JSON）；gRPC 服务间调用；Outbox/Inbox 事件驱动分布式事务。

#### 3.3.6 测试与质量工具

- 默认选型：`testing` + `httptest` + `github.com/google/go-cmp/cmp` + `testcontainers-go`。
- 静态检查：`golangci-lint`。
- 不作为默认：大量 mock 框架。

#### 3.3.7 依赖注入与参数校验

- DI：手写 Wiring，各微服务 `internal/bootstrap/server.go` 为组合根；共享基础设施由 `gitee.com/hq_project/yomi-boot/bootstrap.MustInitInfra` 经插件链初始化（Fail Fast），通过显式 `NewXxx` 构造函数链完成业务依赖装配。
- 共享基础设施收口在 `gitee.com/hq_project/yomi-boot` module，各业务服务 `go.mod`：标准 `require gitee.com/hq_project/yomi-boot`（本地可用 `go.work` / 相对路径 `replace`）。
- 参数校验：显式代码校验，必要时局部补充轻量校验库。
- 不作为默认：反射式/代码生成 DI 框架（如 `fx`、`dig`、`github.com/google/wire`）；大范围 tag 反射校验框架。

#### 3.3.8 精度小数

- 默认选型：`github.com/shopspring/decimal`。
- 选型理由：金额、价格、数量等需要精确表示的数值场景必须使用 `decimal.Decimal`，`float32`/`float64` 在运算和序列化中存在精度丢失风险，不适用于金融级精确计算。
- `decimal.Decimal` 原生支持 `json.Marshaler`/`json.Unmarshaler` 和 `sql.Scanner`/`driver.Valuer` 接口，可直接用于 JSON 序列化和 GORM 持久化。
- 不作为默认：`float32`、`float64` 用于金额/数量等精确计算场景。

#### 3.3.9 业务日历定时任务

- 默认选型：`github.com/robfig/cron/v3`（经 `gitee.com/hq_project/yomi-boot/cronjob` 封装）。
- 选型理由：日历表达式触发成熟；集群互斥复用 `gitee.com/hq_project/yomi-boot/distlock`；生命周期仅认 `ctx`，与 runner / plugin 拼装一致。
- 配套约束：业务模块禁止直接依赖 `robfig/cron`；内部固定间隔轮询（Outbox 等）继续使用 `gitee.com/hq_project/yomi-boot/roundrobin`，不得用本能力冒充。
- 不作为默认：各服务自行引入 cron 库、中心化调度框架、用 `roundrobin` 表达日历任务。

### 3.4 不作为默认选型的技术

以下技术不作为默认选型；只有在收益显著、替代方案不足且经过评审确认时，才可局部引入：

| 技术 | 不默认引入原因 |
| ---- | -------------- |
| `echo` / `fiber` | 多套 Web 框架并存会增加维护成本和上下文切换，gin 已满足项目需求 |
| `sqlx` / `xorm` | 多套 SQL 访问库并存会增加维护成本，gorm 已满足项目需求 |
| 第二套配置库 | 与 `viper` 并存会让配置来源、优先级和解码口径失控，增加排障成本 |
| `log/slog` / `logrus` | 与默认 `zap` 日志栈并存会造成输出格式、调用点和级别控制口径不一致 |
| 第二套 MQ 基线 | 默认 MQ 为 Kafka；并存 RabbitMQ、NATS、Pulsar 等消息模型会提高系统复杂度和运维成本 |
| `resty` | 与标准库 HTTP Client 能力重复，默认收益不足以覆盖额外抽象层 |
| `fx` / `dig` / `github.com/google/wire` | 依赖注入链条更隐蔽；组合根已收口在各服务 `internal/bootstrap/server.go` |
| 大范围 tag 校验框架 | 容易把核心校验逻辑埋进 tag 和反射，削弱规则可读性和可调试性 |
| 一体化脚手架框架 | 会同时绑死路由、配置、日志、ORM 和生命周期管理，偏离少框架原则 |

### 3.5 选型例外规则

- 引入不在默认栈中的第三方依赖时，必须说明标准库或默认库为什么不足。
- 例外依赖只能落在外层位置，不得渗透到内层业务代码。
- 例外选型必须给出替换成本、维护成本和退出策略，避免一次性方便变成长期锁定。

---

## 4. 检查清单

- [ ] HTTP 服务默认选型为 `gin`，未引入第二套 Web 框架
- [ ] 数据库引擎与 SQL 访问方案已分开描述，默认口径为 MySQL + `gorm`
- [ ] 本地事务使用 `gorm.DB.Transaction`，未引入额外事务框架
- [ ] 配置使用 `viper`，未并存第二套配置库；所有配置项已在 yml 中显化
- [ ] 日志使用 `go.uber.org/zap`，统一通过 `gitee.com/hq_project/yomi-boot/logger` 输出，未并存 `log/slog`、`logrus` 等第二套日志入口
- [ ] 服务间通信默认 HTTP/HTTPS，未将 gRPC 作为服务间默认协议
- [ ] 服务注册发现使用 Consul；跨域写使用 DTM Saga
- [ ] DI 通过各服务 `internal/bootstrap/server.go` 手写 Wiring，未引入 Google `wire` / `fx` / `dig` 等 DI 框架
- [ ] 未引入全家桶式一体化脚手架框架
- [ ] 例外选型已说明理由并给出维护与退出策略

---

## 5. 禁止项

- 禁止引入全家桶式 Go Web 框架（同时绑死路由、配置、日志、ORM 和生命周期）。
- 禁止把数据库引擎选型和 SQL 访问方案混写为同一概念，导致技术栈边界失真。
- 禁止在默认栈之外再叠加第二套 Web 框架、第二套 ORM、第二套配置库、第二套 MQ 基线或第二套日志入口。
- 禁止引入反射式 DI 框架（`fx`/`dig`/`github.com/google/wire`）作为默认依赖注入方案；依赖注入统一使用各服务 `internal/bootstrap/server.go` 手写组合根。
- 禁止将 gRPC 作为 BFF 与领域服务之间的默认通信协议。
- 禁止引入 Outbox/Inbox 或 2PC/TCC 替代 DTM Saga 作为默认可分布式事务方案。
- 禁止多套不兼容的 Go 后端基础技术栈在同一仓库长期并存。