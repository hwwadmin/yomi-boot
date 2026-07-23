# Go代码巡检

---

## 0. 基础信息
- **Skill 名称**: Go代码巡检
- **描述**: 对基于 gitee.com/hq_project/yomi-boot 的 Go 后端（本仓为基础库；业务仓为可部署单元）执行分层架构、BFF 编排、数据访问、DTM 事务、令牌鉴权、注释规范等多维巡检；在 **gitee.com/hq_project/yomi-boot 本仓**执行时以 [Go基础库检查规范](../knowledge/go/Go基础库检查规范.md) 为主
- **作者**: zwei
- **创建日期**: 2026-06-02


---

## 1. 目标

- **做什么**：对指定 Go 代码范围执行多维度巡检——分层架构（handler / usecase / contract / service / repository）、Entity/Model 分离、数据访问、事务控制、令牌鉴权、事件机制、注释规范、Redis 设计、数据库表设计，输出问题清单或在用户授权下直接修复。
- **为什么**：将 Go 后端治理动作统一为单一入口，确保分层边界清晰、数据访问规范、事务可控、鉴权安全、缓存和数据库设计合理，减少 Code Review 返工和线上风险。
- **不做什么**：不涉及前端代码、不涉及性能压测或安全渗透测试。

---

## 2. 输入/依赖

### 2.0 调用方式与输入

调用本 Skill 时，需至少提供目标目录或文件范围，并说明巡检维度（可指定分层架构 / 数据访问 / 事务 / 令牌 / 事件 / 注释，不指定则默认全维度巡检）以及是否允许直接修复。

> 示例调用：「请使用 Go代码巡检 Skill，巡检 `internal/biz/` 目录，重点关注事务和分层架构，直接修复明显问题。」

### 2.1 前置 Skill 依赖

| 名称 | 状态 | 说明 |
| ---- | ---- | ---- |
| — | — | 无前置依赖 |

### 2.2 知识文档依赖

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| `_prompts/doc/基础库能力索引.md` | 全文 | gitee.com/hq_project/yomi-boot 各能力库速查；判断是否应复用基础库而非平行实现 |
| `_prompts/doc/knowledge/go/Go业务分层规范.md` | §3 | handler / usecase / contract / service / repository 分层、跨模块协作、轻量 DDD-lite 与贫血/充血 Entity 建模 |
| `_prompts/doc/knowledge/api/微服务调用规范.md` | §3 | POST/JSON、Header 链路 ID、bizId/bizSubId、idempotency |
| `_prompts/doc/knowledge/go/Go架构规范.md` | §3 | gitee.com/hq_project/yomi-boot、services、Workspace、bootstrap、Consul、DTM |
| `_prompts/doc/knowledge/go/Go数据库规范.md` | §3 | GORM 链式查询、Model 映射规则、事务感知 repository |
| `_prompts/doc/knowledge/go/Go事务规范.md` | §3 | 本地子事务、DTM Saga、Execute/Compensate |
| `_prompts/doc/knowledge/go/Go注释规范.md` | §3 | 文件头注释、类型与字段注释、函数注释、常量枚举注释 |
| `_prompts/doc/knowledge/go/Go选型规范.md` | §3 | 技术栈基线、依赖选择口径 |
| `_prompts/doc/knowledge/redis/Redis设计规范.md` | §3 | Key 命名、TTL、数据结构、缓存策略、分布式锁与敏感数据约束 |
| `_prompts/doc/knowledge/db/数据库表设计规范.md` | §3 | 表命名、主键设计、审计字段、字段类型、索引设计 |
| `_prompts/doc/knowledge/api/API编码适配规范.md` | §3 | Encoder 适配与显式注册；不规定具体 JSON 形状 |
| `_prompts/doc/knowledge/go/Go代码规范.md` | §3 | 校验分层、handler 辅助、字段规范；HTTP 方法摘要见 §3.3（权威分属架构/微服务规范） |

### 2.3 数据表依赖

| 表名 | DDL 来源 | 说明 |
| ---- | -------- | ---- |
| — | — | 无数据库依赖 |

---

## 3. 技术方案

### 3.1 巡检维度

各维度检查项见 §3.3–§3.9；规范原文以对应知识文档为准。

| 维度 | 核心规范来源 |
| ---- | ------------ |
| 分层架构 | Go业务分层规范、Go架构规范、GoBFF规范、Go代码规范 |
| BFF 编排 | GoBFF规范 |
| 数据访问 | Go数据库规范 |
| 事务控制 | Go事务规范 |
| 注释规范 | Go注释规范 |
| 接口协议 | Go架构规范、微服务调用规范、API编码适配规范、Go代码规范 |
| Redis 设计 | Redis设计规范 |
| 数据库表设计 | 数据库表设计规范 |

### 3.2 执行步骤

