# Go配置规范

---

## 0. 文档信息
- **文档名称**: Go配置规范
- **描述**: 定义 gitee.com/hq_project/yomi-boot 配置加载抽象、段注册、严格根 key 策略、业务扩展约定，以及通过配置段 `enabled` 按需裁剪基础设施能力（支撑轻量单体至分布式微服务）。
- **作者**: zwei
- **创建日期**: 2026-07-18
---

## 1. 文档定位

- **用途**：约束配置所有权、注册加载流程与扩展方式，避免业务配置污染 `gitee.com/hq_project/yomi-boot`；说明能力按需裁剪的配置入口。
- **边界**：架构总览与部署形态见 [Go架构规范](./Go架构规范.md)；本文件是配置专题权威说明。
- **适用对象**：`gitee.com/hq_project/yomi-boot/component/core/config`、`gitee.com/hq_project/yomi-boot/boot`、`gitee.com/hq_project/yomi-boot/boot/server`、`gitee.com/hq_project/yomi-boot/plugin`、`gitee.com/hq_project/yomi-boot/plugin/builtin`，以及业务工程中各可部署单元的 `config` 与 `internal/bootstrap`、`internal/config`（惯例目录示例：`services/{unit}/`）。

---

## 2. 设计目标

1. **配置分层**：组件库保留运行时 `Config`（如 `db.Config`、`token.Config`），不写 `mapstructure`；所有部署 YAML 绑定统一为 `BootConfig`（`SectionKey`、`BootConfig`、`ConfigSection`、`ToConfig`），放在 `plugin/builtin/{lib}`（`cors`、`router` 也各占一个绑定包，即使没有生命周期插件实现）。
2. **薄配置内核**：`gitee.com/hq_project/yomi-boot/component/core/config` 只提供加载 / 注册 / 校验编排（`Section`/`Registry`/`Config`/`BoolOr` 等）。
3. **主动注册**：`plugin/builtin.DefaultConfigs()` 返回全部内置配置段，`boot.MustInit` 用 `config.NewRegistry()` 装入；业务服务经 `plugin.PluginOptions.AddConfigSection` 追加。
4. **Fail Fast**：加载与校验失败即 `Must*` panic。
5. **未知根 key 严格失败**：YAML 根层未注册 key 一律拒绝。
6. **按需裁剪**：各能力插件是否真正启动由配置段开关控制，不强制引入未用中间件。

---

## 3. 原则

| # | 原则 |
|---|------|
| P1 | 谁实现能力，谁拥有运行时配置语义与 Validate；YAML 绑定与 `SectionKey` 归 `plugin/builtin/{lib}` |
| P2 | `gitee.com/hq_project/yomi-boot/component/core/config` 机制层；组装根在 `boot` |
| P3 | `gitee.com/hq_project/yomi-boot` 禁止承载无对应能力包的产品/领域配置 |
| P4 | 业务配置由服务经 `plugin.PluginOptions.AddConfigSection` 注册，推荐根 key 为 `biz` |
| P5 | 注册显式、可测；禁止 `init()` 副作用注册 |
| P6 | YAML 根 key 集合 ⊆ 注册表 key 集合（严格） |
| P7 | 严格只约束根段；段内未知字段可忽略 |
| P8 | 未使用的能力应关闭对应 `enabled`；切换部署形态优先改配置，不改 gitee.com/hq_project/yomi-boot 源码 |

---

## 4. 核心抽象

| 类型/API | 职责 |
|----------|------|
| `Section` | `Key` / `Target` / `Validate(cfg)` |
| `SectionKey` | `plugin/builtin/{lib}` 导出的 YAML 根 key 常量；`NewSection` / `MustSection` 禁止魔法字符串 |
| `Registry` | `Register` / `Has` / `Keys` |
| `Config` | 已绑定段；`MustSection[T]` |
| `NewSection` | 指向已有 target 的段工厂 |
| `LoadConfig` / `MustLoadConfig` | 严格根 key → Bind → Validate |
| `DefaultConfigs` | `plugin/builtin` 根包：返回全部 `plugin/builtin/{lib}` 的 `ConfigSection()` |
| `PluginOptions.AddConfigSection` | 业务在默认内置配置段后追加 |
| `plugin.PluginSlot` | 统一插件插槽：`Config` + 有序插件链；配置统一经 `Config` + `MustSection` 读取 |
| `PluginSlot.Config` | 启动后可取 boot 段与自定义段 |

### 4.1 加载流程

