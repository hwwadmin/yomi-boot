# Go数据库规范

---

## 0. 文档信息
- **文档名称**: Go数据库规范
- **描述**: 定义 Go 后端数据库访问的分层职责、GORM Model 映射规则、链式查询调用方式以及事务控制规范。
- **作者**: zwei
- **创建日期**: 2026-06-01

---

## 1. 文档定位

- **用途**：约束 Go 项目中数据库访问层的实现方式，包括 GORM Model 设计与 `orm.BaseModel` 组合复用、链式查询调用、事务感知的 repository 实现，确保数据访问层统一、可维护。复杂查询可回退到原生 SQL。
- **边界**：不覆盖表命名、字段设计、索引策略和 DDL 编写细则，相关内容由数据库表设计规范约束；不覆盖数据库运维、部署和迁移流程。
- **适用对象**：所有使用 `gorm.io/gorm` 作为数据库访问方案的 Go 项目。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于基于 gitee.com/hq_project/yomi-boot 的业务工程中 repository 实现（惯例目录示例：`services/{unit}/internal/biz/repo/`）。
- 适用于 Go Model 与数据库表之间的 GORM 映射约定，以及 `orm.BaseModel` 的组合复用。
- 适用于 GORM 链式查询和原生 SQL 混合使用的场景。

### 2.2 关联文档

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| `_prompts/knowledge/Go业务分层规范.md` | 业务分层 | 约束 domain/repo 分层职责与 Entity 建模口径 |
| `_prompts/knowledge/Go架构规范.md` | 项目架构 | 约束 repository 层在整体项目结构中的位置 |
| `_prompts/knowledge/Go事务规范.md` | 事务控制 | 约束 `tx.Executor` 接口、`tx.Manager` 实现、Context 事务传递与 repository 事务感知实现 |
| `_prompts/knowledge/Go选型规范.md` | 技术栈基线 | 约束 gorm 作为默认 SQL 访问库的选型理由 |
| `_prompts/knowledge/数据库表设计规范.md` | 表设计规范 | 约束表命名、字段、索引与 DDL 设计，Go Model 须与其保持一致 |
| `_prompts/knowledge/Go注释规范.md` | 注释规范 | 约束 Model 和 Repository 的注释口径 |

### 2.3 阅读建议

- 实现数据访问层时，先阅读"Entity 与 Model 映射规则"；**目录分层与 domain/repo 职责**以 [Go业务分层规范](./Go业务分层规范.md) 为准。
- 编写 repository 时，参考"Repository 层职责与示例"和 [Go事务规范](./Go事务规范.md)。

---

## 3. 技术方案

### 3.1 目录结构

数据访问代码位于 `internal/biz/repo/`（接口在 `repo/contract/`，实现在 `repo/db/`）；Entity/DTO 在 `internal/biz/domain/contract/`。完整目录树与分层职责见 [Go业务分层规范](./Go业务分层规范.md) §3.2、§3.5；文件命名见 [Go文件命名规范](./Go文件命名规范.md)。

数据访问以 GORM 链式 API 为主。DDL 分两类维护：

- **本服务业务表**：`services/{unit}/script/sql/`（含 `00_database.sql` 建库脚本）
- **平台共享表**（`sys_biz`、`sys_barrier`）：权威 DDL 与运行时封装见 [微服务调用规范](./微服务调用规范.md) §3.4.2；各领域库初始化时须在本库额外执行对应 SQL，**禁止**在 `services/{unit}/script/sql/` 复制；领域服务不在 `repo/` 重复实现 `sys_biz` 访问。

#### 3.1.1 微服务数据库隔离与命名

- **一服务一库**：每个可部署单元使用独立 MySQL 数据库，禁止跨单元共库、跨库 JOIN 或外键。
- **命名规范**：`yomi_go_{unit}`，其中 `{unit}` 为短名（领域单元可去掉 `-service` 后缀；入口编排保留 `-bff` 语义时用下划线），例如 `{domain-service}` → `yomi_go_{domain}`，`{bff}` → `yomi_go_{bff}`。
- **入口编排允许使用本进程独立库**：配置 `db` 段并初始化 GORM，仅承载本进程基础设施表（如事件收件箱）；**禁止**连接领域库或在编排库中建领域业务表。令牌 Store 与业务缓存仍用 **Redis**。
- **初始化脚本**：各单元在 `services/{unit}/script/sql/` 维护 `00_database.sql`（建库）及本单元表；编排层事件收件箱可引用 `gitee.com/hq_project/yomi-boot/sql/sys_event.sql` 中的 inbox 段；平台共享表（`sys_biz`/`sys_barrier`）按 [微服务调用规范](./微服务调用规范.md) §3.4.2 在领域写库场景执行。
### 3.2 GORM 查询方式

