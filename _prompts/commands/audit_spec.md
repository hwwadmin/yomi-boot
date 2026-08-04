---
description:结构化需求评审文档生成
---

1. **定位 `_prompts`（强制，不可跳过）**：先查当前项目根目录下的 `_prompts/`；若不存在，再查当前项目依赖的外部库 `yomi-boot`（`gitee.com/hq_project/yomi-boot`）包下的 `_prompts/`。两处均查不到则**终止执行**并说明原因。定位成功后阅读 `_prompts/rules/资源根目录规则.md`，再基于 `_prompts/agent/requirements-audit-agent.md` 开始生成审核文档。

2. 生产过程需要基于第一性原理和对抗性审查进行思考。

3. 思考过程需要参考当前业务工程 `_doc/` 中的产品需求文档（若有）。
