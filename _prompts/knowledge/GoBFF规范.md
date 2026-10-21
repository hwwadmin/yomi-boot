# GoBFF规范

---

## 0. 文档信息
- **文档名称**: GoBFF规范
- **描述**: 定义入口编排（BFF）服务类型的目录职责、跨域编排、DTM Saga 与令牌注入；微服务信封与幂等细则见微服务调用规范。
- **作者**: zwei
- **创建日期**: 2026-06-17

---

## 1. 文档定位

- **用途**：约束入口编排层 `handler`、`bff`、`saga` 的职责边界与协作方式。
- **边界**：可部署服务类型与 gitee.com/hq_project/yomi-boot 能力见 [Go架构规范](./Go架构规范.md)；领域分层见 [Go业务分层规范](./Go业务分层规范.md)；DTM 子事务见 [Go事务规范](./Go事务规范.md)；**微服务信封与幂等见 [微服务调用规范](./微服务调用规范.md) §3（权威），本文不重复**。
- **适用对象**：入口编排（BFF）服务类型的可部署单元（含单体中的编排模块）。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于所有**入口编排（BFF）**服务类型的可部署单元或其模块。
- 入口编排是唯一允许跨域 HTTP 调用的层（分布式形态）。
- 入口编排是唯一提交 DTM Saga 的层（若启用跨域写）。
- 入口编排是唯一解析 token 并完成鉴权的层（若启用 `auth`）。

### 2.2 关联文档

| 文档 | 关联点 |
| ---- | ------ |
| [Go架构规范](./Go架构规范.md) | 目录、`router_prefix`、中间件、Consul |
| [Go业务分层规范](./Go业务分层规范.md) | BFF 与领域职责对比 |
| [Go事务规范](./Go事务规范.md) | Saga 编排与领域子事务 |
| [Redis设计规范](./Redis设计规范.md) | BFF 令牌 Store 键 |
| [API编码适配规范](./API编码适配规范.md) | BFF 对外 Encoder 适配 |
| [微服务调用规范](./微服务调用规范.md) | BFF→领域 POST/JSON、信封与幂等（权威） |

---

## 3. 技术方案

### 3.1 BFF 内部分层

```text
handler/     对外 HTTP；解析入参；调 bff / saga
bff/         只读多服务聚合，按 usecase 平铺，不进 DTM
saga/        写操作（含单域），按 usecase 平铺，提交 DTM Saga
```

**存储约束**：

- BFF **允许**连接本服务独立 MySQL/PostgreSQL（配置 `db`、经 `gitee.com/hq_project/yomi-boot` 初始化 GORM），用于基础设施表（如 `gitee.com/hq_project/yomi-boot/component/event` 收件箱幂等），库名遵循一服务一库（`yomi_go_{bff}`）。
- BFF **禁止**连接或访问**领域服务数据库**；禁止在 BFF 内建设领域业务表、领域 Repository 或跨库 JOIN。
- 允许使用 **Redis** 作为令牌 Store 与业务缓存（经 `gitee.com/hq_project/yomi-boot/component/redis.RedisService`）。

| 目录 | 职责 | 禁止 |
| ---- | ---- | ---- |
| `handler/` | 路由绑定、鉴权后的协议适配 | 手写 SQL / 直操领域库；写复杂跨域编排 |
| `bff/` | 多服务只读聚合（如 Profile + 角色） | 提交 DTM；写领域数据 |
| `saga/` | 写 Saga 定义与提交（含单域） | 实现领域补偿逻辑 |

> BFF 通过**领域 client**（业务工程独立模块，路径惯例 `{client}/{domain}`，非 gitee.com/hq_project/yomi-boot）调用下游领域服务，通过 `infra.TokenService`（来自 `gitee.com/hq_project/yomi-boot/component/token`）完成令牌签发/校验/撤销。事件 Inbox 等本库访问经 `gitee.com/hq_project/yomi-boot/component/event` 等组件，不在 `handler/` 内散落 SQL。

### 3.2 跨域调用规则

#### 3.2.1 唯一跨域方

- 所有对下游**领域**的 HTTP 调用必须经领域 client 契约模块（路径惯例 `{client}/{domain}`）。
- client 经 Consul 服务名发现，禁止硬编码 IP。

#### 3.2.2 微服务调用契约

- 调领域**一律 POST + JSON**；信封、`bizId`/`bizSubId`、响应回显、幂等与禁止项，**以 [微服务调用规范](./微服务调用规范.md) §3 为唯一权威**（本文不展开）。
- BFF **对外**接口可使用 GET 或 POST（CDN/SEO）；完整 URL = `http.router_prefix`（按端配置）+ handler 相对路径，见 [Go架构规范](./Go架构规范.md) §3.6；出口编码见 [API编码适配规范](./API编码适配规范.md)。

**BFF 侧摘要**（细则见权威文档）：

