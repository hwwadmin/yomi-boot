# Go代码规范

---

## 0. 文档信息
- **文档名称**: Go代码规范
- **描述**: 定义 Go 后端校验与编排分层、依赖注入与运行时判空约定、接口实现编译期校验、Redis 经 `RedisService` 访问约束、业务接口协议选型、前后端交互时间类型、雪花 ID 字段类型规范，以及枚举定义与使用规范（含 IsValid/UnmarshalJSON 默认实现要求）。
- **作者**: zwei
- **创建日期**: 2026-06-02

---

## 1. 文档定位

- **用途**：作为 **gitee.com/hq_project/yomi-boot 本库及基于 gitee.com/hq_project/yomi-boot 的业务工程** Go 后端代码编写的补充规范，约束校验与编排分层、注入判空、接口实现编译期校验、Redis 访问收口和业务接口协议选型等常见问题。
- **边界**：不覆盖目录结构、路由注册、事务、事件、数据库、缓存、日志等基础设施设计；相关内容应分别参考 [Go架构规范](./Go架构规范.md)、[Go业务分层规范](./Go业务分层规范.md)、[API编码适配规范](./API编码适配规范.md) 及对应的事务、数据库、Redis 等规范。
- **适用对象**：gitee.com/hq_project/yomi-boot 本库相关封装，以及业务工程中 model、usecase、service、repository、handler 的编写与评审。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于基于 gitee.com/hq_project/yomi-boot 的业务工程中各服务 `internal/biz/`（及同类分层目录）下的领域代码，以及入口编排的 `internal/handler/` 协议适配代码。
- 适用于新写业务模块时的代码风格约束，以及重构既有模块时的逐层调整指引。

### 2.2 关联文档

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| `_prompts/knowledge/Go业务分层规范.md` | 业务分层 | 定义 handler / usecase / contract / service / repository 分层与轻量 DDD-lite 建模 |
| `_prompts/knowledge/Go架构规范.md` | 项目架构 | 定义 gitee.com/hq_project/yomi-boot、业务可部署单元与 bootstrap 装配 |
| `_prompts/knowledge/GoBFF规范.md` | BFF | 跨域编排、喂饱参数、令牌实现 |
| `_prompts/knowledge/Go注释规范.md` | 注释规范 | 约束 Go 代码注释口径 |
| `_prompts/knowledge/Go选型规范.md` | 技术栈基线 | 约束默认技术选型 |
| `_prompts/knowledge/API编码适配规范.md` | 接口契约 | 约束 handler 经 Encoder 写出；不规定具体 JSON 形状 |

### 2.3 阅读建议

- 编写新业务模块时，优先阅读「校验与编排分层」，先定各层边界再写代码。
- 评审既有代码时，重点阅读“各层不自判注入”、“Redis 访问收口”和“业务接口默认 POST/JSON”，确认新老模块是否混入不一致写法。

---

## 3. 技术方案

### 3.1 校验与编排分层

不同性质的校验与编排落在不同的层，不得交叉。完整分层口径见 [Go业务分层规范](./Go业务分层规范.md) §3.2–§3.4。

#### 3.1.1 校验分层

| 层 | 校验职责 | 典型场景 |
|----|----------|----------|
| **usecase** | 入参基础校验 | `Validate()` 内检查 input 非 nil、ID > 0、必填字段非空、Trim、分页参数、`JSONInt64` 合法、枚举格式合法 |
| **service** | 领域规则与不变量校验 | 唯一性判断、状态流转合法性、超级管理员保护、关联实体须 ENABLED、密码加密与归一化 |
| **repository** | 无业务校验 | 只执行 SQL 读写和 Entity ↔ Model 映射，不检查模型字段是否有效 |

**禁止重复校验**：同一规则不得在 usecase 与 service 两层同时实现。例如「roleName 非空」归 usecase，「roleName 全局唯一」归 service。

#### 3.1.2 编排分层

| 层 | 编排职责 | 典型场景 |
|----|----------|----------|
| **usecase** | 应用编排 | 入参 Validate、调用 domain service、写用例包裹 idempotency.Execute + TxRunner |
| **service** | 同域编排 | 创建前唯一性校验再落库；删除前清关联再删主表；Assign 前校验角色状态再 ReplaceRels |
| **repository** | 无编排 | 单步 SQL/GORM 读写 |

单域写操作的 usecase 可以较薄：Validate → tx → 调用一次 `{Module}Service` 命令 → 转 Output。

