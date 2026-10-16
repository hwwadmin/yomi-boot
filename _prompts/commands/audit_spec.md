---
description:结构化需求评审文档生成
---

1. **定位 `_prompts`（强制）**：按顺序尝试：① 当前项目根 `_prompts/`；② 环境变量 `YOMI_PROMPTS_ROOT`（`yomi-boot` 仓库根，或该根下含 `_prompts` 的父路径）；③ Git submodule / 同级克隆的 `yomi-boot/_prompts/`；④ 依赖库 `yomi-boot`（`gitee.com/hq_project/yomi-boot`，经 go.mod / `replace` / module cache 解析到仓库根）包内的 `_prompts/`。均未找到则**终止**并列出已尝试路径。定位成功后，`_prompts/skills`、`knowledge` 均相对该根；业务产物（`_doc/`、`_feat-doc/`、`_task-doc/`、`_quality-doc/`、`_ipc-doc/`）写入**当前业务工程根**（boot 库维护模式除外）。

2. **思维与口径（全程适用；分阶段执行 Skill 时也不得仅在落盘前补写；含义同 `deep_think` command）**：

   - **第一性原理**：从问题本质出发：目标、约束、因果关系；避免套用未经检验的惯例或隐含假设。
   - **对抗性审查**：主动质疑结论：反例、边界、遗漏、可行替代方案；信息不足须标**待确认**，不得臆造为已确认事实。
   - **PRD 与 knowledge 边界**：参考当前业务工程 `_doc/` 中的 PRD（若有），核对 spec 业务口径；冲突记入 audit 发现，不得静默改写 PRD；`_prompts/knowledge/` 为技术规范，**不得当作产品 PRD**（见 `prd_spec`）。
   - **分阶段小结**：每完成一个阶段，用 3–5 条记录：**本阶段假设**、**反例/边界**、**待确认项**（写入 audit §0/§2，不得只留在对话里）。
   - **禁止**：为填模板而编造规模事实（读写峰值、方法强度、表数据量）；跳过 Skill §6（DoD）仍落盘 audit。

3. 按下列顺序执行《显化审核》（`_prompts/skills/显化审核.md`）：

   - Read Skill §0–§2、§3.1 流程对照表。
   - **阶段 0（按需）**：按 Skill §3.1.1 做能力画像；命中适用项时**必须**调用原生工具 `AskQuestion` 采集，执行至用户确认；无适用项时记录「全部跳过」后进入阶段 1。
   - **阶段 1–3**：§3.2–§3.4（含 §3.3 事务强制检查清单）；按需 Read 各 Go knowledge。
   - **写 audit**：按 Skill **§4.1 章纲**，产出 `_feat-doc/{YYYYMMDD}_{需求名称}/audit.md`。

4. 落盘前 Read Skill §6（DoD）并逐项勾选；通过后定稿 audit，并在结论中明确「是否存在阻塞问题」与「修正后是否可进入实现阶段」。
