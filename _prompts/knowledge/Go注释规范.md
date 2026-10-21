# Go注释规范

---

## 0. 文档信息
- **文档名称**: Go注释规范
- **描述**: 统一 Go 后端代码注释口径，覆盖文件头注释、类型与字段注释、接口与函数注释、常量与枚举注释以及关键步骤注释写法。
- **作者**: zwei
- **创建日期**: 2026-06-01

---

## 1. 文档定位

- **用途**：作为 gitee.com/hq_project/yomi-boot 本库及基于 gitee.com/hq_project/yomi-boot 的业务工程中 Go 注释口径的单一事实来源，统一注释写法、落位和最小覆盖范围。
- **边界**：不覆盖业务架构设计、分层职责和接口契约设计，相关内容由 [Go业务分层规范](./Go业务分层规范.md)、[GoBFF规范](./GoBFF规范.md) 和 [Go数据库规范](./Go数据库规范.md) 约束。
- **适用对象**：Go 后端开发、重构、代码评审和巡检场景。

---

## 2. 适用范围与关联

### 2.1 适用范围

- **适用于** `gitee.com/hq_project/yomi-boot/` 本库各包，以及基于 gitee.com/hq_project/yomi-boot 的业务工程可部署单元（惯例示例：`services/{unit}/cmd/`、`services/{unit}/internal/biz/`）下 `.go` 文件的注释编写。
- 重点覆盖文件头注释、类型与字段注释、函数与方法注释、接口声明注释、测试方法注释以及方法体关键步骤注释。
- 不要求为显而易见的标准库调用、纯 getter/setter 或同名直译代码添加冗余注释。

### 2.2 关联文档

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| `_prompts/knowledge/Go业务分层规范.md` | 业务分层 | 约束注释所在文件的目录结构和分层职责 |
| `_prompts/knowledge/Go架构规范.md` | 项目架构 | 约束 cmd、bootstrap、platform 目录结构 |
| `_prompts/knowledge/Go数据库规范.md` | 数据库访问 | 约束 Model 和 Repository 的注释口径 |
| `_prompts/knowledge/Go选型规范.md` | 技术栈 | 约束默认库和第三方库的注释约定 |
| `_prompts/knowledge/Go集成测试规范.md` | 集成测试注释 | 约束集成测试方法注释为一行测试目的说明 |

### 2.3 阅读建议

- 统一注释风格时，优先阅读"基本原则"和"注释场景与写法"章节。
- 编写 Handler、UseCase、Service、Repository 时，结合 [Go业务分层规范](./Go业务分层规范.md) 一起核对注释落位。
- 评审代码时，先判断是否真的需要注释，再判断注释是否符合本文口径。

---

## 3. 注释规范

### 3.1 基本原则

| 规则 | 要求 | 说明 |
| ---- | ---- | ---- |
| 语言 | 注释必须使用中文 | 禁止英文注释或中英混写注释；Go 文档注释的约定英文短语（如 `//nolint`、`//go:embed`）除外 |
| 形式 | 函数、类型、常量等所有声明级符号统一使用 `// 注释` 双斜线 | 注释紧贴被注元素上方，不空行 |
| 作者信息 | 作者标识（如 `@author zwei`）只在文件头块注释中出现 | 函数/方法级注释和 type/interface 声明级注释不需要写作者 |
| 句号 | 不是长难句的注释默认不用句号结尾 | 短句、短语式注释保持直述和可扫读性；只有长难句、多分句或完整断句说明时再使用句号 |
| 注释目标 | 注释意图、边界、前置条件、返回值约定 | 不重复翻译变量名和显而易见的代码 |
| 优先级 | 优先改进命名和拆分，再补注释 | 注释不能用来掩盖混乱结构 |
| 文档注释 | 所有函数必须包含注释，遵循 Go 文档注释惯例 | 以函数名开头，例如 `// Execute 执行用户注册流程` |

### 3.2 文件头注释

每个 `.go` 文件顶部必须包含文件头注释，使用 `// Package` 行描述职责，后跟 `// @author` 作者标识：

```go
// Package appctx 提供应用静态上下文能力
//
// @author zwei
package appctx
```

或用于 handler 入口文件：

```go
// Package handler 入口编排 HTTP 协议适配（示例包名随业务工程调整）
//
// @author zwei
package handler
```

规则：

- 文件头注释结构为：第一行 `// Package xxx ...` 描述包整体职责，空行后 `// @author xxx`。
- `main.go` 文件头注释描述入口用途，同样后跟 `@author`：

```go
// 后台服务入口
//
// @author zwei
package main
```
- 如果文件是某个包下的具体实现文件（如 `repository.go`、`service.go`），不强制在每个文件重复包注释，但应在该包至少一个文件（通常为 `model.go` 或 `xxx.go`）中保留包注释。

### 3.3 类型与字段注释

所有类型定义（结构体、接口）必须带有注释，描述该类型的用途：

```go
// UserQuery 用户列表查询条件
type UserQuery struct {
    Keyword string `json:"keyword"` // 关键字模糊搜索
    Status  string `json:"status"`  // 状态筛选，为空则不限制
    Page    int    `json:"page"`    // 页码，从 1 开始
    Size    int    `json:"size"`    // 每页条数
}
```

规则：

- 类型注释写在类型声明上方，格式为 `// 类型名 描述`。
- 导出字段的注释写在行尾或字段上方，说明字段的用途、格式约束或默认行为。
- 如果字段名已自解释（如 `UserName string`），可省略字段注释。
- 非导出字段在可能引起歧义时补充注释。

### 3.4 函数与方法注释

所有函数必须包含注释，遵循 Go 文档注释惯例：