```go
// usecase：负责入参基础校验
func (i *CreateUserInput) Validate() error {
    if strings.TrimSpace(i.Username) == "" {
        return bizerr.ErrUserNameRequired
    }
    if strings.TrimSpace(i.Password) == "" {
        return bizerr.ErrUserPasswordRequired
    }
    return nil
}

// service：负责同域编排与领域不变量，依赖 contract 接口与 DTO
func (s *Service) Create(ctx context.Context, input *domainuser.CreateInput) (*domainuser.User, error) {
    if input.Nickname == "" {
        input.Nickname = input.Username
    }
    hashed, err := passwd.Hash(input.Password) // gitee.com/hq_project/yomi-boot/component/core/bc/passwd
    if err != nil {
        return nil, bizerr.ErrUserPasswordEncryptFailed
    }
    return s.repo.Create(ctx, &domainuser.CreatePersistInput{
        Username: input.Username, PasswordHash: hashed, ...
    })
}

// repository：只做 SQL 读写和 Entity 映射，不做业务校验
func (r *Repository) Create(ctx context.Context, input *domainuser.CreatePersistInput) (*domainuser.User, error) {
    model := toUserModel(input)
    if err := r.BaseRepository.Create(ctx, model); err != nil {
        return nil, err
    }
    return toUserEntity(model), nil
}
```

### 3.2 各层方法不自判注入

usecase、service、handler、transaction、router 各层方法内**不对注入的依赖做 nil 判空**。依赖装配的正确性由 `boot` 启动阶段保证，不在运行期每层兜底。

```go
// 正确：不检查 s == nil 或 s.repo == nil
func (s *Service) GetByID(ctx context.Context, id int64) (*domainuser.User, error) {
    return s.repo.FindByID(ctx, id)
}

// 错误：每层方法内自行判空是冗余的
// func (s *Service) GetByID(ctx context.Context, id int64) (*UserModel, error) {
//     if s == nil || s.repo == nil {
//         return nil, bizerr.ErrUserRepositoryUnavailable
//     }
//     return s.repo.FindByID(ctx, id)
// }
```

handler 的 `Routes` 方法也遵循同一原则：router 由 router 包在启动时注入，方法内不再对 router 做 nil 判空。

> **路由路径口径**：handler 的 `Routes` 内注册**相对路径**（**示例**：`/v1/{resource}/profile`），完整 URL = `router_prefix` + 相对路径。资源段使用**单数名词**。
>
> **领域写接口**：仅注册 Action/Compensate 成对路由，使用 `dtm.BindAndHandleAction` / `dtm.BindAndHandleCompensate`；**禁止** `handler.BindAndHandle` 暴露可被 BFF 直调的写接口。详见 [微服务调用规范](./微服务调用规范.md) §3.1.1。

```go
// 正确：领域读接口（示例）
func (h *ResourceHandler) Routes(router *gin.RouterGroup) {
    router.POST("/internal/v1/{resource}/profile", h.handleGetProfile) // 经 url 常量注册
}

// 正确：领域写接口（仅 DTM；示例）
func (h *ResourceHandler) Routes(router *gin.RouterGroup) {
    router.POST(resourceapi.PathUpdateStatusAction, h.handleUpdateStatusAction)
    router.POST(resourceapi.PathUpdateStatusCompensate, h.handleUpdateStatusCompensate)
}

func (h *ResourceHandler) handleUpdateStatusAction(c *gin.Context) {
    dtm.BindAndHandleAction(c, h.UpdateResourceStatus.Execute)
}
```

**前提**：各服务 `internal/bootstrap/server.go` 必须保证依赖在启动阶段完成注入；`MustInit` / 插件 `Init` 失败时 Fail Fast（log + panic），未初始化的依赖不应拖到运行期才暴露。

#### 3.2.1 Redis 访问收口

- **BFF**：Redis 用于令牌 Store（session 持久化），经 `gitee.com/hq_project/yomi-boot/component/common/token` + `gitee.com/hq_project/yomi-boot/component/common/redis.RedisService` 访问。
- **领域服务**：`MustInit` 启用 Redis 供 `gitee.com/hq_project/yomi-boot/component/common/idempotency` 三层幂等使用；**禁止** usecase/service/handler 直接访问 Redis。
- 禁止业务层直接使用 `*redis.Client`；读写经 `gitee.com/hq_project/yomi-boot/component/common/redis.RedisService`（仅 gitee.com/hq_project/yomi-boot 内部组件允许）。

| 场景 | 允许方式 | 禁止方式 |
| ---- | -------- | -------- |
| BFF token Store | `gitee.com/hq_project/yomi-boot/component/common/token` + `gitee.com/hq_project/yomi-boot/component/common/redis.RedisService` | 领域业务层访问 Redis |
| 领域 idempotency | `gitee.com/hq_project/yomi-boot/component/common/idempotency.Guard`（bootstrap 注入） | usecase 直接调 Redis |
| gitee.com/hq_project/yomi-boot/component/common/redis 实现 | `RedisHelper` 内部持有 `*redis.Client` | 业务层 import go-redis |