| 层级 | 职责摘要 |
| ---- | -------- |
| `saga/` | **所有写唯一入口**：提交 DTM Saga；按微服务规范生成并写入信封 |
| 领域 client | **读**：POST 封装；**写**：经 `dtm.AddSagaStep` 追加分支，禁止直调 Action URL |
| 链路 | Header `X-Trace-ID`（不在 Body） |

- **禁止**领域 client 封装绕过 DTM 的领域写直调方法。
- **禁止** `*Facade` / `Direct*` 在 DTM 未配置时回退直调；DTM 不可用则写操作失败。

```text
// 示例：saga/create_resource.go（信封字段生成见微服务调用规范 §3.2）
bizID := idempotency.BizID()
actionSubID := idempotency.BizID()
compensateSubID := idempotency.BizID()
resourceClient.CreateResource(ctx, saga, payload)
saga.Submit()
```

```text
BFF：token 中间件 → Claims → saga 提交 DTM（写）/ 领域 client 读领域（读）
领域：DTM 回调 Action/Compensate → 解析信封 → usecase（幂等见微服务调用规范 §3.4）
```

#### 3.2.3 喂饱领域接口（显式传参）

领域服务不读 token、不跨域补数。BFF 必须把下游接口所需字段**显式写入 Request Body**（与信封同级平铺）；字段名与是否必填**由各接口 DTO 定义**，非全局信封：

| 语义 | 常见字段（按需） | 注入方 |
| ---- | ---------------- | ------ |
| 当前操作者 | `operatorId` | BFF 从 Claims 解析（仅接口需要时传） |
| 被操作主体 | `targetId` / `relId` + `relType` | 前端入参 + BFF 校验 |
| Saga 补偿快照 | `previousStatus` 等 | BFF 前置查询写入 payload |

```go
// 示例：BFF bff/get_resource_profile.go（示意）
claims := token.FromContext(c)
resource, err := b.resourceClient.GetProfile(ctx, claims.SubjectID)
```

### 3.3 令牌（gitee.com/hq_project/yomi-boot/component/token 统一注入）

#### 3.3.1 gitee.com/hq_project/yomi-boot/component/token 统一提供

`gitee.com/hq_project/yomi-boot/component/token` 提供完整的令牌能力，包括 Store（Redis）、Claims、鉴权中间件、`TokenService`（签发/校验/撤销）。BFF 在 `NewServer` 中通过 `infra.TokenService` 直接注入，**无需**在 BFF 内部独立实现。

| 层级 | 内容 |
| ---- | ---- |
| `gitee.com/hq_project/yomi-boot/component/token` | Store 接口、Redis 实现、Claims、`TokenService`、中间件 `AuthMiddleware`、Context 读写 |
| BFF `handler/` | 通过 `infra.TokenService` 使用签发/校验/撤销能力 |

- 多入口时在 `config/*.yml` 的 **`auth`** 段按端配置不同 `store_prefix`；键模式见 [Redis设计规范](./Redis设计规范.md) §3.2.3。
- `auth.enabled=true` 时由 token 插件在 `MustInitInfra` 阶段初始化（依赖 Redis）。

#### 3.3.2 登录 / 登出

**登录（示例）：**

```text
auth_handler → subjectClient.VerifyCredentials
            → infra.TokenService.Issue
            → roleClient.QueryRolesByRel（可选）
            → 返回 token + subject + roleIds
```

**登出：**

```text
auth_handler → infra.TokenService.Revoke
不经角色域
```
**资源注册（Saga 示例）：**

```text
saga/register_resource.go → SubmitSaga({resource} Register Action/Compensate)
Saga 成功后按业务键回查组装响应；登录时再 Issue Token
```

**创建主体（Saga 示例）：**

```text
saga/create_resource.go → SubmitSaga({resource} Create Action/Compensate)
Saga 成功后按业务键回查组装响应
```

### 3.4 bff/ 只读聚合

- 按 **usecase 平铺**，一文件一用例，如**示例** `get_resource_profile.go`。
- 典型模式：并行或串行调多个领域 client，合并为对外 Output。
- **不**提交 DTM；**不**写领域数据库。

### 3.5 saga/ 写操作

- 按 **usecase 平铺**，一文件一 Saga 用例（含 create、update-status、register 等）；**单域写亦经 Saga**（可为单分支）。
- **仅** BFF `saga/` 调用 `gitee.com/hq_project/yomi-boot/cloud/dtm` 提交 Saga；**禁止** BFF 经领域 client 直调领域写接口。
- DTM **必须**配置；`dtmClient.Enabled()` 为 false 时写操作返回 `dtm.ErrNotConfigured`，**禁止** Facade/Direct 回退。
- Saga 定义 Action URL 与 Compensate URL（来自领域 client 的 URL 辅助函数），指向领域成对 Handler。
- BFF 负责组装信封与业务 payload（含补偿快照）；字段规则见 [微服务调用规范](./微服务调用规范.md) §3.2。
- Saga 提交成功后，如需对外响应，可前置查询或按业务键回查，**禁止**再 POST 领域写 Action URL。
- **禁止**在 BFF 实现领域补偿业务逻辑。