#### 3.2.1 基本原则

- **优先使用 GORM 链式查询**。常规 CRUD 操作使用 GORM 的链式 API（`Where`、`Find`、`First`、`Create`、`Save`、`Delete` 等）。
- **复杂查询回退到原生 SQL**。当链式查询难以表达（如多表复杂 JOIN、窗口函数、递归 CTE）时，使用 `gorm.DB.Raw()` 执行原生 SQL；SQL 文本放在 repository 文件内私有常量，使用 `?` 占位传参，禁止拼接用户输入。
- 查询结果由 GORM 自动扫描到 Model 结构体，无需手写行解析。

#### 3.2.2 GORM 链式查询规范

##### 基本规则

- 优先使用 GORM 链式查询。常规 CRUD 操作使用 GORM 的链式 API。
- 复杂查询回退到原生 SQL。
- **执行器获取**：统一通过 `tx.Exec(ctx, r.DB)` 获取 GORM 实例（自动识别事务上下文）。
- **条件片段**：`Where` 条件使用 GORM 标准占位符写法（如 `"status = ?"`、`"id IN ?"`）；列名须为代码中固定的数据库字段名，禁止将用户输入或变量拼接到 SQL 片段中。
- **LIKE 查询**：模糊匹配的值须通过 `orm.LikeFull` / `orm.LikeLeft` / `orm.LikeRight` 转义通配符后传入。
- **分页**：使用 `orm.ApplyPage(query, page, pageSize, order)` 或基类 `r.CountPage(ctx, query, page, pageSize, order, &rows)`。
- **基类优先**：单行/批量 CRUD 优先使用 `BaseRepository` 提供的 `FirstByID`、`FirstByField`、`UpdateByID` 等方法，避免重复实现。

##### 示例

```go
// 单行查询（repository 内部使用 userModel）
var model userModel
found, err := r.FirstByField(ctx, "username", username, &model)

// 多行查询
var list []userModel
err := tx.Exec(ctx, r.DB).Where("status = ?", status).Find(&list).Error

// 条件分页（LIKE + 等值）
query := tx.Exec(ctx, r.DB).Model(&userModel{})
if input.Username != "" {
    query = query.Where("username LIKE ?", orm.LikeFull(input.Username))
}
if input.Status != "" {
    query = query.Where("status = ?", input.Status)
}
var rows []userModel
total, err := r.CountPage(ctx, query, input.Page, input.PageSize, "id DESC", &rows)

// 写入
err = r.BaseRepository.Create(ctx, model)

// 更新（使用基类）
err = r.UpdateFieldByID(ctx, id, &userModel{}, "status", newStatus)

// 多字段更新
err = tx.Exec(ctx, r.DB).Model(&userModel{}).
    Where("id = ?", id).
    Updates(map[string]any{"nickname": displayName}).Error

// 软删除
err = r.SoftDeleteByID(ctx, id, &userModel{})

// 硬删除
err = r.DeleteByID(ctx, id, &userModel{})
```

##### LIKE 与辅助方法

```go
// LIKE 值转义（列名固定，值走参数化）
query.Where("username LIKE ?", orm.LikeFull(val))  // %value%
query.Where("code LIKE ?", orm.LikeLeft(val))      // %value
query.Where("name LIKE ?", orm.LikeRight(val))     // value%

// 其他常见条件
query.Where("status = ?", status)
query.Where("id IN ?", ids)
query.Where("age BETWEEN ? AND ?", min, max)
```

辅助方法位于 `gitee.com/hq_project/yomi-boot/component/common/db/orm`：`like.go` 提供 LIKE 模式（`LikeLeft`/`LikeRight`/`LikeFull`），`page.go` 提供分页（`ApplyPage`）；事务感知执行器 `Exec` 位于 `gitee.com/hq_project/yomi-boot/component/common/db/tx`。

#### 3.2.3 原生 SQL 回退

复杂查询使用 `gorm.DB.Raw()` 执行原生 SQL，SQL 文本定义为 repository 内私有常量，参数通过占位符传入：