```text
开始
  ↓
1. 确认巡检范围与巡检维度
  ↓
2. 读取对应知识文档中的规范基线
  ↓
3. 按维度逐一执行检查（详见 §3.3–§3.9）：
   - 分层架构：扫描 `internal/biz/` 目录与依赖方向、bootstrap 装配与路由注册链
   - 数据访问 / 事务 / 令牌 / 事件 / 注释 / Redis / 表设计：对照对应 § 检查项
  ↓
4. 汇总问题清单，标注优先级与规范依据
  ↓
5. 输出问题清单，或在用户授权下直接修复
  ↓
6. 修复后重新执行编译或 golangci-lint 确认结果
  ↓
结束
```

### 3.3 分层架构巡检

| 检查项 | 检查内容 | 依据 |
| ------ | -------- | ---- |
| 依赖引用 | 业务单元是否标准 `require gitee.com/hq_project/yomi-boot`（或本地 `go.work` / `replace`）；`gitee.com/hq_project/yomi-boot` 不依赖业务代码 | Go架构规范 §3.5 |
| bootstrap | 各服务 `internal/bootstrap/` 以 `server.go` 为入口；Kafka 消费者用 `*_consumer_plugin.go` + `AddCustomPlugin`；无 `wire/` 子目录 | Go架构规范 §3.4 |
| BFF 目录 | 是否含 `internal/handler/`、`bff/`、`saga/`、`bizerr/`；无领域库访问/领域业务表；本服务库仅基础设施用途 | GoBFF规范 §3.1 |
| 领域目录 | `biz/handler`、`usecase` 平铺；含 `api_types.go` 别名 client DTO；无 token 中间件 | Go业务分层规范 §3.4 |
| 领域零互调 | 领域服务是否无 HTTP 调其他领域 | Go架构规范 §3.1.3 |
| 微服务契约 | 符合 [微服务调用规范](../api/微服务调用规范.md) §3 | 微服务调用规范 |
| 业务幂等 | 写 usecase 使用 `gitee.com/hq_project/yomi-boot/idempotency.Execute` | 微服务调用规范 §3.4 |
| BFF 喂饱 | 各接口 DTO 按需传参；领域不读 token | GoBFF规范 §3.2.3 |
| handler | 协议适配是否经 `gitee.com/hq_project/yomi-boot/handler`；领域写用 `dtm.BindAndHandleAction/Compensate` | Go代码规范 §3.3.1 |
| usecase | 平铺命名；Saga 写含 `Compensate`；无跨域 HTTP | Go业务分层规范 §3.4.2 |
| Saga Handler | Action/Compensate 是否独立入口，路由 `/compensate` 成对 | Go事务规范 §3.4.2 |
| gitee.com/hq_project/yomi-boot | 是否无业务依赖；基础设施经 `plugin` + 各库 `{lib}/plugin` 生命周期管理；`event/` 为可选组件 | Go架构规范 §3.3、§3.4.4 |
| 令牌 | BFF 经 `infra.TokenService`（gitee.com/hq_project/yomi-boot/token）注入；config 使用 `auth` 段 | GoBFF规范 §3.3 |
| depguard | 分层依赖方向是否符合规范 | Go业务分层规范 §3.8 |

### 3.4 数据访问巡检

| 检查项 | 检查内容 | 依据 |
| ------ | -------- | ---- |
| GORM 查询 | 常规 CRUD 用链式 API；复杂查询用 `Raw` + repository 内 SQL 常量；禁止字符串拼接 SQL | Go数据库规范 §3.2 |
| Model 映射 | 是否使用 `gorm:"column:xxx"` tag 映射列名 + 包级表名常量 + `TableName()` 返回该常量 | Go数据库规范 §3.3 |
| 审计字段 | Model 是否包含 `ID`/`CreateTime`/`CreateBy`/`UpdateTime`/`UpdateBy`/`DeleteTime`/`DeleteBy` | Go数据库规范 §3.3.2 |
| 事务感知 | repository 是否通过 `db.Exec(ctx, r.DB)`（内部 `db.GetTx`）感知事务环境 | Go数据库规范 §3.4 |

### 3.5 事务控制巡检

| 检查项 | 检查内容 | 依据 |
| ------ | -------- | ---- |
| 事务入口 | 事务是否在 usecase 层通过 `db.TxExecutor.Execute`（生产实现基于 `gorm.DB.Transaction`）开启 | Go事务规范 §3.2 |
| 事务边界 | 一个事务是否只包含真正需要原子性的操作 | Go事务规范 §3.3 |
| 事务传递 | repository 是否通过 Context 的 `db.GetTx` 自动感知事务 | Go事务规范 §3.5 |
| 分布式事务 | 跨域写是否由 BFF `saga/` 提交 DTM；领域是否 Execute/Compensate 同文件 | Go事务规范 §3.1、§3.4 |

