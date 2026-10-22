# Go异常规范

---

## 0. 文档信息
- **文档名称**: Go异常规范
- **描述**: 定义基于 gitee.com/hq_project/yomi-boot 的业务工程中异常的创建、传播、处理与 Encoder 写出规范。
- **作者**: zwei
- **创建日期**: 2026-06-27

---

## 1. 文档定位

- **用途**：约束 Go 后端异常的定义方式、创建函数选择、分层传播规则与 Handler 层统一处理口径，确保异常可追溯、可响应、可排查。
- **边界**：不覆盖分布式事务的幂等错误处理（见 [Go事务规范](./Go事务规范.md) §3.4）；不覆盖微服务间调用错误的信封解析（见 [微服务调用规范](./微服务调用规范.md) §3）；HTTP 出口编码适配见 [API编码适配规范](./API编码适配规范.md)。
- **适用对象**：`gitee.com/hq_project/yomi-boot/` 本库，以及基于 gitee.com/hq_project/yomi-boot 的业务工程中涉及 error 创建、传播、处理与响应的 Go 代码。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于 `gitee.com/hq_project/yomi-boot/component/core/errors` 系统级错误包（`errors.New`、`errors.Wrap`、`errors.Stack` 等）。
- 适用于 `gitee.com/hq_project/yomi-boot/component/core/errors.BizError`（可选，用于域错误携带 Code/Message）。
- 适用于 `gitee.com/hq_project/yomi-boot/component/core/shared/error` 跨库通用哨兵，以及各库/业务就近定义的哨兵。
- 适用于 `gitee.com/hq_project/yomi-boot/component/common/handler` 中 `WriteBusinessError` 等异常响应逻辑（委托 `response.Encoder`）。
- 不适用于 panic 恢复（见 `gitee.com/hq_project/yomi-boot/boot/bootstrap` 中间件链 Recovery）。

### 2.2 关联文档

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| `_prompts/knowledge/Go代码规范.md` | 编码规范 | 校验分层、接口实现编译期断言 |
| `_prompts/knowledge/Go业务分层规范.md` | 分层职责 | handler/usecase/service/repo 各层职责 |
| `_prompts/knowledge/Go事务规范.md` | 事务异常 | TxRunner/barrier 内异常处理、幂等 Guard 错误 |
| `_prompts/knowledge/微服务调用规范.md` | 微服务信封 | 下游错误传递与 BizError 构造 |
| `_prompts/knowledge/API编码适配规范.md` | Encoder 写出 | 失败经 `EncodeFailure`；JSON 形状由已注册 Encoder 决定 |

---

## 3. 技术方案

### 3.1 异常类型体系

| 层级 | 包路径 | 用途 | 关键特征 |
| ---- | ------ | ---- | -------- |
| 系统级错误 | `gitee.com/hq_project/yomi-boot/component/core/errors` | 非业务错误（基础设施、IO、序列化等），带调用栈 | 基于 `github.com/pkg/errors`；`New`/`Newf`/`Wrap`/`Sentinel` 拍栈 |
| 简单哨兵 | `shared/error` 或各库就近 | 框架/通用失败；`Error()` **即为线上 code** | 用 `errors.NewNoStack` 定义（无栈身份）；返回时 `errors.Sentinel` |
| 业务异常（可选） | `gitee.com/hq_project/yomi-boot/component/core/errors.BizError` | 域规则违背，需同时带 Code 与调试 Message | `NewBiz` / `NewBizf` / `WrapBiz`；按 Code 做 `errors.Is` |

#### 3.1.1 系统级错误 `gitee.com/hq_project/yomi-boot/component/core/errors`

```go
import "gitee.com/hq_project/yomi-boot/component/core/errors"

func New(text string) error                          // 创建并拍栈（一次性错误）
func Newf(format string, args ...any) error          // 格式化创建并拍栈
func NewNoStack(text string) error                   // 无栈身份（仅包级哨兵 var）
func Sentinel(err error) error                       // 调用点为哨兵拍栈；Error()/Is 不变
func Wrap(err error, format string, args ...any) error
func Is(err, target error) bool
func As(err error, target any) bool
func Stack(err error) string
```

- 外来错误（如 GORM）必须经 `errors.Wrap` 附加栈后传播。
- 包级哨兵**禁止**用 `errors.New` 定义（会钉死 init 栈）；须 `NewNoStack` + 返回时 `Sentinel`。

#### 3.1.2 简单哨兵（推荐框架与通用错误）

```go
// gitee.com/hq_project/yomi-boot/component/core/shared/error — 仅跨库通用（无栈身份）
var ErrInternal = errors.NewNoStack("INTERNAL_ERROR") // 系统内部错误

// 库私有：在使用处就近定义，禁止一个 errors.go 堆砌无关哨兵
// token/middleware.go
var ErrTokenRequired = errors.NewNoStack("TOKEN_REQUIRED") // 缺少访问令牌

// 首次返回时拍真实调用点栈
return errors.Sentinel(ErrTokenRequired)
```

