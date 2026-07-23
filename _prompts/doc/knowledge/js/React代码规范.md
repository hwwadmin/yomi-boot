# React代码规范

---

## 0. 文档信息
- **文档名称**: React代码规范
- **描述**: 基于 React + TypeScript + Vite 的前端代码质量基线，聚焦静态检查、类型建模、表单约束、接口契约、雪花 ID 字段类型规范与必要的实现细节约束。
- **作者**: zwei
- **创建日期**: 2026-04-08

---

## 1. 文档定位

- **用途**：作为**业务前端工程**的 React 代码质量基线，约束静态检查、类型建模、表单泛型、接口契约和时间字段处理方式。
- **边界**：本文不重复展开模块分层、状态管理、国际化接入和样式落位规则；这些内容分别以关联规范为准。
- **适用对象**：基于 yt-boot 对接的业务前端工程；开发、重构、巡检、代码评审和静态检查场景（**yt-boot 本仓无前端**）。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于**业务前端工程**的 `.ts`、`.tsx`、API 类型文件、表单交互文件和页面实现文件。
- 重点覆盖 ESLint、TypeScript、React Hooks、Ant Design Form、接口字段建模和共享时间工具使用。
- 涉及分层结构、状态设计、国际化边界和样式协同时，需结合关联文档阅读。

### 2.2 关联文档

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| `_prompts/doc/knowledge/js/React分层规范.md` | 分层与目录边界 | 统一 React 模块分层、目录落位与职责边界 |
| `_prompts/doc/knowledge/js/React状态管理规范.md` | 状态设计 | 统一页面级、模块级和应用级状态分层方式 |
| `_prompts/doc/knowledge/js/React国际化规范.md` | 国际化接入边界 | 统一 `useTranslation()`、`translate()`、系统文案和动态重算边界 |
| `_prompts/doc/knowledge/js/React注释规范.md` | 注释风格 | 统一文件头注释、类型注释、方法注释和关键步骤注释写法 |
| `_prompts/doc/knowledge/api/API编码适配规范.md` | 接口契约 | Encoder 适配；前端消费以本服务已注册 Encoder 为准 |

### 2.3 阅读建议

- 新建 React 模块前，先核对本规范的质量基线，再按需查看 React分层规范。
- 遇到表单、时间字段、接口契约问题时，优先回看本规范第 3 节。
- 遇到分层、状态、国际化、注释或样式问题时，直接跳转对应专项规范，不在本文重复展开。
- 代码评审时，优先核对质量检查清单和验收基线两节。

---

## 3. 质量基线

### 3.1 工具准入

| 工具 | 用途 | 配置位置 |
| ---- | ---- | -------- |
| ESLint | 代码规范检查 | `.eslintrc.js` |
| TypeScript | 类型检查 | `tsconfig.json` |
| Prettier | 代码格式化 | `.prettierrc` |

### 3.2 核心规则

| 类别 | 规则 | 要求 |
| ---- | ---- | ---- |
| 静态检查 | ESLint 与 TypeScript | 必须保持无阻断错误；`strict: true` 必须开启 |
| 导入与调试代码 | import、未使用变量、调试日志 | import 顺序保持稳定；禁止遗留未使用 import、变量、函数以及 `console.log`、`console.error` |
| Hooks规则 | Hook 调用顺序与依赖声明 | 必须遵守 Hook 调用顺序；`useEffect`、`useMemo`、`useCallback` 依赖必须完整且稳定 |
| 类型建模 | `any`、闭集字段、状态判断 | 禁止使用 `any`；闭集字段必须使用枚举或联合类型；禁止直接用业务字符串字面量做状态判断 |
| 类型收窄 | 类型断言与固定键映射 | 优先通过类型建模、泛型和守卫消除断言；固定键映射必须使用受限键类型，禁止放宽到 `Record<string, ...>` |
| 表单泛型 | `Form.useForm()`、`validateFields()` | 必须显式传入泛型，确保结果无隐式 `any` |
| 创建分支收窄 | 创建/编辑共用表单 | 调用创建 API 前必须显式组装并收窄必填字段，禁止把含可选字段的 `values` 直接传入严格请求类型 |
| 接口契约 | API 类型定义 | 必须对齐后端最终 HTTP JSON 契约，禁止直接照抄后端内部模型源码类型 |
| 接口错误 | BFF `code` / `msg` | 只允许根据 `code` 映射当前语言的 i18n 文案；`msg` 仅为后端开发调试字段，前端禁止读取、展示或参与业务判断 |
| 时间字段 | 业务时间字段与包装层字段 | 业务 `createTime`/`updateTime` 等一般为 JSON `number`（Unix 毫秒）；包装层字段类型以本服务已注册 Encoder 契约为准（见 [API编码适配规范](../api/API编码适配规范.md)） |
| 时间展示 | 页面展示时间 | 统一使用 `src/utils/datetime.ts` 中的共享格式化函数，禁止散落 `new Date(raw)`、`Number(raw)`、`replace('T', ' ')` |
| 类型导入 | `verbatimModuleSyntax` 场景 | 纯类型导入必须使用 `import type` |
| Transfer 键类型 | `Transfer` 的 `onChange` 返回值 | 若页面状态定义为 `string[]`，必须显式执行 `keys.map(String)` 后再写回状态 |
| 雪花 ID 字段 | id、ids 语义字段的 JSON 类型 | 所有 id 语义字段（**示例** `id`、`{entity}Id`、`relId`）必须使用 `string`；ids 语义字段（**示例** `roleIds`）必须使用 `string[]`；**禁止**使用 `number` 或 `number \| string` |