```go
const complexReportSQL = `
SELECT r.id, r.code, COUNT(o.id) AS order_count
FROM t_{resource} r
LEFT JOIN t_{other} o ON o.resource_id = r.id
WHERE r.status = ? AND r.create_time >= ?
GROUP BY r.id, r.code
`

var results []ReportDTO
err := tx.Exec(ctx, r.DB).Raw(complexReportSQL, status, since).Scan(&results).Error
```

### 3.3 Entity 与 Model 映射规则

Entity（domain/contract）与 Model（repo/db，包内私有）分离；Mapper 收口 Entity ↔ Model 转换；repository 对外只返回 Entity。架构边界见 [Go业务分层规范](./Go业务分层规范.md) §3.2、§3.4；Entity 默认贫血、按需充血见同规范 §3.1.2。

#### 3.3.1 Entity 定义（domain/contract 层）

```go
// 示例：internal/biz/domain/contract/{domain}/{resource}_entity.go
package domain

import "{client}/{domain}/enums"

// Resource 表示领域实体（示例）
type Resource struct {
    ID          int64                 // ID 是主键
    Username    string                // Username 是登录名（示例字段）
    Nickname    string                // Nickname 是展示名称
    Email       string                // Email 是邮箱地址
    Status      enums.ResourceStatus  // Status 是当前状态
    CreateTime  int64                 // CreateTime 是创建时间，Unix 时间戳（毫秒）
    UpdateTime  int64                 // UpdateTime 是更新时间，Unix 时间戳（毫秒）
}
```

Entity 规则：

- 不含 GORM tag、不含 `json` tag（除非确有内部序列化需求）。
- 时间字段使用 `int64` 毫秒时间戳，与 HTTP 接口口径一致。
- 枚举字段使用 `{client}/{domain}/enums`（**领域 client**）中的枚举类型（平台级枚举如 `DbType` 在 `gitee.com/hq_project/yomi-boot/component/core/shared/enums`）。

#### 3.3.2 Model 定义（repo/db 层）

Go Model 与数据库表一一对应，使用 GORM tag 映射列名：

```go
// 示例：internal/biz/repo/db/{domain}/{resource}_model.go
package repo

import (
    "gitee.com/hq_project/yomi-boot/component/common/db/orm"
    "{client}/{domain}/enums"
)

const tableName = "t_resource"

type resourceModel struct {
    Username    string                `gorm:"column:username"`
    Password    string                `gorm:"column:password"`
    Nickname    string                `gorm:"column:nickname"`
    Email       *string               `gorm:"column:email"`
    Status      enums.ResourceStatus  `gorm:"column:status"`
    orm.BaseModel
}

func (userModel) TableName() string {
    return tableName
}
```

Model 映射规则：

| 规则 | 说明 |
| ---- | ---- |
| 可见性 | Model 结构体通常包内私有（小写开头，如 `userModel`），不暴露给 service 上层以外的包 |
| 表名 | 在包级声明表名常量（如 **示例** `const tableName = "t_{resource}"`），`TableName()` 方法返回该常量；同一文件多个 Model 时使用具名常量 |
| 列映射 | 使用 `gorm:"column:column_name"` tag，列名与数据库表字段一致 |
| 可空字段 | 使用 `*T` 指针类型（如 `*string`），对应表的 `NULL` 列 |
| 非空字段 | 使用 `T` 值类型（如 `string`、`time.Time`），对应表的 `NOT NULL` 列 |
| 枚举字段 | 使用 `enums.XxxStatus` 等自定义 string 类型 |
| 审计字段 | 通过内嵌 `orm.BaseModel` 复用 |
| Go 字段名与列名 | 允许不一致（如 `Nickname` → `gorm:"column:nickname"`） |
| 软删除 | 通过 `orm.BaseModel` 内嵌的 `DeleteTime gorm.DeletedAt` 实现 |

#### 3.3.3 Mapper 定义

```go
// internal/biz/repo/db/user/user_mapper.go
package repo

import (
    "gitee.com/hq_project/yomi-boot/component/core/utils/ptrutil"

    domainuser "{domain-service}/internal/biz/domain/contract/user" // 示例：业务工程 module 路径
)

func toUserEntity(model *userModel) *domainuser.User {
    if model == nil {
        return nil
    }
    return &domainuser.User{
        ID:          model.ID,
        Username:    model.Username,
        Nickname:    model.Nickname,
        Email:       ptrutil.StringFromPtr(model.Email),
        Status:      model.Status,
        CreateTime:  model.CreateTime.UnixMilli(),
        UpdateTime:  model.UpdateTime.UnixMilli(),
    }
}
```

