# Go业务分层规范

---

## 0. 文档信息
- **文档名称**: Go业务分层规范
- **描述**: 定义入口编排（BFF）与领域两类服务类型的业务分层、职责边界、DDD-lite 建模及 depguard 依赖方向；具体业务服务为服务类型的参考实现。
- **作者**: zwei
- **创建日期**: 2026-06-17

---

## 1. 文档定位

- **用途**：约束入口编排（`handler/bff/saga`，经 client 契约模块调用下游）与领域（`handler/usecase/domain/repo`）各层职责、依赖方向与跨域协作规则。
- **边界**：可部署服务类型与目录模式见 [Go架构规范](./Go架构规范.md)；入口编排细则见 [GoBFF规范](./GoBFF规范.md)；事务见 [Go事务规范](./Go事务规范.md)；信封与写幂等见 [微服务调用规范](./微服务调用规范.md) §3；**文件命名**见 [Go文件命名规范](./Go文件命名规范.md)。
- **适用对象**：基于 gitee.com/hq_project/yomi-boot 的入口编排与领域可部署单元（含单体中对应模块）内业务代码的新增、重构与评审。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于**入口编排（BFF）**服务类型的可部署单元或其模块分层。
- 适用于**领域**服务类型的可部署单元或其模块分层。
- 默认轻量 DDD-lite + 贫血模型；充血模型按需采用，口径见 §3.1.2。
- 具体服务名与业务域随产品演进，以服务类型为准。
### 2.2 关联文档

| 文档 | 关联点 |
| ---- | ------ |
| [Go架构规范](./Go架构规范.md) | 目录与模块划分 |
| [GoBFF规范](./GoBFF规范.md) | BFF 跨域、喂饱参数、Saga |
| [Go事务规范](./Go事务规范.md) | usecase 本地子事务与 Compensate |
| [Go数据库规范](./Go数据库规范.md) | Entity/Model、repository |
| [Go代码规范](./Go代码规范.md) | 校验分层、依赖注入 |
| [微服务调用规范](./微服务调用规范.md) | 信封解析、写幂等（权威） |

---

## 3. 技术方案

### 3.1 建模原则

#### 3.1.1 轻量 DDD-lite

- 领域逻辑收口在 `domain/service`；持久化收口在 `repo/db`。
- usecase 编排应用服务，不承载领域规则。
- Entity 默认贫血；复杂状态机场景可按需充血，规则仍优先归 service。

#### 3.1.2 Entity 贫血与充血

- **默认贫血**：Entity 以字段为主，规则在 `domain/service`。
- **按需充血**：仅当实体自身状态迁移逻辑复杂且内聚时，在 Entity 上增加行为方法。
- 充血方法**不得**访问 repository、不得跨 Entity 编排、不得替代 service 对外入口。

### 3.2 两类服务分层对照

```text
【BFF】                          【领域】
handler/                         biz/handler/
                                  biz/usecase/        （平铺）
                                  biz/domain/
                                  biz/repo/
```

> BFF 不含 `token/` 和 `client/` 内部目录：令牌能力由 `gitee.com/hq_project/yomi-boot/component/common/token` 统一提供，下游调用经**领域 client**（业务工程独立模块，路径惯例 `{client}/{domain}`，非 gitee.com/hq_project/yomi-boot）。

| 能力 | BFF | 领域 |
| ---- | --- | ---- |
| 跨域 HTTP | ✓ 领域 client | ✗ |
| 读 token / 鉴权 | ✓ gitee.com/hq_project/yomi-boot/component/common/token | ✗ |
| 只读聚合 | ✓ bff | ✗ |
| DTM Saga 提交 | ✓ saga | ✗ |
| 本地 DB 事务 | ✗ | ✓ usecase |
| DTM 子事务 Execute/Compensate | ✗ | ✓ usecase |
| 领域规则 | ✗ | ✓ domain/service |

### 3.3 BFF 各层职责

详见 [GoBFF规范](./GoBFF规范.md)。摘要：

| 层 | 职责 |
| -- | ---- |
| `handler/` | 对外路由；薄适配；调下层 |
| `bff/` | 多服务只读聚合 |
| `saga/` | 跨域写 Saga 编排 |

> 令牌能力经 `gitee.com/hq_project/yomi-boot/component/common/token` 注入；下游调用经领域 client。

### 3.4 领域各层职责

#### 3.4.1 handler/

- 对内 REST 与 DTM 回调的协议适配。
- 绑定 Input/Output；调用 usecase。
- **Saga 写操作**：正向与补偿拆为**两个独立 Handler 方法**（如 `UpdateStatusAction`、`UpdateStatusCompensate`），禁止在同一 Handler 内 `switch op` 混合。
- **禁止**读取 token、`SessionFromContext`；**禁止**跨域 HTTP。

#### 3.4.2 usecase/

- 文件**按 usecase 平铺**于 `biz/usecase/`，包名 `usecase`。
- `api_types.go` 将 `{client}/{domain}/dto` 类型别名为本服务 Input/Output，避免重复定义 wire DTO。
- **硬约束**：一个 usecase 文件仅一个用例 struct、仅一个 `Execute`；参与 Saga 的写用例在**同文件**仅增加一个对应的 `Compensate`；**禁止** `*_usecases.go` 等多用例合文件。
- 通过 Input 获取各接口 DTO 声明的业务字段（如按需 `operatorId`、`targetUserId`），**不**从 Context 解析身份。
- 写用例通过 `idempotency.Execute(guard, txRunner, ctx, key, fn)` 包裹业务逻辑；细则见 [微服务调用规范](./微服务调用规范.md) §3.4。
- 写事务边界见 [Go事务规范](./Go事务规范.md) §3.3、§3.4。
- **禁止**跨域编排；**禁止**调用其他领域 HTTP；**禁止** usecase 互调。

