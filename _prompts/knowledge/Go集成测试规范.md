# Go集成测试规范

---

## 0. 文档信息
- **文档名称**: Go集成测试规范
- **描述**: 定义 BFF 与领域微服务 HTTP 集成测试的目录组织、运行方式与用例编写口径。
- **作者**: zwei
- **创建日期**: 2026-06-17

---

## 1. 文档定位

- **用途**：约束各微服务 `test/` 下 HTTP 集成测试的目录落位、用例编写与结果处理口径。
- **测试目标**：验证接口**能正确响应**——服务可达、路由可达、请求可发出、响应可读取。
- **不做的事**：不验证业务是否正确（参数校验、错误码、字段值等由与被测源码同包的单元测试负责）。
- **边界**：不覆盖单元测试、性能压测、Consul/DTM 环境搭建。
- **适用对象**：业务工程可部署单元下的 HTTP 接口集成测试（惯例目录示例：`services/{unit}/test/`）。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 对已运行 HTTP 服务发起真实请求的黑盒测试。
- **BFF 与领域服务**：测试用例与 `cfg_test.go` **平铺**在 `test/` 目录，不再按 handler 模块分子目录。
- 每个测试函数**只调用一个 HTTP 接口**。
- 不直接调用 handler、usecase 或 repository。
- HTTP 请求统一使用 `gitee.com/hq_project/yomi-boot/testclient` 封装。
- 请求体**复用接口定义的请求对象**，禁止使用 `map[string]any` 等松散结构。

### 2.2 关联文档

| 文档 | 关联点 |
| ---- | ------ |
| [Go架构规范](./Go架构规范.md) | 多服务与路由前缀 |
| [GoBFF规范](./GoBFF规范.md) | BFF 对外路径 |
| [API编码适配规范](./API编码适配规范.md) | Encoder 适配 |

---

## 3. 技术方案

### 3.1 目录结构

**入口编排（BFF）：**

```text
services/{bff}/test/
├── cfg_test.go             # BaseURL、Token、登录账号等环境配置
├── login_test.go           # 一接口一文件
├── health_test.go
└── ...
```

**领域：**

```text
services/{domain}/test/
├── cfg_test.go             # ServiceBaseURL 等环境配置
├── health_test.go
└── ...
```

规则：

- 所有集成测试文件与 `cfg_test.go` **直接平铺**在该可部署单元的 `test/` 目录。
- **一个 HTTP 接口对应一个 `_test.go` 文件**，文件名体现接口语义（如 `login_test.go`）。
### 3.2 请求对象

- 集成测试的请求参数须**直接复用接口使用的请求 struct**，保证字段名、类型与 handler 绑定一致。
- **BFF**：复用 `internal/bff`、`internal/saga` 等层的 Input / Request 类型（如 `bff.AdminLoginInput`）。
- **领域服务**：复用 `internal/biz/usecase` 的 Input 类型，或**领域 client**（`{client}/{domain}`）中定义的 Request 类型。
- 可在 `cfg_test.go` 中封装返回请求对象的辅助函数（如 `LoginInput()`），避免在用例内重复构造。
- **禁止**使用 `map[string]any`、`map[string]string` 等 map 作为请求体。

示例：

```go
body, ok := testclient.Post(LoginURL(), "", LoginInput())

body, ok := testclient.Post(LoginURL(), "", bff.AdminLoginInput{
    Username: LoginUsername,
    Password: LoginPassword,
})
```

### 3.3 运行方式

- 集成测试前须启动目标服务（及依赖的下游服务，按场景）。
- 登录类用例使用的账号密码须与测试环境已有数据一致（仓库内无统一 seed/init-db 脚本）。
- BFF 集成测试默认打 BFF 对外完整路径（`router_prefix` + 相对路径；**示例**：`http://127.0.0.1:18080{router_prefix}/v1/...`）。
- 领域集成测试打领域内路径（**示例**：`/internal/v1/{resource}/profile` 等**读**接口）。
- 领域**写**接口为 Action/Compensate 成对，由 DTM 回调；领域集成测试若覆盖写路径，须模拟 DTM 回调（POST + JSON 信封），或改测 BFF 对外写接口。
- 通过环境变量或 `test/cfg_test.go` 配置 baseURL。

### 3.4 单一接口原则

- 每个 `TestXxx` 只发**一次** HTTP 请求，验证一个路由。
- 不测调用链；不测 BFF 到领域的全链路（可另建 E2E 场景，不在本规范默认范围）。

### 3.5 响应处理

- 使用 `gitee.com/hq_project/yomi-boot/testclient` 发起请求，读取 `(body, ok)` 返回值。
- `ok` 为 true 表示 HTTP 状态码 200；失败时 `body` 含响应体或错误信息。
- 需要解包业务载荷时，直接用 `response.Enc().Parse` / `IsSuccess`（与 httprecord 同口径）；测试进程须已 `MustRegister`（或 `RegisterDefaultForTest`）。
- 默认**不断言**业务 `code` 字符串（如 `"OK"` / `"USER_NOT_FOUND"`）（由与被测源码同包的单元测试覆盖）。
- 失败时打印 URL、请求体、响应体，便于排查。

### 3.6 微服务调用

- 领域集成测试打 `/internal/v1/...`，**POST + JSON**；链路 ID 经 Header `X-Trace-ID` 传播（testclient 自动处理）。
- 集成环境须已初始化目标领域库：执行 `services/{unit}/script/sql/` 下建库与业务表 DDL，并按 [微服务调用规范](./微服务调用规范.md) §3.4.2 在该库执行平台表 DDL（`sys_biz`、`sys_barrier`）。

---

## 4. 检查清单

- [ ] 测试位于对应服务的 `test/` 目录（平铺，无子目录）
- [ ] 一接口一文件
- [ ] 不直接调用 handler/usecase/repository
- [ ] 请求体复用接口定义的 struct，未使用 map
- [ ] BFF 测试路径与 `router_prefix` 一致
- [ ] 领域测试不依赖 token（身份通过 Body 传参）
- [ ] HTTP 请求使用 `gitee.com/hq_project/yomi-boot/testclient`

---

## 5. 禁止项

- 禁止在业务工程根目录随意堆放跨服务集成测试；测试须落在各可部署单元自身 `test/` 下。
- 禁止在 `test/` 下再建 `integration/`、`testutil/` 等子目录。
- 禁止使用 `map[string]any` 等 map 构造请求体。
- 禁止集成测试直接调用 usecase.Execute。
- 禁止一个测试函数连续测多个 HTTP 接口。