Mapper 规则：

- 所有 Entity ↔ Model 转换统一收口在 `{module}_mapper.go`，不散落在 repository 方法中。
- repository 对外返回 Entity，对内操作 Model。
- 时间字段在 mapper 中完成 `time.Time` ↔ `int64` 毫秒时间戳转换。

#### 3.3.4 基础 Model（BaseModel）

项目使用自定义 `BaseModel` 替代 `gorm.Model`，以支持业务审计字段。`BaseModel` 定义在 `gitee.com/hq_project/yomi-boot/component/common/db/orm/base_model.go` 中：

```go
type BaseModel struct {
    ID         int64          `gorm:"column:id;primaryKey"`
    CreateTime time.Time      `gorm:"column:create_time;autoCreateTime"`
    CreateBy   *int64         `gorm:"column:create_by"`
    UpdateTime time.Time      `gorm:"column:update_time;autoUpdateTime"`
    UpdateBy   *int64         `gorm:"column:update_by"`
    DeleteTime gorm.DeletedAt `gorm:"column:delete_time;index"`
    DeleteBy   *int64         `gorm:"column:delete_by"`
}
```

业务 Model 通过内嵌组合使用（定义在 `internal/biz/repo/db/{module}/{module}_model.go` 中）：

```go
type userModel struct {
    Username    string           `gorm:"column:username"`
    Password    string           `gorm:"column:password"`
    Nickname    string           `gorm:"column:nickname"`
    Email       *string          `gorm:"column:email"`
    Status      enums.UserStatus `gorm:"column:status"`
    orm.BaseModel
}
```

规则：

- `BaseModel` 放置在 `gitee.com/hq_project/yomi-boot/component/common/db/orm/` 中，作为平台基础设施。
- 业务 Model 通过内嵌 `orm.BaseModel` 自动获得 ID、审计字段和软删除能力。
- Entity 中的时间字段为 `int64` 毫秒时间戳，Model 中为 `time.Time`，由 mapper 转换。

#### 3.3.5 Model 与表设计的对齐要求

Model 定义必须与数据库表设计规范保持一致：

- 字段名和类型必须与表定义中的列一一对应。
- 业务枚举字段在 Go 中使用自定义 `string` 类型（如 `enums.UserStatus`），与表中 `varchar` 列对应，枚举值使用全大写（如 `"ENABLED"`、`"DISABLED"`）。
- 金额字段在 Go 中统一使用 `decimal.Decimal`，口径见 [Go代码规范](./Go代码规范.md) §3.6；MySQL 侧使用 `DECIMAL(precision, scale)`。
- 主键字段统一为 `int64` 类型，由 `snowflake` ID 生成器赋值，非数据库自增。
- `TableName()` 方法返回的表名常量必须与数据库表名一致。

#### 3.3.6 复杂原生 SQL 管理

复杂原生 SQL 不单独建 `sqlfiles` 包或根目录 `sql/` 查询目录；在对应 `repo/db/{module}/` 文件内以私有常量维护 SQL 文本，通过 `tx.Exec(ctx, r.DB).Raw(...)` 执行，并遵守 §3.2.3 占位符传参规则。

### 3.4 Repository 层职责与示例

#### 3.4.1 职责边界

- repository 只负责数据读写；入参/返回值用 domain/contract 的 Entity 和 DTO，不暴露 Model。
- 事务由 usecase 控制；repository 通过 `tx.Exec(ctx, r.DB)` 感知事务，方法签名不含 `*gorm.DB`。

#### 3.4.2 Repository 定义与实现

Repository 嵌入 `orm.BaseRepository` 基类，实现 repo/contract 中的 `{Module}Repository` 接口：