Key 命名、TTL 见 [Redis设计规范](./Redis设计规范.md)。

### 3.3 业务接口 HTTP 方法

| 场景 | 方法 | 规范 |
| ---- | ---- | ---- |
| **入口编排对外**（`router_prefix`，按端在配置中设置；**示例**：如管理端/App） | GET 或 POST | CDN/SEO 可用 GET；见 [Go架构规范](./Go架构规范.md) §3.6 |
| **微服务间**（BFF→领域、DTM→领域） | **POST only** | 见 [微服务调用规范](./微服务调用规范.md) §3（权威） |

业务对外接口主输入约定见 [Go架构规范](./Go架构规范.md) §3.6；出口编码见 [API编码适配规范](./API编码适配规范.md)。

```go
// 正确：业务接口使用 POST + JSON body（Routes 内为相对路径；示例资源）
router.POST("/v1/{resource}/profile", h.handleGetProfile)
router.POST("/v1/{resource}/query", h.handleQueryResources)

// 健康检查仍可使用 GET
// 健康检查在 bootstrap 中注册：platformhandler.NewHealthHandler(consulPath, opsPath, infra.AppCtx)
```

### 3.3.1 handler 协议适配辅助

业务 handler 的 JSON 绑定、用例执行、分页响应与错误映射，统一通过 `gitee.com/hq_project/yomi-boot/component/core/handler` 完成。

##### 3.3.1.1 领域 handler

| 场景 | 用法 |
| ---- | ---- |
| 绑定 + 执行 | `handler.BindAndHandle(c, uc.Execute)` |
| 绑定 + 分页 | `handler.BindAndHandlePage(c, uc.Execute)` |
| DTM Action | `dtm.BindAndHandleAction(c, uc.Execute)` |
| DTM Compensate | `dtm.BindAndHandleCompensate(c, uc.Compensate)` |

- 领域 handler **禁止**从 token Context 取 userId；身份字段由 BFF 按接口 DTO 传入。
- import 时可用别名：`platformhandler "gitee.com/hq_project/yomi-boot/component/core/handler"` 或 `handler "gitee.com/hq_project/yomi-boot/component/core/handler"`。

##### 3.3.1.2 BFF handler

| 场景 | 用法 |
| ---- | ---- |
| 对外 API | `platformhandler.BindAndHandle(c, bffFn)` |
| 登录等需 Request | `platformhandler.BindAndHandleWithRequest(c, fn)` |

- BFF 经 `gitee.com/hq_project/yomi-boot/component/common/token` 解析 Claims 后，按下游接口 DTO **按需**写入业务字段。
- 登出等无 Body 场景：`platformhandler.HandleVoid(c, revokeFn)`。

分页 usecase 入参嵌入 `dto.PageInput`，出参直接返回 `*dto.PageOutput[TItem]`；`Validate()` 中调用 `i.PageInput.ValiPage()` 校验分页参数。分页辅助方法见 [Go数据库规范](./Go数据库规范.md) §3.2.2。

用例执行失败时，由已注册的 `response.Encoder` 写出失败响应；未知错误如何映射由该 Encoder 决定（boot 未指定 `Options.ResponseEncoder` 时默认注册预制实现；低层启动路径须显式 `MustRegister`）。

```go
// 正确：通过 gitee.com/hq_project/yomi-boot/component/core/handler 收口重复逻辑（示例）
func (h *ResourceHandler) handleGetProfile(c *gin.Context) {
    platformhandler.BindAndHandle(c, h.GetProfile.Execute)
}

// 错误：重复手写绑定 + 执行 + 响应样板代码
// func (h *ResourceHandler) handleGetProfile(c *gin.Context) {
//     var input resourceusecase.GetProfileInput
//     if err := c.ShouldBindJSON(&input); err != nil {
//         platformhandler.WriteBusinessError(c, bizerr.ErrInvalidBody)
//         return
//     }
//     output, err := h.GetProfile.Execute(c.Request.Context(), &input)
//     if err != nil {
//         platformhandler.WriteBusinessError(c, err)
//         return
//     }
//     platformhandler.WriteSuccess(c, output)
// }
```

### 3.3.2 分页出入参（dto.PageInput / dto.PageOutput）

分页 usecase 统一使用 `gitee.com/hq_project/yomi-boot/component/core/shared/dto/page.go` 中的结构体，避免在各模块重复定义 `page` / `pageSize` / `total` 字段。