### 3.3 雪花 ID 字段类型规范

#### 3.3.1 背景

后端使用雪花算法（Snowflake）生成分布式主键，ID 值远超 JavaScript `Number.MAX_SAFE_INTEGER`（2^53），以 JSON 数字接收会丢失精度。后端通过 `JSONInt64` / `JSONInt64Slice` 类型将所有 id/ids 字段序列化为 JSON 字符串，前端必须对齐为 `string` / `string[]`。

#### 3.3.2 类型建模规则

| 场景 | 字段示例 | 必须使用的类型 | 禁止使用的类型 |
|------|----------|---------------|---------------|
| 单个 ID | `id`、`{entity}Id`、`relId`（**示例**） | `string` | `number`、`number \| string` |
| ID 列表 | `roleIds` | `string[]` | `number[]`、`Array<number \| string>` |

#### 3.3.3 API 记录接口

对接后端原始响应的 `XxxApiRecord` 接口中，id 字段类型也必须是 `string`：

```typescript
// 正确：后端 JSONInt64 序列化为字符串，前端对齐
// 作者: zwei
interface RoleApiRecord {
  id: string           // 后端 dto.JSONInt64 → JSON 字符串
  roleCode: string
  roleName: string
}

interface UserApiRecord {
  id: string
  roleIds?: string[]   // 后端 dto.JSONInt64Slice → JSON 字符串数组
}

// 错误：混用 number | string 是多余的防御
// interface RoleApiRecord {
//   id: number | string  // 禁止
// }
```

#### 3.3.4 归一化函数简化

由于后端保证输出 JSON 字符串，前端归一化函数无需再 `map(String)`：

```typescript
// 正确：直接返回，无需 map
// 作者: zwei
function normalizeApiRoleIds(roleIds?: string[]): string[] {
  return roleIds ?? []
}

// 错误：多余的 String 映射
// function normalizeApiRoleIds(roleIds?: ReadonlyArray<number | string>): string[] {
//   return roleIds?.map((roleId) => String(roleId)) ?? []
// }
```

### 3.4 跨规范边界

以下内容不在本文重复展开，统一引用专项规范：

1. React 模块分层、`page.tsx` 入口、Hook/Domain/API 边界、类型与样式收口：遵循 `React分层规范`
2. Store 设计、全局状态与业务状态边界：遵循 `React状态管理规范`
3. 系统文案、语言切换、翻译接入和重算边界：遵循 `React国际化规范`
4. 文件头注释、单行注释、类型注释和关键步骤注释：遵循 `React注释规范`

---

## 4. 质量检查清单

- [ ] ESLint 与 TypeScript 检查通过，且 `strict: true` 已开启
- [ ] 无未使用的 import、变量、函数和调试日志
- [ ] 禁止使用 `any`，闭集字段已使用枚举或联合类型建模
- [ ] 不存在无保护的类型断言和放宽到 `Record<string, ...>` 的固定键映射
- [ ] 所有 `Form.useForm()` 调用均显式传入泛型，创建分支已完成必填字段收窄
- [ ] 前端类型定义与后端最终 HTTP JSON 契约一致，未直接照抄后端内部源码字段类型
- [ ] 接口错误只根据 `code` 映射 i18n 文案，未读取、展示或使用响应 `msg`
- [ ] 业务时间字段与包装层数值字段类型声明正确，页面展示统一使用共享时间工具
- [ ] 开启 `verbatimModuleSyntax` 时，纯类型导入已使用 `import type`
- [ ] `Transfer` 写回 `string[]` 状态时已显式执行 `keys.map(String)`
- [ ] 所有 id 语义字段（**示例** `id`、`{entity}Id`、`relId`）使用 `string`，未使用 `number` 或 `number | string`
- [ ] 所有 ids 语义字段（roleIds 等）使用 `string[]`，归一化函数无需 `map(String)`
- [ ] 涉及分层、状态、国际化、注释或样式边界的问题，已回看对应专项规范，不在本文重复裁定

---

## 5. 验收基线

- [ ] ESLint 检查通过，无 error、warning
- [ ] TypeScript 编译通过，且满足 strict 模式
- [ ] Prettier 检查通过
- [ ] 无 `any`、无业务字符串字面量状态判断、无无保护的类型断言
- [ ] 表单泛型、创建分支字段收窄和 `validateFields()` 类型提示均符合规范
- [ ] API 契约、时间字段类型、雪花 ID 字段类型和时间展示方式均符合规范
- [ ] 纯类型导入和 `Transfer` 键类型处理均符合规范
- [ ] 所有 id/ids 语义字段均使用 `string` / `string[]`，无 `number` 或 `number | string` 残留
- [ ] 若本次改动涉及分层、状态、国际化、注释或样式边界，已额外满足对应专项规范
