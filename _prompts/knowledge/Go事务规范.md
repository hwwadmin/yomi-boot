# Go事务规范

---

## 0. 文档信息
- **文档名称**: Go事务规范
- **描述**: 定义领域服务本地事务控制、DTM Saga 子事务、usecase Execute/Compensate 与 Handler Action/Compensate 分工规范。
- **作者**: zwei
- **创建日期**: 2026-06-17

---

## 1. 文档定位

- **用途**：约束领域 usecase 层事务边界、DTM barrier 下的本地子事务、以及 BFF 与领域在分布式写场景下的职责划分。
- **边界**：Saga 编排见 [GoBFF规范](./GoBFF规范.md)；repository 事务感知见 [Go数据库规范](./Go数据库规范.md)；`TxExecutor` 实现位于 `gitee.com/hq_project/yomi-boot/component/db`；**信封与业务幂等细则见 [微服务调用规范](./微服务调用规范.md) §3.4，本文不重复**。
- **适用对象**：领域写 usecase、DTM 分支 Handler、BFF `saga/` 实现与评审。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 领域服务内需要原子性的本地数据库操作。
- BFF 发起的跨域写，通过 DTM Saga 协调多服务本地子事务。
- 不适用于纯查询；不适用于 BFF 令牌操作（Redis Store）；BFF 本库仅用于基础设施（如 event Inbox），领域写仍经 Saga 下沉领域服务。

### 2.2 关联文档

| 文档 | 关联点 |
| ---- | ------ |
| [Go架构规范](./Go架构规范.md) | DTM、gitee.com/hq_project/yomi-boot/cloud/dtm |
| [GoBFF规范](./GoBFF规范.md) | BFF 提交 Saga、payload 喂饱 |
| [Go业务分层规范](./Go业务分层规范.md) | usecase/handler 分层 |
| [Go数据库规范](./Go数据库规范.md) | repository 事务感知 |
| [微服务调用规范](./微服务调用规范.md) | 信封、bizId、bizSubId、sys_biz 幂等 |

---

## 3. 技术方案

### 3.1 分布式事务总览

```text
BFF saga/          提交 DTM Saga，定义分支顺序与 payload
       ↓ POST/JSON（bizId、bizSubId 在 Body；链路 ID 在 Header）
DTM Server           调度各分支 Action / Compensate
       ↓
领域 handler         Action 或 Compensate 独立入口
       ↓
领域 usecase         Execute 或 Compensate（同文件）+ barrier + 本地子事务
       ↓
domain / repo        无感知 DTM
```

- **默认方案**：DTM Saga + 各服务本地子事务（barrier 保证幂等）+ `gitee.com/hq_project/yomi-boot/component/idempotency` 业务幂等。
- **可选方案**：`gitee.com/hq_project/yomi-boot/component/event` Outbox/Inbox（`event.outbox_enabled` / `event.inbox_enabled` 启用时）；当前领域服务默认未开启。

### 3.2 各层事务职责