| 类型 | 用法 | 说明 |
| ---- | ---- | ---- |
| `dto.PageInput` | **嵌入** usecase 入参 | 含 `page`、`pageSize` JSON 字段；`ValiPage()` 校验并填充默认值 |
| `dto.PageOutput[T]` | **包装** usecase 出参 | 含 `Items []T` 与 `Total int64`；Execute 直接返回 `*dto.PageOutput[TItem]`，不另建 `XxxOutput` 类型别名 |

```go
type QueryUsersInput struct {
    Username string `json:"username"`
    dto.PageInput
}

func (uc *QueryUsersUseCase) Execute(ctx context.Context, input *QueryUsersInput) (*dto.PageOutput[QueryUsersItem], error) {
    input.PageInput.ValiPage() // 或在 Validate() 中调用
    // ...
    return &dto.PageOutput[QueryUsersItem]{Items: items, Total: total}, nil
}
```

handler 侧：`platformhandler.BindAndHandlePage(c, h.QueryUsers.Execute)`，无需手动拆 `Items` 与 `Total`。

### 3.4 前后端交互时间类型

前后端交互时，时间类型的选型应遵循以下规则。**本规则仅约束 HTTP 接口的出入参结构体**（即 usecase 层的 `*Input` / `*Output` 以及 handler/test 中直接对应 JSON 响应的结构体），不约束 domain/contract Entity、领域 DTO、GORM Model（`internal/biz/repo/db/{module}/` 包内）等内部类型。

| 场景 | 类型 | 说明 |
|------|------|------|
| 时间点（含时分秒） | `int64` / Unix 时间戳（毫秒） | 默认使用 Unix 时间戳，单位为毫秒；JSON 字段名使用 camelCase（如 `createTime`） |
| 日期（仅年月日） | `string` | 格式如 `"2006-01-02"`，需在 DTO 注释中额外说明日期含义 |
| 其他特殊格式 | `string` | 需在 DTO 注释中说明格式和含义 |

**禁止在 HTTP 接口出入参中直接使用 `time.Time`**，以避免序列化时因时区、格式不一致导致前后端歧义。

```go
// 正确：时间点使用 int64 时间戳，JSON 字段名使用 camelCase
// 作者: zwei
type UserDTO struct {
    CreateTime int64 `json:"createTime"` // 创建时间，Unix 时间戳（毫秒）
}

// 正确：日期使用 string
// 作者: zwei
type UserDTO struct {
    Birthday string `json:"birthday"` // 出生日期，格式：2006-01-02
}

// 错误：禁止直接使用 time.Time
// 作者: zwei
// type UserDTO struct {
//     CreateTime time.Time `json:"createTime"`
// }
```

### 3.5 枚举定义与使用规范

#### 3.5.1 枚举独立定义

不同领域模块的枚举必须完全独立定义。API 层枚举定义在 `{client}/{domain}/enums/`（**领域 client**，业务工程独立模块）；领域 Entity/Model 引用对应 client 枚举包。

```go
// 正确：各领域各自独立定义状态枚举（领域 client 包；示例域）
// {client}/{domain-a}/enums/resource_status.go
type ResourceAStatus string
// {client}/{domain-b}/enums/resource_status.go
type ResourceBStatus string

// 错误：领域 A 复用领域 B 的枚举
// type ResourceAModel struct {
//     Status enums.ResourceBStatus // 禁止
// }
```

#### 3.5.2 模型与 DTO 字段使用枚举类型

领域模型和领域 DTO 中应为枚举语义的字段一律使用枚举类型，不得使用裸 `string`。

```go
// 正确：Model 和 DTO 使用枚举类型
type MemberModel struct {
    Gender enums.MemberGender `gorm:"column:gender" json:"gender,omitempty"`
    Status enums.MemberStatus `gorm:"column:status" json:"status"`
}

type CreateInput struct {
    Gender enums.MemberGender
}

type QueryPageInput struct {
    Status enums.MemberStatus
}

type UpdateStatusInput struct {
    Status enums.MemberStatus
}

// 错误：使用裸 string
// type MemberModel struct {
//     Gender string
//     Status string
// }
```

#### 3.5.3 usecase 层枚举直达

usecase 层的输入/输出结构体中枚举字段直接使用枚举类型，JSON 序列化由底层 `string` 类型自动处理。**禁止**用 `string` 接收后再做类型转换，也**禁止**为 `string→枚举` 的转换编写归一化中间层函数。

