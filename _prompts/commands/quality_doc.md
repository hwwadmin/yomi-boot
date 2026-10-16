---
description:生成测试用例与产品验收文档（可选）
---

1. **定位 `_prompts`（强制）**：按顺序尝试：① 当前项目根 `_prompts/`；② 环境变量 `YOMI_PROMPTS_ROOT`（`yomi-boot` 仓库根，或该根下含 `_prompts` 的父路径）；③ Git submodule / 同级克隆的 `yomi-boot/_prompts/`；④ 依赖库 `yomi-boot`（`gitee.com/hq_project/yomi-boot`，经 go.mod / `replace` / module cache 解析到仓库根）包内的 `_prompts/`。均未找到则**终止**并列出已尝试路径。定位成功后，`_prompts/skills`、`knowledge` 均相对该根；业务产物（`_doc/`、`_feat-doc/`、`_task-doc/`、`_quality-doc/`、`_ipc-doc/`）写入**当前业务工程根**（boot 库维护模式除外）。

2. 再**全文执行** `_prompts/skills/质量文档生成.md`（《质量文档生成》）。

3. **建议**：对应需求的 `audit.md` 无未闭合严重项后再生成；若用户明确要求跳过审核，须在会话中说明。

4. 阅读用户指定的需求显化文档（当前业务工程路径，如 `_feat-doc/{YYYYMMDD}_{需求名称}/spec.md`）；若提供或可定位产品需求文档，一并阅读 `_doc/*-prd.md`。

5. 生产过程需要基于第一性原理和对抗性审查进行思考；仅从上游已确认口径派生，不得编造需求点。

6. 按《质量文档生成》Skill 同轮产出两份文档到**当前业务工程**根目录：

   - `_quality-doc/{YYYYMMDD}_{需求名称}/test.md`
   - `_quality-doc/{YYYYMMDD}_{需求名称}/accept.md`

7. 本指令为可选质量侧产物，与 `check_code` 同级：不由 `do_spec` 触发，不以巡检结果为前置。