```go
package repo

import (
    "context"
    "gorm.io/gorm"
    domainuser "{domain-service}/internal/biz/domain/contract/user" // 示例：业务工程 module 路径
    userrepocontract "{domain-service}/internal/biz/repo/contract/user" // 示例
    "gitee.com/hq_project/yomi-boot/component/common/db/orm"
)

type Repository struct {
    orm.BaseRepository
}

func NewRepository(d *gorm.DB) userrepocontract.UserRepository {
    return &Repository{BaseRepository: orm.BaseRepository{DB: d}}
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*domainuser.User, error) {
    model, err := r.findModelByID(ctx, id)
    if err != nil {
        return nil, err
    }
    return toUserEntity(model), nil
}

func (r *Repository) Create(ctx context.Context, input *domainuser.CreatePersistInput) (*domainuser.User, error) {
    model := toUserModel(input)
    if err := r.BaseRepository.Create(ctx, model); err != nil {
        return nil, err
    }
    return toUserEntity(model), nil
}

func (r *Repository) UpdateStatus(ctx context.Context, input *domainuser.UpdateStatusInput) error {
    return r.UpdateFieldByID(ctx, input.ID, &userModel{}, "status", input.Status)
}
```

> **调用层规范**：
>
> | 操作 | 方法 | 说明 |
> | ---- | ---- | ---- |
> | 新增 | `Create(ctx, model)` | 单条插入 |
> | 批量新增 | `CreateInBatches(ctx, models, batchSize)` | 分批插入 |
> | 按 id 查单行 | `FirstByID(ctx, id, &dest)` | 基类返回 `(found, err)`，repo 映射为领域错误 |
> | 按字段查单行 | `FirstByField(ctx, field, value, &dest)` | 同上 |
> | 查全部 | `FindAll(ctx, model, &dest)` | 全表查询 |
> | 批量查 | `FindByIDs(ctx, ids, model, &dest)` | `WHERE id IN (?)` |
> | 计数 | `ExistsByField(ctx, field, value, &model{})` | `COUNT(*)>0` |
> | 全量更新 upsert | `UpdateAll(ctx, model)` | 按主键匹配，存在则更新全部字段，不存在则插入 |
> | 批量全量更新 upsert | `UpdateAllBatch(ctx, models)` | 逐条 upsert |
> | 按 id 更新多字段 | `UpdateByID(ctx, id, &model{}, updates)` | `updates` 为 `map[string]any` |
> | 按 id 更新单字段 | `UpdateFieldByID(ctx, id, &model{}, field, value)` | 简化单字段更新 |
> | 批量更新 | `UpdateBatch(ctx, ids, model, updates)` | `WHERE id IN (?)` |
> | 硬删除 | `DeleteByID(ctx, id, &model{})` | `Unscoped` 模式 |
> | 软删除 | `SoftDeleteByID(ctx, id, &model{})` | 委托基类 |
> | 批量硬删除 | `DeleteBatch(ctx, ids, model)` | `Unscoped` + `IN` |
> | 批量软删除 | `SoftDeleteBatch(ctx, ids, model)` | `Model` + `IN` |
> | 分页 | `CountPage(ctx, query, page, pageSize, order, &rows)` | 内部调用 `ApplyPage` |
> | 原生 SQL | `tx.Exec(ctx, r.DB).Raw(sql, args...).Scan(&dest)` | 仅用于复杂查询 |
>
> 对于复合条件、多表 JOIN、`gorm.Expr` 表达式等场景，直接使用原生 GORM 链式写法。

### 3.5 事务控制

事务控制规范统一收口到 [Go事务规范](./Go事务规范.md)，本文仅列出与 repository 层相关的核心约定：

- Repository 嵌入 `orm.BaseRepository`，通过 `tx.Exec(ctx, r.DB)` 自动感知事务上下文——在事务闭包内自动使用事务连接，事务外使用普通连接。
- Repository 不自行管理事务，事务边界由 usecase 通过 `tx.Runner.Run` 控制。
- Repository 方法签名中**不出现 `*gorm.DB` 参数**，事务通过 Context 隐式传递。

详细的事务管理器实现、usecase 中使用事务的示例以及分布式事务方案，请直接参考 [Go事务规范](./Go事务规范.md)。

---

## 4. 检查清单

对照 §3.3–§3.4 与 [Go业务分层规范](./Go业务分层规范.md) §4；重点确认 Entity/Model 分离、Mapper 收口、`tx.Exec` 事务感知、构造器返回 repo/contract 接口。

## 5. 禁止项

- 禁止 Entity 含 GORM tag，禁止 Model 暴露给 usecase/handler。
- 禁止 repository 自行管理事务、向上暴露 `*gorm.DB`、直接返回 `gorm.ErrRecordNotFound`。
- 禁止 repository 将用户输入或变量动态拼接到 SQL 片段中；固定列名的 GORM 占位符写法（如 `"status = ?"`）或原生 SQL 文件不受此限。
- 禁止 Entity 与 Model 混写在同一文件。
