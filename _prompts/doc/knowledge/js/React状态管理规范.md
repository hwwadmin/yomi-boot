# React状态管理规范

---

## 0. 文档信息
- **文档名称**: React状态管理规范
- **描述**: 基于 React 的状态管理规范，包含 Zustand/Context 使用、Hook 状态协调、模块状态落位和持久化策略
- **作者**: zwei
- **创建日期**: 2026-04-08

---

## 1. 文档定位

- **用途**：作为**业务前端工程**状态设计的基线文档，约束 Zustand、Context、Hook 状态协调以及 React 模块中的状态落位方式。
- **边界**：不替代组件结构规范和 API 契约规范。
- **适用对象**：业务前端工程的页面状态设计、Store 建模、状态治理和问题排查场景（**yomi-boot 本仓无前端**）。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于**业务前端工程**的页面状态、模块共享状态、应用级全局状态和 URL 状态设计。
- 重点覆盖 Zustand、Context、Hook 状态协调、Store 设计边界与 URL 状态的使用原则。
- 不负责定义组件结构和样式组织细节，这两部分应结合关联文档阅读。

### 2.2 关联文档

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| `_prompts/doc/knowledge/js/React分层规范.md` | 分层职责边界 | 统一 React 模块分层中的状态拆分方式 |
| `_prompts/doc/knowledge/js/React国际化规范.md` | 语言偏好持久化 | 约束 locale 存储、全局语言切换和 UI 状态边界 |

### 2.3 阅读建议

- 设计状态方案时，先阅读状态选型和状态分层原则。
- 新建 Store 前，再核对文件组织、命名和持久化策略。
- 设计语言偏好持久化、全局语言切换或 locale 同步逻辑时，补充阅读 React国际化规范。
- 排查状态混乱问题时，优先检查页面级、模块级和应用级状态是否越界。

---

## 3. 技术方案

### 3.1 前端设计

#### 3.1.1 状态管理选型

| 方案 | 适用场景 | 项目选用 |
| ---- | -------- | -------- |
| Zustand | 全局状态、跨组件共享 | ✓ |
| React Context | 主题、身份认证、国际化 | ✓ |
| useState | 组件内部状态 | ✓ |
| URL State | 页面参数、筛选条件 | ✓ |

#### 3.1.2 Zustand Store 规范

```typescript
// store/auth.ts - 认证状态示例
import { create } from 'zustand'

interface User {
  id: string
  username: string
  nickname: string
}

interface AuthState {
  user: User | null
  token: string | null
  setAuth: (user: User, token: string) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  token: null,
  setAuth: (user, token) => {
    saveEncryptedString('token', token)
    set({ user, token })
  },
  logout: () => {
    removeEncryptedString('token')
    set({ user: null, token: null })
  },
}))
```

其中 `saveEncryptedString` / `removeEncryptedString` 代表项目统一的敏感信息持久化封装；认证状态示例中禁止直接把 token 明文落到浏览器持久化存储。

#### 3.1.3 Store 文件结构

```
src/store/
├── index.ts          # 导出所有 store
├── auth.ts           # 认证状态
├── resource.ts       # 菜单/按钮资源状态
├── theme.ts          # 主题状态
└── xxx.ts            # 按业务模块拆分（如后续 user.ts、settings.ts）
```

#### 3.1.4 模块状态落位补充口径

React 模块的目录结构、分层职责和依赖方向统一以 `React分层规范` 为单一事实来源，本文不重复维护另一套 `page.tsx / hooks / domain / api` 职责表。

本文只补充状态管理相关口径：

1. 页面级 UI 状态优先放在模块 `hooks/` 中，不要回流到 `page.tsx`
2. Zustand / Context 只承载模块级或应用级共享状态，不替代 `hooks/` 处理页面局部流程
3. Store 不直接调用 `api/`，模块内接口调用与请求生命周期编排由 `hooks/` 统一协调
4. `domain/` 只返回无状态纯逻辑结果，不持有 React / Zustand 状态，也不调用 `api/`

#### 3.1.5 状态分层原则

| 层级 | 状态类型 | 存储方式 | 示例 |
| ---- | -------- | -------- | ---- |
| 页面级 | 页面 Hook 内状态 | useState / useReducer | 表单输入、分页、弹窗开关、加载状态 |
| 模块级 | 业务相关状态 | Zustand | 用户信息、购物车 |
| 应用级 | 全局共享状态 | Context/Zustand | 主题、语言、认证 |
| URL级 | 页面参数状态 | useSearchParams | 筛选条件、排序 |

#### 3.1.6 持久化策略

| 数据类型 | 持久化方式 | 示例 |
| -------- | ---------- | ---- |
| 敏感信息 | localStorage（加密） | token |
| 用户偏好 | localStorage | 主题、语言 |
| 缓存数据 | localStorage/sessionStorage | 列表缓存 |
| 运行时状态 | 内存 | 加载状态 |

---

## 4. 约束与规范

- [ ] 遵循 React代码规范中的约束
- [ ] 遵循 React分层规范中的约束
- [ ] 每个 Store 必须使用 TypeScript interface 定义状态类型
- [ ] Store 文件放在 `src/store/` 目录下，按业务模块命名
- [ ] 禁止在 Store 中直接调用 API，接口调用与请求生命周期编排统一放在模块 `hooks/`
- [ ] 页面级 UI 状态优先放在模块 `hooks/`，不要塞回 `page.tsx` 或无差别提升到 Store
- [ ] Zustand / Context 仅承载共享状态，禁止把纯业务规则塞进 Store
- [ ] `domain/` 层保持无状态纯逻辑，不得持有 React / Zustand 状态，也不得调用 `api/`
- [ ] 状态修改必须通过 actions（set 方法），禁止直接修改 state
- [ ] 需要持久化的状态必须显式声明持久化策略
- [ ] token 等敏感信息如需持久化，必须通过统一加密存储封装处理，禁止直接明文写入 `localStorage`
- [ ] 每个 Store 的职责必须单一，不允许一个大 Store 包含所有状态
- [ ] 页面级 UI 状态优先放在模块 `hooks/` 中，禁止把纯业务规则塞进 Zustand 或页面组件

---

## 5. 验收标准（DoD — Definition of Done）

### 5.1 前端验收

- [ ] Store 状态类型定义完整，无隐式 any
- [ ] 状态修改通过 actions，不直接修改 state
- [ ] 持久化状态正确处理（token 等敏感信息加密存储）
- [ ] Store 职责单一，无状态混乱
- [ ] 模块状态落位符合 React分层规范，本文未展开的职责边界已按主规范复核
- [ ] 页面级 UI 状态位于模块 `hooks/`，共享状态才进入 Store
- [ ] Store 不直接发请求，接口调用由 `hooks/` 统一协调
- [ ] `domain/` 保持无状态纯逻辑，未引入 React、Zustand、HTTP，且不调用 `api/`

---

