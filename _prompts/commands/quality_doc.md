---
description:生成测试用例与产品验收文档（可选）
---

1. 先阅读 `_prompts/rules/资源根目录规则.md`，再基于 `_prompts/agent/quality-agent.md` 开始生成质量文档。

2. 阅读用户指定的需求显化文档（当前业务工程路径，如 `_feat-doc/{YYYYMMDD}_{需求名称}/spec.md`）；若提供或可定位产品需求文档，一并阅读 `_doc/*-prd.md`。

3. 生产过程需要基于第一性原理和对抗性审查进行思考；仅从上游已确认口径派生，不得编造需求点。

4. 按《质量文档生成》Skill 同轮产出两份文档到**当前业务工程**根目录：

   - `_quality-doc/{YYYYMMDD}_{需求名称}/test.md`
   - `_quality-doc/{YYYYMMDD}_{需求名称}/accept.md`

5. 本指令为可选质量侧产物，与 `check_code` 同级：不由 `do_spec` 触发，不以巡检结果为前置。