```go
// 正确：usecase I/O 直接用枚举类型
type CreateMemberInput struct {
    Gender enums.MemberGender `json:"gender,omitempty"`
}

type CreateMemberOutput struct {
    Status enums.MemberStatus `json:"status"`
}

// Execute 中直接赋值，无需转换
func (uc *CreateMemberUseCase) Execute(ctx context.Context, input *CreateMemberInput) (*CreateMemberOutput, error) {
    // ...
    return &CreateMemberOutput{
        Status: created.Status, // 直接赋值，不用 string(created.Status)
    }, nil
}

// 错误：用 string 中转再转换
// type CreateMemberInput struct {
//     Gender string `json:"gender,omitempty"`
// }
// Gender: enums.MemberGender(input.Gender), // 禁止
// Status: string(created.Status),           // 禁止
```

#### 3.5.4 枚举默认实现校验与反序列化方法

使用 `const` + `string` 类型实现的枚举（如 `type XxxStatus string`），默认必须实现 `IsValid` 校验函数和 `UnmarshalJSON` 反序列化方法。`UnmarshalJSON` 内部应完成归一化并调用 `IsValid` 校验，确保非法值在反序列化阶段即被拦截。若因特殊原因无需实现，必须在枚举定义处显式说明理由。

```go
// 正确：枚举默认实现 IsValid + UnmarshalJSON
// 示例：{client}/{domain}/enums/resource_status.go
type ResourceStatus string

const (
    ResourceStatusEnabled  ResourceStatus = "ENABLED"
    ResourceStatusDisabled ResourceStatus = "DISABLED"
)

// IsValid 判断资源状态是否为合法枚举。
func IsValidResourceStatus(s ResourceStatus) bool {
    return s == ResourceStatusEnabled || s == ResourceStatusDisabled
}

// UnmarshalJSON 实现 json.Unmarshaler，反序列化时自动归一化并校验。
func (s *ResourceStatus) UnmarshalJSON(data []byte) error {
    var str string
    if err := json.Unmarshal(data, &str); err != nil {
        return err
    }
    val := ResourceStatus(strings.ToUpper(strings.TrimSpace(str)))
    if val == "" {
        *s = ""
        return nil
    }
    if !IsValidResourceStatus(val) {
        return fmt.Errorf("invalid ResourceStatus: %s", str)
    }
    *s = val
    return nil
}

// 正确：特殊情况无需实现，必须显式说明理由
// BizError 为 struct 类型（`gitee.com/hq_project/yomi-boot/component/core/errors.BizError`），哨兵变量定义在各服务 `bizerr/errors.go`，无需 IsValid/UnmarshalJSON。
```

### 3.6 金额字段默认使用 Decimal 类型

#### 3.6.1 背景

金额、价格、费率等金融数据必须精确表示和运算。Go 原生 `float32` 和 `float64` 采用 IEEE 754 二进制浮点表示，无法精确表示十进制小数（如 `0.1`），在累加、乘除等运算中会累积误差，不适用于需要精确到分的金额计算场景。

#### 3.6.2 类型选型

| 场景 | 类型 | 说明 |
|------|------|------|
| 金额/价格/费率等精确数值 | `decimal.Decimal` | 使用 `github.com/shopspring/decimal`，可精确表示十进制小数 |
| 纯计数（如库存数量、人数） | `int64` | 整数场景使用原生整数类型 |
| 百分比/比率（仅展示用） | `decimal.Decimal` | 同样禁止使用浮点型 |

**禁止在任何涉及金额的字段中使用 `float32` 或 `float64`**。

> `decimal.Decimal` 原生实现了 `json.Marshaler`/`json.Unmarshaler` 和 `sql.Scanner`/`driver.Valuer` 接口，可直接用于 JSON 序列化和 GORM 持久化，无需额外适配。

#### 3.6.3 各层使用规范

```go
// 正确：Model、DTO、Input/Output 中金额字段统一使用 decimal.Decimal
// 作者: zwei
import "github.com/shopspring/decimal"

type ProductModel struct {
    Price    decimal.Decimal `gorm:"column:price" json:"price"`       // 价格
    Discount decimal.Decimal `gorm:"column:discount" json:"discount"` // 折扣
}

type CreateProductInput struct {
    Price    decimal.Decimal `json:"price"`
    Discount decimal.Decimal `json:"discount"`
}

type ProductDTO struct {
    Price    decimal.Decimal `json:"price"`
    Discount decimal.Decimal `json:"discount"`
}

// 错误：禁止使用 float64 表示金额
// type ProductModel struct {
//     Price    float64 `gorm:"column:price" json:"price"`       // 禁止
//     Discount float64 `gorm:"column:discount" json:"discount"` // 禁止
// }
```

#### 3.6.4 运算规范

涉及金额的加减乘除必须使用 `decimal.Decimal` 提供的方法，禁止转换为 `float64` 后运算再转回。