```go
// 示例：saga/create_resource.go（示意）
bizID := idempotency.BizID()
actionSubID := idempotency.BizID()
compensateSubID := idempotency.BizID()
payload := createResourcePayload{
    MsRequestEnvelope: dto.MsRequestEnvelope{BizID: bizID, BizSubID: actionSubID},
    CompensateBizSubID: compensateSubID,
    Username: input.Username, Password: input.Password,
}
saga, err := dtm.NewSaga(ctx, s.dtmServer)
if err != nil {
    return nil, err
}
if err := s.resourceClient.CreateResource(ctx, saga, payload); err != nil {
    return nil, err
}
if err := saga.Submit(); err != nil {
    return nil, err
}
return s.resourceClient.FindByUsername(ctx, input.Username)
```

### 3.6 下游客户端（领域 client 契约模块）

**领域 client** 为业务工程独立契约模块（非 gitee.com/hq_project/yomi-boot；业务 Workspace 下通常为 `client/`），**按领域分子包**（`{client}/{domain}/`）。参考实现中的包名随业务域变化，不构成本规范硬性清单。

- **读**：`client.go` 中提供查询/验密等方法，封装 URL、Request/Response DTO、POST 调用。
- **写**：提供 `CreateXxx(ctx, saga, payload)` 等方法，内部经 `dtm.AddSagaStep` 向 Saga 追加 Action/Compensate 分支；**不**直接 POST 领域写 URL。
- **路径共享**：`url.go` 中的路径常量同时被领域 `handler/` 与领域 client 使用，确保路由一致。
- 经 Consul 发现；全部 POST + JSON Body；链路 ID 经 Header `X-Trace-ID` 传播。

### 3.7 典型场景分工

| 场景 | handler | bff | saga | 领域 client | token |
| ---- | ------- | --- | ---- | ------------ | ----- |
| 登录 | ✓ 编排 | — | — | 验密+查角色（读） | Issue |
| 登出 | ✓ | — | — | — | Revoke |
| Profile | ✓ 路由 | ✓ 聚合 | — | 读 | 解析 Claims |
| 创建主体 | ✓ | — | ✓ | URL 辅助 | — |
| 禁用主体 | ✓ | — | ✓ | URL 辅助 | Revoke |
| 分配角色 | ✓ | — | ✓ | URL 辅助 | Revoke |
| 单域查询 | ✓ 转发 | — | — | 读 | — |

### 3.8 多入口编排差异（模式）

当存在多个入口编排服务（BFF）可部署单元时（**示例**：如管理端 / App），按端拆分；具体 `router_prefix` 与下游集合由配置决定：

| 项 | 约定 |
| -- | ---- |
| 路由前缀 | 各入口独立 `http.router_prefix`（按端在配置中设置） |
| 验密对象 | 各入口对应的主体领域 |
| token | 统一 `infra.TokenService`（`gitee.com/hq_project/yomi-boot/component/token`），`auth.store_prefix` 按端隔离 |
| 下游领域 client | 仅依赖本入口需要的 `{client}/{domain}` 包 |

---

## 4. 检查清单

- [ ] 跨域 HTTP 仅出现在 client（读）与 `saga/`（写经 DTM）
- [ ] 微服务信封与幂等符合 [微服务调用规范](./微服务调用规范.md) §3
- [ ] 入口编排对外 API 可使用 GET（不受微服务契约限制）
- [ ] 登录/登出/token 撤销经 `infra.TokenService`，不经角色域
- [ ] 只读聚合在 `bff/`，所有写在 `saga/`
- [ ] 所有写操作经 DTM；无 Facade/Direct 回退
- [ ] client 写侧经 `dtm.AddSagaStep` 追加 Saga 分支，无绕过 DTM 的写直调方法
- [ ] Saga 不实现领域补偿，只编排 URL 与 payload
- [ ] 多入口使用独立 `store_prefix`
---

## 5. 禁止项

- 禁止 BFF 直连**领域服务**数据库；禁止在 BFF 库中存放领域业务主数据（本服务基础设施表除外）。
- 禁止 BFF 以 GET 调用领域服务。
- 禁止 BFF 领域 client 直调领域写接口。
- 禁止 `*Facade`、`*Direct*` 或 DTM 未配置时的写操作回退路径。
- 禁止通过 JSON Body 向领域传递 `traceId`（统一使用 Header `X-Trace-ID`）。
- 信封与幂等相关禁止项见 [微服务调用规范](./微服务调用规范.md) §5（不在此重复）。
- 禁止在 `saga/` 或 `client/` 实现领域 Compensate 业务逻辑。
- 禁止领域服务解析 BFF 令牌。
- 禁止 BFF 使用 gRPC 调领域。
- 禁止在 `handler/` 内写大段跨域编排（应下沉 `bff/` 或 `saga/`）。