```go
// NewRepository 创建用户仓储，返回 contract 接口类型
func NewRepository(db *gorm.DB) userrepocontract.UserRepository {
    return &Repository{BaseRepository: db.BaseRepository{DB: db}}
}

// FindByID 根据主键查询用户，返回 nil 表示不存在
func (r *Repository) FindByID(ctx context.Context, id int64) (*domainuser.User, error) {
    // ...
}
```

规则：

- 注释格式：`// 函数名 功能描述`。
- 如果返回值有特殊约定（如返回 nil、空列表等），必须在注释中说明。
- 如果参数有约束（如非空、范围限制等），应在注释中说明。
- 非导出函数与导出函数同等对待，一概需要注释。
- 复杂函数（多参数、多返回值）建议使用结构化 godoc 注释，分段说明参数与返回值：

```go
// FirstByID 根据主键 id 查询单条记录
//
// 参数：
//   - ctx: 上下文，用于事务传递
//   - id: 主键值
//   - dest: 结果接收指针
//
// 返回值：
//   - bool: true 表示找到记录，false 表示未找到
//   - error: 数据库错误
func (r *BaseRepository) FirstByID(ctx context.Context, id int64, dest any) (bool, error) {
    // ...
}
```

#### 3.4.1 接口声明

接口声明必须包含注释，描述该接口的职责和方法契约：

```go
// EventHandler 事件处理器，负责处理特定类型的事件
type EventHandler interface {
    // Handle 处理事件，返回 error 表示处理失败
    Handle(ctx context.Context, event Event) error
}
```

规则：

- 接口本身必须有注释。
- 接口中的每个方法建议有注释，尤其是涉及边界条件或返回值约定时。

### 3.5 常量与枚举注释

#### 3.5.1 业务错误码

业务错误码使用 `var` 块定义哨兵；`Error()`（或 `BizError.Code`）即为线上 code，中文语义写在行注释：

```go
var (
    // ErrUserNotFound 用户不存在
    ErrUserNotFound = errors.NewBiz("USER_NOT_FOUND", "用户不存在")
    // 或 errors.New("USER_NOT_FOUND")
)
```

规则：

- 跨库通用哨兵见 `gitee.com/hq_project/yomi-boot/boot/shared/error`；库私有就近定义；域错误在服务 `bizerr`。
- 错误码命名规则见 [Go异常规范](./Go异常规范.md) §3.4。

### 3.6 关键步骤注释

在方法体内部，关键业务步骤、边界判断或复杂逻辑需要添加行内注释：

```go
func (uc *CreateUserUseCase) Execute(ctx context.Context, input *CreateUserInput) error {
    // 校验入参
    if err := input.Validate(); err != nil {
        return err
    }

    _, err := idempotency.Execute(uc.idemGuard, uc.txRunner, ctx, idempotency.Key{
        BizID: input.BizID, BizSubID: input.BizSubID, OperationType: operationTypeCreateUser,
    }, func(innerCtx context.Context) (CreateUserOutput, error) {
        // 调用领域 Service 创建用户
        user, err := uc.userService.Create(innerCtx, &domainuser.CreateInput{...})
        if err != nil {
            return CreateUserOutput{}, err
        }
        return toCreateUserOutput(user), nil
    })
    return err
}
```

规则：

- 关键步骤注释放在代码上方，使用 `// 描述` 格式。
- 不言自明的标准库调用或链式调用不需要注释。
- 分支条件（if/else）如果条件语义不直观，应在条件上方注释说明判断依据。

### 3.7 特殊标记注释

#### 3.7.1 TODO / FIXME / HACK

```go
// TODO(zwei): 后续接入消息队列后替换为异步通知
// FIXME: 当前实现未处理并发场景，后续需加锁
// HACK: 临时兼容客户端字段名，下个版本移除
```

规则：

- TODO / FIXME / HACK 必须附带原因或期限，禁止裸写无上下文的 `TODO`。
- 已知责任人可写在括号内，如 `TODO(zwei):`。

#### 3.7.2 nolint 与 go:embed

```go
//go:embed config/*.yml
var configFiles embed.FS

//nolint:unused // 由 GORM 回调使用，静态检查误报
func helper() {}
```

规则：

- `//go:embed`、`//nolint` 等指令注释保留英文，不翻译为中文。
- `//nolint` 必须附带注释说明忽略原因。

---

## 4. 检查清单

- [ ] 每个包至少有一个文件包含包注释（`// Package xxx ...`）
- [ ] 所有类型定义都包含类型注释
- [ ] 所有函数都包含注释（不区分导出/非导出）
- [ ] 导出字段在名称不自解释时包含行尾注释
- [ ] 函数注释说明了返回值特殊约定（nil、空列表等）
- [ ] 方法体内关键步骤包含行内注释
- [ ] TODO / FIXME 附带原因或期限
- [ ] 注释使用中文，文件头无作者信息外的冗余元信息
- [ ] 函数/方法级注释未出现 `@author` 等作者标识
- [ ] 文件头注释包含 `@author` 作者标识说明（`// @author xxx`）
- [ ] 复杂函数使用了结构化 godoc 注释（参数/返回值分段说明）

---

## 5. 禁止项

- 禁止使用英文注释或中英混写注释（Go 指令注释 `//go:embed`、`//nolint` 除外）。
- 禁止在函数/方法级注释和 type/interface 声明级注释中写 `@author` 作者标识。
- 禁止为自解释的代码（如 `return nil`、简单的字段赋值）添加冗余注释。
- 禁止裸写无上下文的 `TODO` 或 `FIXME`，必须附带原因或期限。
- 禁止用注释掩盖混乱结构或不合理的命名，应先重构再注释。