```go
// 正确：使用 decimal 方法进行金额运算
// 作者: zwei
func calculateTotal(price, discount decimal.Decimal) decimal.Decimal {
    return price.Sub(discount) // BigDecimal 风格，精确运算
}

func applyTax(amount decimal.Decimal, rate decimal.Decimal) decimal.Decimal {
    return amount.Mul(decimal.NewFromInt(1).Add(rate))
}

// 错误：禁止转为 float64 运算
// func calculateTotal(price, discount decimal.Decimal) decimal.Decimal {
//     f, _ := price.Float64()  // 禁止
//     d, _ := discount.Float64() // 禁止
//     return decimal.NewFromFloat(f - d) // 禁止
// }
```

#### 3.6.5 数据库字段映射

MySQL 中金额字段使用 `DECIMAL(precision, scale)` 类型，通过 GORM 自动映射到 `decimal.Decimal`。

```sql
-- 金额字段使用 DECIMAL 类型
CREATE TABLE t_product (
    id       BIGINT NOT NULL,
    price    DECIMAL(12,2) NOT NULL DEFAULT 0.00 COMMENT '价格',
    discount DECIMAL(12,2) NOT NULL DEFAULT 0.00 COMMENT '折扣'
);
```

### 3.7 通用工具函数统一管理

跨模块复用的通用辅助函数统一收口到 `gitee.com/hq_project/yomi-boot/component/core/utils/` 对应文件中，消除各处私有重复定义。

```go
// 正确：通用函数统一收口到 gitee.com/hq_project/yomi-boot/component/core/utils/ 子包
// gitee.com/hq_project/yomi-boot/component/core/utils/ptrutil/ptr.go
func PtrFromString(value string) *string { /* ... */ }
func StringFromPtr(value *string) string { /* ... */ }

// 错误：在各自模块中重复定义
// internal/biz/domain/service/member/member_service.go
// func nullableString(value string) *string { ... }  // 禁止
// internal/biz/usecase/user/create_user_usecase.go
// func stringFromPtr(value *string) string { ... }    // 禁止
```

### 3.8 雪花 ID 字段类型规范（JSONInt64 / JSONInt64Slice）

#### 3.8.1 背景

项目使用雪花算法（Snowflake）生成分布式主键，ID 值远超 JavaScript `Number.MAX_SAFE_INTEGER`（2^53 ≈ 9×10^15）。若以 JSON 数字传输，前端接收时必然丢失精度。因此所有 API 出入参中的 id/ids 语义字段，必须使用专用的 `JSONInt64` / `JSONInt64Slice` 类型。

#### 3.8.2 类型定义

| 类型 | 定义位置 | 用途 |
|------|----------|------|
| `dto.JSONInt64` | `gitee.com/hq_project/yomi-boot/component/core/shared/dto/json_int64.go` | 单个 ID：接受 JSON 字符串或数字反序列化，序列化时强制输出 JSON 字符串 |
| `dto.JSONInt64Slice` | `gitee.com/hq_project/yomi-boot/component/core/shared/dto/json_int64_slice.go` | ID 列表：接受 JSON 字符串数组或数字数组反序列化，序列化时强制输出 JSON 字符串数组 |
| `dto.NewJSONInt64Slice([]int64)` | 同上 | 将 `[]int64` 转为 `JSONInt64Slice` 的构造函数 |

#### 3.8.3 API 出入参规范

| 场景 | 字段语义 | 必须使用的类型 | 禁止使用的类型 |
|------|----------|---------------|---------------|
| 入参单个 ID | `id`、`{entity}Id`、`relId` 等（**示例**） | `dto.JSONInt64` | `int64`、`string` |
| 入参 ID 列表 | `roleIds` 等 | `dto.JSONInt64Slice` | `[]int64`、`[]string` |
| 出参单个 ID | `id`、`{entity}Id`、`relId` 等（**示例**） | `dto.JSONInt64` | `int64`（含 `json:",string"`）、`string` |
| 出参 ID 列表 | `roleIds` 等 | `dto.JSONInt64Slice` | `[]int64`、`[]string` |

> **关键规则**：出参 ID 字段**禁止**使用 `int64` + `json:",string"` 混合模式（Go 原生的 `,string` 标签只能序列化不能灵活反序列化），必须统一使用 `dto.JSONInt64`。

#### 3.8.4 构造赋值模式

