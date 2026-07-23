---
description:代码巡检
---

1. 先阅读 `_prompts/rules/资源根目录规则.md`，再基于 `_prompts/agent/ipc-agent.md` 开始生成代码巡检文档。

2. 巡检报告必须落盘到**当前业务工程**根目录 `_ipc-doc/`，文件名遵循 `{YYYYMMDD}_{需求名称}-ipc.md`（与 audit 按应开发文档关联命名的规则一致：audit 为 `_feat-doc/{YYYYMMDD}_{需求名称}/audit.md`，ipc 为 `_ipc-doc/{YYYYMMDD}_{需求名称}-ipc.md`）。未关联应开发文档时使用 `_ipc-doc/{YYYYMMDD}_全项目-ipc.md`。

3. 基于第一性原理和对抗性审查进行思考，每个包给出评价，评价标准1到5星，丟星的原因要给出原因


审查标准：

1. 不考虑测试覆盖率

2. 重点关注：系统崩溃（panic/NPE）、业务阻塞（资源泄漏/无限阻塞/goroutine泄漏）、并发死锁（data race/死锁）、业务正确性（数据损坏/违反契约）

3. 架构问题以建议形式提供，不计入丢星

4. 不考虑过于特殊场景的 bug

5. 不考虑实际不会出现场景的 bug

6. 结合项目实际应用场景判断 bug 是否会触发

把结果输出成完整列表给我。
