# React分层规范

---

## 0. 文档信息
- **文档名称**: React分层规范
- **描述**: 统一约束 `src/components` 与 `src/pages` 下 React 模块的目录结构、分层职责、依赖边界和拆分方式
- **作者**: zwei
- **创建日期**: 2026-04-08

---

## 1. 文档定位

- **用途**：为**基于 yomi-boot 的业务前端工程**提供唯一分层模型，避免同一前端仓同时存在多套目录结构和职责口径。
- **核心结论**：`src/components` 与 `src/pages` 只有根目录不同，模块内部结构和规则完全一致。
- **适用对象**：业务前端仓的页面开发、组件抽离、模块重构、代码评审（**yomi-boot 本仓无前端工程**）。
- **边界说明**：
  1. 状态边界细则见 `_prompts/doc/knowledge/js/React状态管理规范.md`
  3. 国际化细则见 `_prompts/doc/knowledge/js/React国际化规范.md`
  4. 注释细则见 `_prompts/doc/knowledge/js/React注释规范.md`
  5. API 出口编码适配见 `_prompts/doc/knowledge/api/API编码适配规范.md`（契约=已注册 Encoder）

---

## 2. 统一模块模型

### 2.1 模块定义

- **通用模块**：位于 `src/components/{module}/`，面向跨页面或跨业务复用。
- **业务模块**：位于 `src/pages/{module}/`，面向具体业务页面或业务场景。
- **统一规则**：无论模块位于 `src/components` 还是 `src/pages`，都统一使用同一套内部结构、同一套职责边界、同一套拆分规则。

### 2.2 标准目录结构

```text
{module}/
├── page.tsx
├── hooks/
│   └── useOrderPage.ts
├── domain/
│   └── order.domain.ts
├── api/
│   └── order.api.ts
├── components/
│   └── OrderTable.tsx
├── types/
│   └── order.types.ts
└── css/
    └── order.module.css
```

说明：

1. `page.tsx` 是模块唯一 UI 入口文件，这里的 `page` 表示“模块入口”，不专指路由页面。
2. 上图是完整模板。若某层当前确实没有职责，可以暂时不创建对应目录；但一旦该层职责出现，必须回到标准结构，禁止混写到其他层。
3. `css/` 与 `types/`、`hooks/`、`domain/` 一样，是独立目录，不允许再把 `.module.css` 文件混放在模块根或其他目录中。

### 2.3 命名约定

| 层级 | 文件命名 | 说明 |
| ---- | ---- | ---- |
| UI 入口 | `page.tsx` | 模块唯一入口文件 |
| 主 Hook | `hooks/use{Module}Page.ts` | 负责状态编排与 API 协调 |
| Domain | `domain/{module}.domain.ts` | 负责纯逻辑 |
| API | `api/{module}.api.ts` | 负责请求边界 |
| Types | `types/{module}.types.ts` | 负责模块私有类型 |
| CSS | `css/{module}.module.css` | 负责模块样式 |

### 2.4 分层职责

| 层级 | 允许内容 | 禁止内容 |
| ---- | ---- | ---- |
| `page.tsx` | UI 组装、事件绑定、调用 Hook、挂载样式 | 请求发送、业务规则、数据解释、复杂类型定义 |
| `hooks/` | 状态协调、副作用、事件编排、调用 `domain`、作为 `api/` 的唯一调用方 | 纯逻辑函数沉积、让 `page.tsx` 或 `components/*.tsx` 直接依赖 `api/` |
| `domain/` | 无状态纯函数、业务规则、数据解释、数据整形 | React、HTTP、页面组件依赖、调用 `api/` |
| `api/` | 请求发送、响应接收、最薄的传输边界适配，仅向 `hooks/` 暴露调用入口 | 业务规则、UI 文案组装、展示逻辑、被 `page.tsx` 或 `components/*.tsx` 直接调用 |
| `components/` | 当前模块内部的纯 UI 拆分块 | 自己处理请求、业务规则、自建分层体系 |
| `types/` | 表单类型、搜索类型、展示模型、除 UI Props 外的模块私有类型 | JSX、状态、副作用 |
| `css/` | 模块样式、局部样式归档 | 业务逻辑、类型定义 |

分层口径：`hooks/` 是模块编排层，负责串联状态、副作用、`domain/` 纯函数与 `api/` 调用；`domain/` 是无状态纯函数层，不承载副作用，也不调用 `api/`。

### 2.5 依赖方向

```text
page.tsx -> hooks / components / types / css
hooks -> domain / api / types
components/*.tsx -> types / css / parent callbacks
domain -> types
api -> shared request layer / shared transport types
```

依赖约束：

1. `page.tsx` 不直接依赖 `api/`、不直接依赖 `domain/`
2. `components/*.tsx` 不直接依赖 `api/`、不直接依赖 `domain/`
3. `hooks/` 是模块编排层，也是模块内调用 `api/` 的唯一入口
4. `domain/` 必须保持无状态，不依赖 React、页面、Hook、HTTP 客户端，且不得调用 `api/`
5. `api/` 不承担业务规则解释，只负责对外请求边界，不向 `domain/` 暴露调用职责

---

## 3. 模块拆分规则

### 3.1 局部组件规则

`components/` 目录默认只放当前模块内部的简单局部 UI 文件，例如：