- **通用**（多库共用）→ `gitee.com/hq_project/yomi-boot/component/core/shared/error`。
- **非通用** → 哪个库用就在哪个库定义；不要集中到无关包。
- **权限类**（如 `PERMISSION_DENIED`）由**业务**定义，gitee.com/hq_project/yomi-boot 不提供。
- **禁止**裸 `return ErrXxx`；中间层对已 `Sentinel`/`Wrap` 的 err 只透传，禁止再包。

#### 3.1.3 业务异常 `gitee.com/hq_project/yomi-boot/component/core/errors.BizError`（域错误可选）

```go
type BizError struct {
    Code    string
    Message string
    Cause   error
}

func NewBiz(code, msg string) *BizError
func NewBizf(code, format string, args ...any) *BizError
func WrapBiz(cause error, code, msg string) *BizError
```

服务级哨兵示例（业务仓）：

```go
// internal/biz/bizerr/errors.go
var (
    ErrUserNotFound = errors.NewBiz("USER_NOT_FOUND", "用户不存在")
    // 或 errors.NewNoStack("USER_NOT_FOUND")，返回时 errors.Sentinel(...)
)
```

- 包级 `*BizError` 本身不带栈；若日志需要真实调用点，返回 `errors.Sentinel(bizerr.ErrXxx)`。
- **已删除** `response.BizCode` / `FromBizCode` / `response/code.go`；禁止再依赖。

### 3.2 异常创建与传播

#### 3.2.1 创建选择矩阵

| 场景 | 使用函数 | 说明 |
| ---- | -------- | ---- |
| 定义包级哨兵 | `errors.NewNoStack(code)` | 无栈身份；code 为线上字符串 |
| 返回预定义哨兵 | `errors.Sentinel(ErrXxx)` | 仅在首次抛出点拍栈；干净单段栈 |
| 一次性独立错误 | `errors.New` / `Newf` | 创建即拍栈 |
| 包装外来 error | `errors.Wrap(err, format, args...)` | 附加调用栈 |
| 域错误需独立 msg | `errors.NewBiz(code, msg)` | Encoder 可分别取 Code/Message（实现自定） |
| 获取完整栈信息 | `errors.Stack(err)` | 仅用于日志，**禁止**返回给前端 |

#### 3.2.2 传播分层规则

| 层 | 错误类型 | 传播方式 | 说明 |
| -- | -------- | -------- | ---- |
| **repository** | `errors.Wrap` / `Sentinel(哨兵)` | 向上透传 | "未找到"等语义 `return errors.Sentinel(bizerr.ErrXxx)` |
| **domain/service** | 哨兵 / BizError / Wrap | 领域违背 `Sentinel`；其余 Wrap | 不做跨域错误转换；已有栈只透传 |
| **usecase** | 同上 | Validate 用 `Sentinel`；其余透传 | 不消费错误（交给 handler） |
| **handler** | 不解析业务码 | 统一 `WriteBusinessError` | 归一超时/超限时 `Sentinel` 新哨兵后交给 Encoder |

#### 3.2.3 日志与传播分离原则

1. **返回 error 默认不打印日志**：中间层包装后向上抛，**不在本层打印日志**。
2. **处理 error 必须打印日志，且不继续向上抛出**：Handler 经 `WriteBusinessError` 写响应时自动记录日志。

### 3.3 异常处理与 Encoder 写出

#### 3.3.1 Handler 统一处理入口

```go
func WriteBusinessError(c *gin.Context, err error)
func WriteBusinessErrorWithFallback(c *gin.Context, err, fallback error)
func WriteFailure(c *gin.Context, err error) // 同 WriteBusinessError
```

映射逻辑（写出前）：

```text
normalizeFailure(err)
  ├─ nil → sharederr.ErrInternal
  ├─ DeadlineExceeded → ErrRequestTimeout
  ├─ MaxBytesError → ErrRequestBodyTooLarge
  ├─ *BizError → 原样
  ├─ 有 fallback 且非 BizError → 使用 fallback 哨兵
  └─ 其余 → 原样 err

→ Enc().EncodeFailure(c, out)
```

- 写出细节（字段、HTTP 状态、如何从 `BizError`/哨兵取码）由**已注册 Encoder**决定；预制实现仅供显式选用，见 [API编码适配规范](./API编码适配规范.md)。
- Handler **禁止**自行 `c.JSON` 写业务失败响应（DTM 特例除外）。

#### 3.3.2 与响应 JSON 的关系

本规范**不**规定 HTTP JSON 字段；成败如何呈现由本服务 Encoder 契约决定。

#### 3.3.3 兜底链路