```go
// 正确：从 int64 显式转换为 dto.JSONInt64
// 作者: zwei
type GetProfileOutput struct {
    ID      dto.JSONInt64      `json:"id"`
    RoleIds dto.JSONInt64Slice `json:"roleIds"`
}

func buildOutput(resource *domainresource.Resource, roleIDs []int64) *GetProfileOutput {
    return &GetProfileOutput{
        ID:      dto.JSONInt64(resource.ID),
        RoleIds: dto.NewJSONInt64Slice(roleIDs),
    }
}

// 错误：混用 int64 + json:",string"
// type GetProfileOutput struct {
//     ID int64 `json:"id,string"`  // 禁止
// }

// 错误：直接赋值 int64 给 JSONInt64 字段
// ID: model.ID  // 编译错误，需显式转换
```

#### 3.8.5 排除范围

以下场景**无需**使用 JSONInt64，保持原生 `int64`/`[]int64`：

- **Redis 内部存储结构体**（如 `TokenSession`），其 `json` 标签仅用于序列化到 Redis，不暴露给前端
- **使用 `json:"-"` 隐藏的字段**（如 usecase 内部写入、不参与 JSON 绑定的 `OperatorID`），该字段不参与 JSON 序列化
- **domain/contract Entity 内部字段**（如**示例** `domainresource.Resource.ID`），Entity 不与前端直接交互
- **GORM Model 内部字段**（如**示例** `resourceModel`），Model 仅在 repository 包内使用

```go
// 正确：TokenSession 是 Redis 内部结构，无需 JSONInt64
// 作者: zwei
type TokenSession struct {
    UserID  int64   `json:"user_id"`
    RoleIDs []int64 `json:"role_ids"`
}

// 正确：json:"-" 隐藏字段无需 JSONInt64（由 usecase 从 context 写入，非前端入参）
// 作者: zwei
type CreateRoleInput struct {
    RoleCode    string `json:"roleCode"`
    RoleName    string `json:"roleName"`
    Description string `json:"description"`
    OperatorID  int64  `json:"-"`
}
```

### 3.9 接口实现编译期校验

凡 struct 显式实现某接口（如 `{Module}Service`、`{Module}Repository`、`RedisService`、`tx.Executor` 等），**必须**在实现类型定义之后、构造函数之前添加编译期断言，确保接口方法增删或签名变更时编译器立即报错，而不是等到 bootstrap 装配或运行期才暴露遗漏。

```go
// 正确：实现类型定义后立即声明编译期断言
type RedisHelper struct {
    client *redisv9.Client
}

var _ RedisService = (*RedisHelper)(nil)

func NewRedisHelper(client *redisv9.Client) *RedisHelper {
    return &RedisHelper{client: client}
}
```

| 场景 | 写法 | 说明 |
| ---- | ---- | ---- |
| 指针接收者实现接口 | `var _ XxxService = (*XxxImpl)(nil)` | 项目内默认方式；`*Impl` 须满足接口全部方法 |
| 值接收者实现接口 | `var _ Xxx = Xxx{}` | 仅当实现 struct 全部方法均为值接收者时使用 |

**放置位置**：与实现 struct 同文件、紧邻 struct 定义之后；一行断言即可，无需额外注释。

**适用范围**：

- `gitee.com/hq_project/yomi-boot/` 下的基础设施实现（见 [Go架构规范](./Go架构规范.md)）
- `internal/biz/domain/service/{module}/` 对 `domain/contract` 中 `{Module}Service` 的实现
- `internal/biz/repo/db/{module}/` 对 `repo/contract` 中 `{Module}Repository` 的实现
- 其他对外暴露、需被替换或 mock 的接口实现（如事件 Handler、Outbox Publisher 等）

**不要求**添加断言的场景：usecase、handler 等通常不声明「实现某接口」的类型；测试文件内的 mock 实现可省略（同文件内仅用于单测时）。

```go
// 正确：repository 实现编译期校验
type Repository struct {
    orm.BaseRepository
}

var _ userrepocontract.UserRepository = (*Repository)(nil)

// 错误：缺少编译期断言，接口方法变更时可能静默遗漏
// type Repository struct { orm.BaseRepository }
// func NewRepository(...) *Repository { ... }
```

---

## 4. 检查清单