#### 3.4.3 domain/contract/

- `{域}/` 子包：`{module}_contract.go`（Service 接口）、`{module}_entity.go`、`{module}_dto.go`。
- 纯领域类型，无 GORM tag，无 HTTP 依赖。

#### 3.4.4 domain/service/

- 实现 contract 中的 `{Module}Service`。
- 同域编排、领域规则、不变量校验。
- **禁止**依赖其他域 Service；**禁止** HTTP 出站。

#### 3.4.5 repo/

- 接口：`repo/contract/{域}/{module}_repo_contract.go`。
- 实现：`repo/db/{域}/` 含 `{module}_repo.go`、`{module}_model.go`、`{module}_mapper.go`。
- 只负责数据存取；通过 `db.Exec(ctx, r.DB)` 感知事务。
- **禁止**业务规则；**禁止**自行开启事务。

#### 3.4.6 internal/bizerr/

- 本服务领域专属业务异常哨兵（如 `bizerr.ErrUserNotFound`），使用 `errors.NewBiz` / `errors.NewNoStack` 定义；返回时经 `errors.Sentinel` 拍真实调用点栈。
- 跨库通用哨兵收口在 `gitee.com/hq_project/yomi-boot/core/shared/error`（`NewNoStack`）；库私有哨兵在对应库就近定义；**无** `response/code.go`。
- 跨服务通用 dto/utils 使用 `gitee.com/hq_project/yomi-boot/core/shared`；跨服务枚举使用 `{client}/{domain}/enums`。

### 3.5 跨域协作规则

```text
跨域只发生在 BFF：
  BFF handler/bff/saga → 领域 client → 领域 handler → usecase → domain → repo
```

| 场景 | 正确 | 错误 |
| ---- | ---- | ---- |
| Profile 含角色 | 入口编排 `bff/` 调主体域 + 角色域聚合 | 主体域 usecase 调角色域 |
| 登录 | 入口编排验密 + 本进程 token | 角色域签发 token |
| 分配角色 | 入口编排校验主体 + 角色域分配 | 角色域调主体域校验存在性 |
| 操作者 ID | 入口编排按接口 DTO 按需传 `operatorId` | 领域 `SessionFromContext` |

### 3.6 角色域（RBAC）边界

- 职责：角色 RBAC；按主体 id 查角色；角色 CRUD/分配。
- **不含**：登录、令牌、验密（验密在主体所在领域）。
- 分配角色用例只操作本库 role/role_rel；主体是否存在由入口编排前置校验。
### 3.7 请求调用链

**BFF 对外读接口：**

```text
Client → BFF handler → bff → 领域 client → 领域 handler → usecase → service → repo
```

**BFF 跨域写：**

```text
Client → BFF handler → saga → DTM → 领域 Action/Compensate handler → usecase → service → repo
```

**BFF 单域写（仍经 Saga，禁止领域 client 直调）：**

```text
Client → BFF handler → saga → DTM → 单分支 Action/Compensate handler → usecase → service → repo
```

### 3.8 depguard 依赖方向

**领域 `internal/biz`：**

```text
handler → usecase → domain/contract
usecase → domain/contract, gitee.com/hq_project/yomi-boot/component/common/db, gitee.com/hq_project/yomi-boot/component/cloud/dtm（barrier）, gitee.com/hq_project/yomi-boot/component/common/idempotency（写幂等）
service → domain/contract, repo/contract
repo/db → domain/contract, gitee.com/hq_project/yomi-boot/component/common/db
```

禁止：

- handler → repo（跳过 usecase）
- usecase → 其他 usecase
- service → 其他域 service
- 任何层 → 其他服务领域 client 或 HTTP 包

**BFF `internal`：**

```text
handler → bff, saga
saga → 领域 client（业务工程独立模块引用）
```

禁止 BFF import 领域 `internal/biz`。

---

## 4. 检查清单

- [ ] 领域 handler/usecase 不读 token、不跨域 HTTP
- [ ] BFF 调领域时按接口 DTO 显式传参（如按需传 `operatorId`）
- [ ] 领域 usecase 平铺；Saga 写用例同文件含 `Compensate`
- [ ] 领域 Saga Handler：Action 与 Compensate 独立入口
- [ ] BFF 写操作（含单域）均经 `saga/` 提交 DTM，BFF 不可直调领域写接口
- [ ] Entity/Model 分离；repository 不承载业务规则
- [ ] depguard 依赖方向符合 §3.8

---

## 5. 禁止项

- 禁止领域服务之间 HTTP 互调。
- 禁止领域 usecase 跨域编排（跨域编排与令牌相关逻辑仅在 BFF）。
- 禁止领域 handler 从 token Context 取 userId。
- 禁止在领域 usecase 使用 `token.SessionFromContext`。
- 禁止 Handler 内混合 DTM Action 与 Compensate 路由逻辑。
- 禁止将补偿 usecase 拆到独立目录（须与 `Execute` 同文件）。
- 禁止 BFF 直调领域写接口（单域写亦须经 `saga/` + DTM）。