```text
BindAndHandle / Handle / ...
  └─ WriteBusinessError(c, err)
       └─ normalize → log → Enc().EncodeFailure
            └─ 未知 err 如何映射由 Encoder 决定；
               若需固定 INTERNAL_ERROR，应返回 sharederr.ErrInternal
```

### 3.4 错误码定义规范

#### 3.4.1 命名规则

- 错误码使用 `SCREAMING_SNAKE_CASE`。
- 哨兵变量名：`Err{语义}`，如 `ErrUserNotFound`、`ErrTokenRequired`。
- `Error()` 字符串 = 线上 code；描述写在注释。

| 层级 | 示例 | 定义位置 |
| ---- | ---- | -------- |
| 系统通用 | `INTERNAL_ERROR`、`INVALID_BODY`、`INVALID_INPUT`、`REQUEST_TIMEOUT`、`REQUEST_BODY_TOO_LARGE` | `gitee.com/hq_project/yomi-boot/component/core/shared/error` |
| 令牌域（boot） | `TOKEN_INVALID`、`TOKEN_REQUIRED`、`TOKEN_VALIDATE_FAILED` | `gitee.com/hq_project/yomi-boot/component/common/token` 就近 |
| 限流 / 下游 | `RATE_LIMITED`、`DOWNSTREAM_ERROR` | 对应库就近 |
| 业务域 | `USER_NOT_FOUND` 等 | 各服务 `bizerr` |
| 权限域 | `PERMISSION_DENIED` 等 | **仅业务仓**，gitee.com/hq_project/yomi-boot 不定义 |

#### 3.4.2 定义位置

| 内容 | 位置 |
| ---- | ---- |
| 跨库通用哨兵 | `gitee.com/hq_project/yomi-boot/component/core/shared/error/error.go` |
| 库私有哨兵 | 使用该错误的源文件内就近 `var` |
| 服务域哨兵 | `services/{unit}/internal/biz/bizerr/` 或入口编排 `internal/bizerr/` |
| `BizError` 类型 | `gitee.com/hq_project/yomi-boot/component/core/errors/biz_error.go` |

### 3.5 启动阶段异常（Fail Fast）

三阶段策略权威如下（架构装配侧应用见 [Go架构规范](./Go架构规范.md) §3.4.1）：

| 阶段 | 策略 | 典型场景 |
| ---- | ---- | -------- |
| 启动 | log + **panic** | 配置、插件 Init、`MustRegister` 缺失（`MustReady`）、`MustBuildApp`；插件 Init 失败**不**回滚已启动插件 |
| 运行 | `error` 传播 + Handler → Encoder | usecase/service/repository、HTTP 请求；Recovery 捕获 handler panic |
| 关闭 | 尽力释放，失败静默跳过 | 插件 Close、Cleanup |

---

## 4. 检查清单

- [ ] 外来错误已用 `errors.Wrap` 附加调用栈
- [ ] 包级哨兵用 `errors.NewNoStack` 定义，首次返回经 `errors.Sentinel`（禁止裸 `return ErrXxx`）
- [ ] 框架/通用失败使用 `shared/error` 或库内哨兵，`Error()` 为 code
- [ ] 域错误在业务 `bizerr`，未要求 gitee.com/hq_project/yomi-boot 增加权限类哨兵
- [ ] Handler 统一经 `WriteBusinessError` → Encoder，未绕过 Encoder 手写业务响应（HTTP/JSON 形状由 Encoder 决定，见 [API编码适配规范](./API编码适配规范.md)）
- [ ] `main` 已 `response.MustRegister`
- [ ] `errors.Stack` 仅用于日志
- [ ] 中间层包装上抛时未打印日志；已拍栈 err 未再次 `Sentinel`

---

## 5. 禁止项

- 禁止依赖已删除的 `response/code.go`、`FromBizCode`、`WriteError(code, msg)`。
- 禁止在 gitee.com/hq_project/yomi-boot 定义业务权限错误（如 `PERMISSION_DENIED`）。
- 禁止在无关包用一个 `errors.go` 堆砌多域哨兵；库私有错误就近定义。
- 禁止用 `errors.New` 定义包级哨兵（钉死 init 栈）；须 `NewNoStack`。
- 禁止裸 `return ErrXxx` / 对已 `Sentinel`/`Wrap` 的 err 再 `Sentinel`。
- 禁止在 repository / service / usecase 内自行 `c.JSON` 写业务响应。
- 禁止将 `errors.Stack(err)` 输出到 HTTP body。
- 禁止在 handler/usecase 绕过 Encoder 用手写 `c.JSON` / 状态码表达业务成败；HTTP/JSON 映射由已注册 Encoder 决定（见 [API编码适配规范](./API编码适配规范.md)）。DTM 写回调走独立契约（见微服务调用规范）。
- 禁止中间层在 Wrap 上抛的同时打印日志。
- 禁止包 `init` 静默注册 Encoder。
