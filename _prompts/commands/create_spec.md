---
description: 需求显化文档生成
---

1. **思维与口径（全程适用；分阶段执行 Skill 时也不得仅在落盘前补写；含义同 `deep_think` command）**：

   - **第一性原理**：从问题本质出发：目标、约束、因果关系；避免套用未经检验的惯例或隐含假设。
   - **对抗性审查**：主动质疑结论：反例、边界、遗漏、可行替代方案；信息不足须标**待确认**，不得臆造为已确认事实。
   - **PRD 与 knowledge 边界**：**产品口径**以工程根 `_doc/` 中的 PRD（若有）为准，须参考并对齐，冲突记入 spec §3，不得静默改写 PRD；`_prompts/knowledge/` 为 Go/架构/事务等技术规范，**不得当作产品 PRD**（见 `prd_spec`）。
   - **分阶段小结**：每完成一个阶段，用 3–5 条记录：**本阶段假设**、**反例/边界**、**待确认项**（写入 spec §3，不得只留在对话里）。
   - **禁止**：为填模板而编造业务结论、表量、QPS、并发峰值；跳过 Skill §6（DoD）仍落盘 spec。

2. 按下列顺序执行《需求显化》（`_prompts/skills/需求显化.md`），**不得跳过**所列 `knowledge` 必读：

   - **阶段 A（spec 章 0–7）**：Read Skill §0–§2、§3.0–§3.2 及关联 knowledge（`knowledge/数据库表设计规范.md` 等）；产出对应 spec 章节。
   - **阶段 B（spec 章 8–11）**：Read Skill §3.3–§3.4.2、`knowledge/Go事务规范.md`、`knowledge/Go数据库规范.md`；**同轮**产出 Ch10 与 Ch11 并对照事务列与 Step 1。
   - **阶段 C（spec 章 12–17）**：Read Skill §3.5–§3.8、Skill **§4.1 章纲**；产出剩余章节。

3. 落盘前 Read Skill §6（DoD）并逐项勾选；通过后写入 `_feat-doc/{YYYYMMDD}_{需求名称}/spec.md`。