- [ ] usecase 的 `Validate()` 覆盖入参基础校验（判空、格式、Trim 等），不承担领域不变量
- [ ] service 承担同域编排、领域规则与不变量校验，repository 不承载业务校验
- [ ] usecase 与 service 未重复实现同一校验规则
- [ ] usecase 只做应用编排（Validate → idempotency/TxRunner → service），跨域编排只在 BFF
- [ ] 各层方法未对注入依赖做 `== nil` 判空
- [ ] 显式实现接口的 struct（Service、Repository、platform 能力等）已添加 `var _ Interface = (*Impl)(nil)` 编译期断言
- [ ] Redis 读写通过 `redis.RedisService` 完成，业务与 platform 消费方未直接使用 `*redis.Client`；业务层不调用 `redisplugin.Redis.Client()`
- [ ] 业务接口统一使用 POST + JSON body，health 探针使用 GET
- [ ] DTO 中时间字段使用 `int64` 时间戳或 `string` 日期，JSON 字段名使用 camelCase（如 `createTime`），不直接使用 `time.Time`
- [ ] 各领域模块的枚举独立定义，不复用其他模块的枚举类型
- [ ] Model 和 DTO 中枚举语义字段使用枚举类型，不使用裸 `string`
- [ ] usecase I/O 中枚举字段直接用枚举类型，不通过 `string` 中转转换
- [ ] 枚举辅助函数为包级纯函数，定义在对应枚举文件中
- [ ] 使用 const 实现的枚举默认实现了 `IsValid` 和 `UnmarshalJSON`，非法值在反序列化时即被拦截
- [ ] 金额、价格、费率等精确数值字段使用 `decimal.Decimal`，未使用 `float32`/`float64`
- [ ] 金额运算使用 `decimal.Decimal` 方法，未转换为浮点再运算
- [ ] 数据库金额字段使用 `DECIMAL(precision, scale)` 类型
- [ ] 通用工具函数统一收口在 `gitee.com/hq_project/yomi-boot/component/core/utils/` 对应文件中
- [ ] API 出入参中 id 语义字段（**示例** `id`、`{entity}Id`、`relId`）使用 `dto.JSONInt64`
- [ ] API 出入参中 ids 语义字段（roleIds 等）使用 `dto.JSONInt64Slice`
- [ ] 出参 ID 字段未使用 `int64` + `json:",string"` 混用模式
- [ ] 构造 DTO 时已显式转换：`ID: dto.JSONInt64(model.ID)`、`RoleIds: dto.NewJSONInt64Slice(roleIDs)`
- [ ] Redis 内部结构体和 `json:"-"` 字段保持原生 `int64`，未误用 JSONInt64

---

## 5. 禁止项

- 禁止 repository 层做模型字段非空、ID 有效性等业务校验。
- 禁止各层方法在运行期对注入依赖做防御式判空，以此替代启动阶段的装配验证。
- 禁止显式实现接口的 struct 缺少 `var _ Interface = (*Impl)(nil)` 编译期断言，导致接口方法不完整时无法被编译器拦截。
- 禁止在 `gitee.com/hq_project/yomi-boot/component/common/redis/` 与 BFF 令牌装配之外，业务层直接使用 `*redis.Client`。
- 禁止业务层调用 `redisplugin.Redis.Client()`；原始客户端只允许 `plugin/builtin` 内部插件在初始化期复用，连接由 redis 插件在 `Init` 内创建，经 `Service()` 对外暴露，并在插件 `Close()` 中关闭。
- 禁止 Routes 方法内对 `router` 做 `== nil` 判空。
- 禁止业务接口使用 GET + query 参数作为主要输入形式，除非是 health 探针或明确的 CDN/SEO 包装层。
- 禁止 DTO 中直接使用 `time.Time` 类型作为字段类型与前端交互。
- 禁止跨模块复用枚举类型（如 Member 使用 `enums.UserStatus`）。
- 禁止 Model 或 DTO 中枚举语义字段使用裸 `string` 代替枚举类型。
- 禁止 usecase 层用 `string` 接收枚举参数后再做 `enums.Xxx(input.Status)` 类型转换。
- 禁止在 usecase 层编写 `string→枚举` 的归一化中间层函数。
- 禁止 const 实现的枚举缺少 `IsValid` 和 `UnmarshalJSON` 方法，除非在定义处显式说明无需实现的理由。
- 禁止在 service 层对枚举字段做重复校验（校验应在 UnmarshalJSON 或 usecase 层完成）。
- 禁止 API 出入参中 id 语义字段使用裸 `int64` 或 `int64` + `json:",string"` 代替 `dto.JSONInt64`。
- 禁止 API 出入参中 ids 语义字段使用 `[]int64` 代替 `dto.JSONInt64Slice`。
- 禁止将 `dto.JSONInt64` / `dto.JSONInt64Slice` 用于 Redis 内部存储或 `json:"-"` 隐藏字段。
- 禁止在任何涉及金额、价格、费率等精确数值的字段中使用 `float32` 或 `float64`。
- 禁止将 `decimal.Decimal` 转为 `float64` 进行运算后再转回 `decimal.Decimal`。
- 禁止数据库金额字段使用 `FLOAT` 或 `DOUBLE` 类型代替 `DECIMAL`。
