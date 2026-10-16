---
description:代码巡检
---

1. **定位 `_prompts`（强制）**：按顺序尝试：① 当前项目根 `_prompts/`；② 环境变量 `YOMI_PROMPTS_ROOT`（`yomi-boot` 仓库根，或该根下含 `_prompts` 的父路径）；③ Git submodule / 同级克隆的 `yomi-boot/_prompts/`；④ 依赖库 `yomi-boot`（`gitee.com/hq_project/yomi-boot`，经 go.mod / `replace` / module cache 解析到仓库根）包内的 `_prompts/`。均未找到则**终止**并列出已尝试路径。定位成功后，`_prompts/skills`、`knowledge` 均相对该根；业务产物（`_doc/`、`_feat-doc/`、`_task-doc/`、`_quality-doc/`、`_ipc-doc/`）写入**当前业务工程根**（boot 库维护模式除外）。

2. 再**全文执行** `_prompts/skills/代码巡检.md`（《代码巡检》）。

3. 巡检报告必须落盘到**当前业务工程**根目录 `_ipc-doc/`，文件名遵循 `{YYYYMMDD}_{需求名称}-ipc.md`（与 audit 按应开发文档关联命名的规则一致：audit 为 `_feat-doc/{YYYYMMDD}_{需求名称}/audit.md`，ipc 为 `_ipc-doc/{YYYYMMDD}_{需求名称}-ipc.md`）。未关联应开发文档时使用 `_ipc-doc/{YYYYMMDD}_全项目-ipc.md`。

4. 基于第一性原理和对抗性审查进行思考，每个包给出评价，评价标准 1 到 5 星，丢星的原因要给出原因。

审查标准：

1. 不考虑测试覆盖率

2. 重点关注：系统崩溃（panic/未捕获异常）、业务阻塞（资源泄漏/无限阻塞/goroutine 泄漏）、并发死锁（data race/死锁）、业务正确性（数据损坏/违反契约）

3. 架构问题以建议形式提供，不计入丢星

4. 不考虑过于特殊场景的 bug

5. 不考虑实际不会出现场景的 bug

6. 结合项目实际应用场景判断 bug 是否会触发

把结果输出成完整列表给我。