```text
读文件 → ExpandEnv → viper + AutomaticEnv
→ 解析 YAML 根 key，与注册表做差集（非空则失败）
→ 按注册顺序 UnmarshalKey
→ Section.Validate(cfg)
→ 返回 Config；`NewPluginSlot(Config, 插件链)` → app 插件提供 AppCtx → 各插件读自己的 BootConfig、ToConfig 后在自身字段构造组件；后序插件经 slot 查找前序插件访问器；HTTP 装配在 `boot/server` 读 `plugin/builtin/cors` / `plugin/builtin/router` 的 BootConfig 与插件访问器
```

### 4.2 未知根 key

- **策略：严格失败**。
- 文案：`配置存在未注册段: a, b`。
- 仅检查 YAML **根层** key。

---

## 5. 业务扩展（用法）

```go
// services/{bff}/internal/bootstrap/server.go
var pluginOpts plugin.PluginOptions
pluginOpts.AddConfigSection(svcconfig.BizSection()) // key: biz
slot := boot.MustInit(path, "YOMI_{BFF}", pluginOpts)
defer slot.Close()
biz := config.MustSection[svcconfig.BizConfig](slot.Config, svcconfig.SectionKey)
```

YAML：

```yaml
biz:
  perm:
    unknown: deny
    skip_permission_paths: [...]
```

---

## 6. 能力按需裁剪（enabled）

- gitee.com/hq_project/yomi-boot 各基础设施能力通过对应配置段启用；未启用则插件不启动（或写入 Noop），**不强制**引入 Consul / Kafka / Redis Stream / DTM 等运行时依赖。
- 插件在 `plugin/builtin` 根包中按固定顺序登记；**Init 是否真正启动由各自配置决定**（见 `gitee.com/hq_project/yomi-boot/plugin/builtin/plugins.go`）。
- 典型开关（以各配置段字段名为准）：
  - `db.enabled`、`redis.enabled`、`token.enabled`
  - `consul.enabled`、`kafka.publisher_enabled` / `kafka.consumer_enabled`、`redis_stream.enabled`、`dtm.enabled`
  - `event.outbox_enabled`、`event.inbox_enabled`
  - `pprof.enabled`、`http_record.enabled`、`oss.enabled`
  - `cors.enabled`（默认 true；历史无开关即启用，现允许关闭；不需要 CORS 的新服务应显式设为 false）
  - `idempotency.stale_cleaner_enabled` 等同类开关
- **默认值语义**：历史无开关的可裁剪能力新增 `enabled` 时，`*bool` 的 nil 必须解释为 true（如 `cors.enabled`）；已有 `enabled` 的能力保持原默认值（大多为 false）。强制启动项（`app`/`http`/`log`/`snowflake`）不提供 `enabled`。禁用某段时跳过本段语义校验；中间件类能力仍挂载在链上，由运行时 `MiddlewareEnabled` 短路。
- 部署形态（轻量单体 / 大单体 / 分布式微服务）的推荐开关组合见 [Go架构规范](./Go架构规范.md) §3.1.0 与 §3.4.5。
- **裁剪原则**：先关后开；业务未用到的中间件不要为了「对齐样板」而全部 `enabled: true`。切换形态优先改 YAML，不改 gitee.com/hq_project/yomi-boot 源码。

---

## 7. 明确不做什么

- 不做配置热更新 / 远程配置中心。
- 不做段内未知字段全局严格模式。
- 不做包级 `init()` 全局注册表。
- 不在 `gitee.com/hq_project/yomi-boot` 定义统一 `BusinessConfig` 接口。
- 不做跨段 `CrossRule`；组合约束留给启动期依赖链 Fail Fast。
- 配置校验禁止穿透库边界（如读取 `http.router_prefix` 校验本段 skip 路径）。

---

## 8. 决策记录

| 决策 | 结论 |
|------|------|
| 扩展方式 | 主动注册（默认 boot + 服务追加） |
| 业务根 key | 推荐 `biz` |
| YAML 绑定归属 | `plugin/builtin/{lib}`（`cors`/`router` 为纯绑定包）；组件库运行时 `Config` 不带 `mapstructure` |
| 校验 | BootConfig 校验 YAML 形状并委托运行时 `Config.Validate`；组件库只校验运行时不变量；禁止 ConfigSection 闭包读取其他库配置 |
| 未知根 key | **严格失败** |
| 权限配置 | BFF `internal/config` 的 `biz.perm`，不进 gitee.com/hq_project/yomi-boot |
| 能力裁剪 | 各插件读本段 `enabled`（及同类开关）；形态切换以配置为准 |
| 历史无开关的可裁剪能力 | 新增 `BootConfig.Enabled *bool`，nil 默认 true；禁用时跳过本段校验，中间件内短路 |
| 强制启动项 | `app`/`http`/`log`/`snowflake` 不提供 `enabled`；`plugin/builtin/router` 的 `BootConfig` 只做映射 |
| 中间件启停 | 中间件始终挂载，由运行时 `MiddlewareEnabled` 短路；不在 `boot` 动态增删 |