- 搜索栏
- 表格列渲染块
- 详情面板
- 状态标签
- 表单弹窗壳子

这些文件默认只承担展示和事件转发，不单独建立自己的 `hooks/`、`domain/`、`types/`、`css/`、`api/`。

类型归属说明：

1. Props 定义属于 UI 接口，不视为必须下沉到 `types/{module}.types.ts` 的业务类型。
2. 简单局部 UI 的 Props 可以直接定义在对应 `page.tsx` 或 `components/*.tsx` 文件内。
3. 只有被模块多层复用的业务类型、表单类型、搜索类型、展示模型，才统一收口到 `types/`。

### 3.2 组件升格规则

当 `components/*.tsx` 中的局部 UI 出现以下任一情况时，不再视为“简单局部组件”，必须升格为独立模块：

1. 出现独立状态协调或副作用
2. 出现独立纯逻辑、规则判断或数据解释
3. 出现独立类型文件需求
4. 出现独立样式目录需求
5. 出现独立请求边界需求

### 3.3 升格后的放置位置

| 场景 | 放置位置 |
| ---- | ---- |
| 仅服务当前业务模块 | `src/pages/{page}/components/{subModule}/` |
| 需要跨页面复用 | `src/components/{subModule}/` |

升格后继续完整应用第 2 章的标准目录结构，不允许发明第二套轻量结构。

---

## 4. 关键约束

### 4.1 结构约束

1. 所有 React 模块统一适用同一套结构，禁止再区分“页面规范”和“组件规范”两套文档口径。
2. 模块根目录只能有一个 UI 入口文件，即 `page.tsx`。
3. 除 UI Props 外，模块私有类型统一收口到 `types/{module}.types.ts`。
4. 模块样式统一收口到 `css/{module}.module.css` 或子模块自己的 `css/`。
5. `.module.css` 不允许散落在模块根、`hooks/`、`domain/`、`api/`、`types/` 同级。

### 4.2 职责约束

1. `page.tsx` 不直接发请求、不直接写业务规则、不直接解释接口数据。
2. `hooks/` 只负责状态协调和编排，作为模块编排层统一组织副作用、`domain/` 调用与 `api/` 调用，不沉积纯逻辑函数，并且是 `api/` 的唯一调用方。
3. `domain/` 只负责无状态纯逻辑，不依赖 React 和 HTTP，不允许调用 `api/`。
4. `api/` 只负责接口边界，不承担展示解释，只允许由 `hooks/` 调用。
5. `components/*.tsx` 默认只做纯 UI，不直接处理请求、业务规则，也不直接调用 `api/`。

### 4.3 代码约束

1. 所有 Props 使用 `interface` 定义，且允许保留在对应 UI 文件中。
2. 展示型或无状态子组件在签名处使用 `Readonly<Props>`。
3. 事件处理函数在传给子组件时使用 `useCallback`。
4. `Form.useForm()` 必须显式传入泛型。
5. 创建/编辑共用表单在调用创建 API 前，必须显式收窄必填字段。
6. 时间展示统一使用 `src/utils/datetime.ts`。
7. `Modal` 统一使用 `destroyOnHidden`。
8. 数值输入统一使用 `InputNumber`。
9. 注释统一遵循 `_prompts/doc/knowledge/js/React注释规范.md`。

---

## 5. 验收清单

### 5.1 结构验收

- [ ] 模块位于 `src/components` 或 `src/pages` 的正确根目录下
- [ ] 模块根目录以 `page.tsx` 作为唯一 UI 入口
- [ ] 模块已按职责拆出 `hooks/`、`domain/`、`api/`、`types/`、`css/`
- [ ] `components/*.tsx` 仍为简单局部 UI；若已复杂化，已升格为独立模块
- [ ] 没有第二套目录模型与本规范并存

### 5.2 职责验收

- [ ] `page.tsx` 中没有请求发送、业务规则和数据解释
- [ ] `hooks/` 中没有可独立复用的纯逻辑函数
- [ ] `hooks/` 是模块内调用 `api/` 的唯一入口
- [ ] `domain/` 保持无状态，不依赖 React 与 HTTP，且不调用 `api/`
- [ ] `api/` 不承载业务规则和展示文案，且只由 `hooks/` 调用
- [ ] `components/*.tsx` 不直接依赖 `api/`，也不直接依赖 `domain/`

### 5.3 类型与样式验收

- [ ] 模块私有类型已独立收口到 `types/{module}.types.ts`
- [ ] 模块样式已独立收口到 `css/` 目录
- [ ] 不存在把 `.module.css` 混放在 `page.tsx`、`hooks/`、`domain/`、`api/`、`types/` 同级的情况
- [ ] 展示型子组件 Props 已使用独立命名 `interface`

### 5.4 通用规则验收

- [ ] 所有 `Form.useForm()` 已显式传入泛型
- [ ] 创建/编辑共用表单不存在直接把可选字段 `values` 传给创建 API 的情况
- [ ] 业务时间字段展示统一通过共享时间工具格式化
- [ ] 新增 `Modal` 已使用 `destroyOnHidden`
- [ ] 数值表单项统一使用 `InputNumber`
- [ ] 若本次改动涉及注释，相关文件已满足 React注释规范


