# spec落地执行

---

## 0. 基础信息

- **Skill 名称**: spec落地执行
- **描述**: 在 `audit.md` 门禁通过后，基于 `spec.md` 拆解 `tasks.md` 并以 TDD 在业务 Go 工程中落地实现
- **作者**: zwei
- **创建日期**: 2026-10-16

---

## 关联知识库

- **执行前必读**：`knowledge/Go业务分层规范.md`、`knowledge/Go代码规范.md`、`knowledge/Go数据库规范.md`、`knowledge/Go事务规范.md`、`knowledge/Go架构规范.md`
- **按需**：`knowledge/GoBFF规范.md`、`knowledge/Go文件命名规范.md`、`knowledge/Go集成测试规范.md`、`knowledge/Go异常规范.md`、`knowledge/微服务调用规范.md`、`knowledge/API编码适配规范.md`、`knowledge/数据库表设计规范.md`、`knowledge/Redis设计规范.md`

## 1. 目标

- **做什么**：阅读 `_feat-doc/.../spec.md`（及可选 `_doc/` PRD、`audit.md`），生成 `_task-doc/.../tasks.md`，按任务 TDD 实现并回写 tasks。
- **为什么**：将已审核的技术 spec 转为可追踪的实现与验证。
- **不做什么**：不修改 spec/audit 正文；不触发 quality_doc / check_code；不写前端实现（前端业务流程由 spec 描述，本工作流落地范围为 Go 后端）。

---

## 2. 规则

1. 涉及基础设施必须先读 `_prompts/knowledge/Go架构规范.md` §3.3（模块职责）与 §3.11（基础库优先使用（强制））：有候选读源码，满足则必须使用对应能力包；无候选或需新增时先按 A/B/C 选择新增方式并获用户确认。
2. 分层遵循 spec「本项目结构约定」；事务与幂等遵循《Go事务规范》，异常与错误码遵循《Go异常规范》。
3. **yomi-boot 根目录**：仅允许基础库与演示改动，禁止实现产品业务域。

---

## 3. 工作流程

- **Step 1**：确认 `audit.md` 无未闭合严重项（尤其事务）；读 spec 全章。
- **Step 2**：拆解 tasks.md（原子任务、依赖、输入输出，按阶段分组）。
- **Step 3**：按任务 TDD：一个功能点对应一组测试方法，先补测试再补实现；`go build ./...` / `go test ./...` 验证；HTTP 集成测试按《Go集成测试规范》与 `gitee.com/hq_project/yomi-boot/component/common/testclient`，参考业务仓 `test/` 平铺目录。
- **Step 4**：每完成一个任务回写 tasks.md 状态与证据。
- **Step 5**：最小范围验证，不扩大改动面。

---

## 4. 约束

- 不得绕过 spec 中的事务与 API 契约（事务落点见 spec 第十一章；HTTP 出口以本服务已注册 Encoder 为准）。
- 不得在业务仓重复实现已有基础库能力（对照《Go架构规范》§3.3 模块职责）。
- 装配惯例：`internal/bootstrap/` 手写装配（`server.go`；Kafka 消费者按需 `*_consumer_plugin.go` + `AddCustomPlugin`）；Handler 经 `gitee.com/hq_project/yomi-boot/component/common/handler` 协议适配、以 `Routes(router)` 注册相对路径。
- 仅修改当前任务相关文件；文件命名遵循《Go文件命名规范》。

---

## 5. 验收标准（DoD）

- [ ] tasks.md 已生成且与 spec 范围一致
- [ ] 实现符合 spec 分层与选型（含 UseCase 编排、领域边界）
- [ ] 写路径事务与幂等落点与 spec 一致（`TxExecutor` / `idempotency.Execute` / Saga Execute-Compensate 成对）
- [ ] 相关测试已执行并通过或说明未执行原因