| 分层 | 职责 | 禁止 |
| ---- | ---- | ---- |
| **BFF saga/** | 提交 Saga；组装 payload（信封 + 补偿快照）；**唯一写入口** | 操作领域 DB；直调领域写 HTTP；Facade/Direct 回退 |
| **领域 handler** | Action 调 `Execute`；Compensate 调 `Compensate`；**仅** DTM 回调入口 | 暴露 BFF 可直调写路由；`BindAndHandle` 写接口；同一 URL `IsCallback` 分流 |
| **领域 usecase** | `idempotency.Execute` + barrier + `TxRunner` 本地子事务 | 跨域 HTTP；读 token |
| **service** | 不感知事务 | 接收 `*gorm.DB` 事务参数 |
| **repository** | `db.Exec(ctx, r.DB)` 感知 Context 事务 | 自行 Begin/Commit |

### 3.3 本地事务（usecase 内部）

- 领域写 usecase 通过 `idempotency.Execute(guard, txRunner, ctx, key, fn)` 包裹；`txRunner` 类型为 `idempotency.TxRunner`。
- 本地事务使用 `idempotency.NewDBTxRunner(infra.TxExecutor)`，内部经 `gitee.com/hq_project/yomi-boot/component/db` 包的 `TxExecutor` 管理 GORM 事务。
- 参与 DTM Saga 分支时传入 `*dtm.TxRunner`；它满足 `idempotency.TxRunner`，由 `cloud/dtm` 配合 barrier 执行。`component/idempotency` 与 `cloud/dtm` 之间没有包依赖。
- 需要写事务的 usecase 构造函数注入 `*idempotency.Guard` 与 `idempotency.TxRunner`，不要写死 `*dtm.TxRunner`。
- 业务结果通过闭包变量捕获，不在事务闭包内组装 HTTP Output。
- **上述事务边界仅用于 usecase 内部**；**禁止**据此在领域 handler 再暴露一条供 BFF 直调的非 Saga 写 HTTP 路由。

```go
var created *domainresource.Resource
err := uc.txRunner.Run(ctx, func(executeCtx context.Context) error {
    // idempotency.Execute + 业务写
    return nil
})
```

- 纯查询不包裹事务。

### 3.4 DTM 子事务（Saga 分支）

#### 3.4.1 usecase：Execute + Compensate 同文件

参与 Saga 的写用例，在**同一文件、同一 struct** 上提供：

```go
func (uc *UpdateResourceStatusUseCase) Execute(ctx context.Context, input *UpdateResourceStatusInput) error
func (uc *UpdateResourceStatusUseCase) Compensate(ctx context.Context, input *UpdateResourceStatusInput) error
```

- 两者均通过 `idempotency.Execute(guard, txRunner, ctx, key, fn)` 包裹，内部配合 DTM barrier 保证幂等与防悬挂。
- `Compensate` 所需数据（如 `PreviousStatus`）由 **BFF 写入 Input**，领域不从其他服务查询。
- 只读 usecase 不提供 `Compensate`。

#### 3.4.2 handler：独立 Action / Compensate 入口

```go
// UpdateStatusAction — 仅调 Execute（示例）
func (h *ResourceHandler) UpdateStatusAction(c *gin.Context) { ... }

// UpdateStatusCompensate — 仅调 Compensate（示例）
func (h *ResourceHandler) UpdateStatusCompensate(c *gin.Context) { ... }
```

路由成对（路径以 `/internal/v1/` 为准，资源段单数；下为**示例**）：

```text
POST /internal/v1/{resource}/update-status          → dtm.BindAndHandleAction
POST /internal/v1/{resource}/update-status/compensate → dtm.BindAndHandleCompensate
```

**禁止**在同一 Handler 方法内根据 DTM `op` 或 `IsCallback` 分流 Action 与 BFF 直调。
**禁止**为同一写语义同时注册 `/{resource}/status`（直调）与 `/{resource}/update-status`（Saga）两条路由。

#### 3.4.3 BFF saga 编排

```go
// BFF 负责：前置校验、payload（bizId、bizSubId）、提交 Saga（示例）
saga, err := dtm.NewSaga(ctx, dtmServer)
if err != nil {
    return err
}
resourceClient.CreateResource(ctx, saga, payload)
saga.Submit()
```

- token 撤销等 BFF 本地副作用可作为独立 Saga 分支或 Saga 成功后同步调用 `gitee.com/hq_project/yomi-boot/component/token`。
- BFF **不实现**领域状态恢复的补偿逻辑。

### 3.5 事务上下文传递

- GORM 事务由 `TxExecutor.Execute` 内部 `db.Transaction` 管理。
- repository 通过 `db.Exec(ctx, r.DB)` 获取当前事务连接。
- 禁止将 `*gorm.DB` 作为参数在层间传递。
- 事务闭包内使用 `executeCtx`，不用外层 `ctx` 写库。

### 3.6 典型 Saga 场景（示例，非强制业务清单）

| 场景（示例） | BFF saga | 领域分支 |
| ------------ | -------- | -------- |
| 禁用资源 | SubmitSaga + revoke token | `{resource}` update-status Action/Compensate |
| 创建资源 | SubmitSaga | `{resource}` create Action/Compensate |
| 注册资源 | SubmitSaga | `{resource}` register Action/Compensate |
| 分配角色 | 前置校验 + SubmitSaga | 角色域 assign Action/Compensate |
| 角色删除 | SubmitSaga + revoke token | 角色域 delete Action/Compensate |

### 3.7 业务幂等与 DTM 分工

业务幂等（`bizId`/`bizSubId`、`gitee.com/hq_project/yomi-boot/component/idempotency`、`sys_biz`）与 DTM `gid`/barrier 的职责边界、执行顺序及禁止项，**一律以 [微服务调用规范](./微服务调用规范.md) §3.4、§5 为准**。

本文仅强调事务层要点：

- 写 usecase 通过 `idempotency.Execute(guard, txRunner, ctx, key, fn)` 在 DTM barrier **同一事务闭包内**执行业务写。
- DTM barrier 防悬挂/重复回调；**不替代** `(biz_id, biz_sub_id)` 业务幂等。

### 3.8 与 IO 的边界

- DTM 子事务闭包内**仅包含**本地 DB 步骤。
- Redis token 撤销、HTTP 通知等放在 BFF 或 `Execute` 返回之后，不放入领域 DB 事务闭包与 DB 强绑定。
- 若 IO 失败需回滚 DB，由 Saga 失败触发 Compensate，而非在单服务事务内混写 IO。

---

## 4. 检查清单

- [ ] 领域写 usecase 经 `idempotency.Execute` + `TxRunner` 控制本地子事务
- [ ] Saga 写用例同文件含 `Execute` 与 `Compensate`
- [ ] Handler Action/Compensate 为独立方法，路由 `/compensate` 成对，且均为 DTM 入口
- [ ] 领域无 BFF 可直调写路由
- [ ] Compensate 输入含 BFF 提供的快照字段
- [ ] repository 通过 Context 感知事务，不自行管理事务
- [ ] BFF saga 只编排，不实现领域补偿
- [ ] 业务幂等与 `gitee.com/hq_project/yomi-boot/component/idempotency` 符合 [微服务调用规范](./微服务调用规范.md) §3.4

---

## 5. 禁止项

- 禁止 repository 或 service 自行开启/提交/回滚事务。
- 禁止在 Handler 内混合 Action 与 Compensate 逻辑，或在同一写 URL 上按回调参数分流直调。
- 禁止领域 handler 暴露非 Saga 写接口（`BindAndHandle` 写 create/update/delete 等）。
- 禁止将 `Compensate` 写到独立 usecase 文件或 `compensate/` 目录。
- 禁止领域 usecase 在 Compensate 中 HTTP 调其他服务获取补偿数据。
- 禁止使用 2PC/TCC 替代 DTM Saga。
- 禁止在事务闭包内组装 HTTP 响应对象。