### 3.6 注释规范巡检

| 检查项 | 检查内容 | 依据 |
| ------ | -------- | ---- |
| 文件头注释 | 每个包是否至少一个文件包含 `// Package xxx ...` 注释 | Go注释规范 §3.2 |
| 类型注释 | 所有导出类型定义是否有注释 | Go注释规范 §3.3 |
| 函数注释 | 所有函数（含非导出）是否有注释，格式是否符合 `// 函数名 描述` | Go注释规范 §3.4 |
| 常量注释 | 同组常量是否有块前注释，不必要的不逐行注释 | Go注释规范 §3.5 |
| 关键步骤 | 方法体内关键业务步骤是否有注释 | Go注释规范 §3.6 |

### 3.8 Redis 设计巡检

| 检查项 | 检查内容 | 依据 |
| ------ | -------- | ---- |
| Key 命名 | 是否遵循 `{项目名}:{模块名}:{业务名}:{标识}` 冒号分层格式 | Redis设计规范 §3.2 |
| TTL 设置 | 是否根据业务特性判断是否需要 TTL，设置的值是否合理 | Redis设计规范 §3.4 |
| 数据结构 | String/Hash/List/Set/ZSet 选择是否匹配业务场景 | Redis设计规范 §3.3 |
| 缓存策略 | 回源、失效、刷新策略是否清晰且一致 | Redis设计规范 §3.4 |
| 分布式锁 | 是否仅在需要跨实例互斥时使用，锁是否设置超时 | Redis设计规范 §3.5 |
| 敏感数据 | 是否避免明文缓存密码、验证码等高敏感信息 | Redis设计规范 §3.7 |

### 3.9 数据库表设计巡检

| 检查项 | 检查内容 | 依据 |
| ------ | -------- | ---- |
| 表命名 | 表名是否使用小写字母和下划线，是否遵循 `t_` 前缀规范 | 数据库表设计规范 §3.1 |
| 主键设计 | 主键是否使用 `bigint`，是否避免自增 | 数据库表设计规范 §3.2.1 |
| 审计字段 | 是否包含 `create_time`/`create_by`/`update_time`/`update_by`/`delete_time`/`delete_by` | 数据库表设计规范 §3.2.2 |
| 字段类型 | 业务枚举、金额、时间字段选型是否合理 | 数据库表设计规范 §3.2.3 |
| 字段注释 | 注释是否完整且与业务语义一致 | 数据库表设计规范 §3.2.4 |
| 索引设计 | 索引命名、数量、组合方式是否合理，是否存在冗余索引 | 数据库表设计规范 §3.3 |

### 3.10 问题分级

| 级别 | 说明 | 示例 |
| ---- | ---- | ---- |
| 🔴 严重 | 可能导致错误行为或架构失控 | Handler 直接操作 DB、事务边界缺失、Token 回检查询库 |
| 🟡 警告 | 影响可维护性和一致性 | 模型混用、SQL 内嵌代码、注释缺失 |
| 🔵 建议 | 可优化项 | 公共逻辑可提取、注释可补充 |

### 3.11 输出要求

若本次执行为"只检查不修复"，输出必须至少包含：

- 巡检维度与范围
- 问题所在文件及行号
- 问题级别
- 问题描述与对应规范依据
- 修复建议

若本次执行为"允许直接修复"，则必须：

- 优先修复低风险、可机械验证的问题
- 修复后重新执行 `golangci-lint run ./...` 或 `go build ./...`
- 在结论中说明已修复项与剩余风险项

---

## 4. 约束与规范

- [ ] 巡检必须以项目 Go 知识文档为唯一规范依据，禁止给出无来源的主观建议
- [ ] 必须遵守 `_prompts/rules/基础库优先使用规则.md`：对照 `_prompts/doc/基础库能力索引.md`，将可复用 gitee.com/hq_project/yomi-boot 能力却平行实现或重复依赖的代码列为问题
- [ ] 未经用户允许，不扩大到无关目录或无关模块
- [ ] 有可执行校验时，修复后必须重新执行编译或 lint 检查
- [ ] 不得把推测写成确认事实，不确定的问题标注为"待确认"
- [ ] 涉及分层、事务、令牌等架构级问题时，必须先回看对应规范再定性

---

## 5. 验收标准（DoD）

### 5.1 仅检查场景

- [ ] 已明确巡检维度、范围和规范依据
- [ ] 已输出问题清单，包含级别、描述、依据和修复建议
- [ ] 已说明是否存在需要人工判断的边界问题

### 5.2 直接修复场景

- [ ] 已修复项均有对应规范依据
- [ ] `golangci-lint run ./...` 或 `go build ./...` 已重新执行并通过
- [ ] 未引入新的编译错误、分层混乱或无关改动
