# React国际化规范

---

## 0. 文档信息
- **文档名称**: React国际化规范
- **描述**: 统一前端系统文案国际化方案，覆盖资源组织、语言切换、React 与非 React 接入方式以及翻译边界约束。
- **作者**: zwei
- **创建日期**: 2026-05-19

---

## 1. 文档定位

- **用途**：作为**业务前端工程**国际化的统一实现基线，约束资源文件结构、语言切换入口、组件接入方式和可翻译范围。
- **边界**：不替代后端多语言方案，不负责业务数据翻译，也不覆盖菜单服务端返回字段的联调改造。
- **适用对象**：业务前端工程 `src/` 下的页面、布局、Hooks、共享工具、枚举标签、国际化资源文件及相关评审场景（**yomi-boot 本仓无前端**）。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于业务前端系统文案国际化，包括页面标题、搜索栏、按钮、表单标签、占位符、校验提示、表格列标题、分页文案、空状态、弹窗标题、确认文案和通用提示消息。
- 适用于运行时语言同步，包括 `i18next`、Ant Design `ConfigProvider locale`、`Intl`、`document.documentElement.lang` 和本地持久化语言偏好。
- 适用于共享枚举标签与非 React 工具模块的翻译接入方式。
- 不适用于业务数据值翻译（用户输入、服务端业务字段原文、接口返回的调试/业务消息等）。
- 若前端使用路由/菜单配置文件承载菜单文案 key（**示例**：Umi `.umirc.ts` + `menuDataRender`），以该工程约定为准；**非**强制后端返回菜单名。

### 2.2 关联文档

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| `_prompts/doc/knowledge/js/React分层规范.md` | 分层接入边界 | 约束 React 模块、局部组件、弹窗、搜索栏和表格组件的翻译落位 |
| `_prompts/doc/knowledge/js/React代码规范.md` | Hook 与类型约束 | 约束 `useTranslation`、`useMemo` 依赖和类型写法的质量基线 |
| `_prompts/doc/knowledge/js/React状态管理规范.md` | 持久化边界 | 约束语言偏好只作为全局 UI 状态，不与业务状态混用 |
| `_prompts/doc/knowledge/api/API编码适配规范.md` | 数据边界 | 约束前端只翻译系统文案；业务失败展示以本服务 Encoder 契约为准，不擅自改写后端业务数据 |

### 2.3 阅读建议

- 初次接入国际化时，先阅读“总体架构”“资源组织与键命名”。
- 开始改页面时，重点阅读“React 组件接入规则”和“非 React 共享层接入规则”。
- 做范围评审或遗漏排查时，重点核对“翻译边界”和“检查清单”。

---

## 3. 国际化实现规范

### 3.1 总体架构

前端项目的国际化能力建议由以下职责组成（路径为常见落位，可按业务前端工程调整）：

- 语言资源注册、默认语言、支持语言、语言归一化、切换入口和副作用同步。
- 把 `i18next` 与 UI 库 locale（**示例**：Ant Design `ConfigProvider`）同步到组件树。
- 双语（或多语）资源文件。
- 语言切换入口组件。
- 应用根挂载与布局/菜单文案转换（若使用 Umi 等框架，按其 `rootContainer` / 菜单渲染钩子接入）。

约束如下：

- 默认语言与支持语言由业务前端工程配置（常见：`zh-CN`、`en-US`）。
- 语言偏好持久化 key 由业务前端自行约定（**示例**：`{app}_locale`，勿把某一产品 key 写成脚手架强制项）。
- 语言切换必须同步 UI 库 locale、`Intl` 和 HTML `lang`；若工程使用日期库（如 `dayjs`）则一并同步，未使用则跳过。
- 统一配置 fallback 语言，避免缺失 key 时出现空值。

参考骨架：

```ts
export const APP_LOCALE_STORAGE_KEY = '{app}_locale' // 示例：按业务前端命名
export const DEFAULT_APP_LOCALE = 'zh-CN' as const
export const SUPPORTED_APP_LOCALES = ['zh-CN', 'en-US'] as const

void i18n.use(initReactI18next).init({
  resources: {
    'zh-CN': { translation: zhCNMessages },
    'en-US': { translation: enUSMessages },
  },
  lng: initialLocale,
  fallbackLng: DEFAULT_APP_LOCALE,
  supportedLngs: [...SUPPORTED_APP_LOCALES],
  returnNull: false,
})
```

### 3.2 资源组织与键命名

资源文件统一放在：

- `src/i18n/resources/zh-CN.ts`
- `src/i18n/resources/en-US.ts`

资源组织规则：

- 以**业务前端模块**建立顶层命名空间（模块名随产品演进；下列仅为结构示例，非强制清单）：`common`、`layout`、`request`、`enum`、`{feature}`。
- 同一模块下按场景拆分为 `pageTitle`、`paginationTotal`、`actions`、`table`、`form`、`messages`、`confirm`、`detail` 等稳定子结构。
- `zh-CN` 与 `en-US` 的对象结构必须完全对齐，只允许值不同，不允许 key 缺失或层级漂移。
- key 命名统一使用语义化 camelCase，不用中文直译拼接；仅在对齐后端枚举编码时保留 `enum.{name}.{VALUE}` 这类稳定标识。

推荐结构（**示例**模块名 `feature`，可替换为实际业务模块）：

```ts
feature: {
  pageTitle: 'Feature Management',
  paginationTotal: 'Total {{count}} items',
  table: {
    title: 'Title',
    status: 'Status',
    publishRange: 'Time Range',
  },
  actions: {
    search: 'Search',
    reset: 'Reset',
    create: 'Create',
  },
  form: {
    fields: {
      title: 'Title',
    },
    placeholder: {
      title: 'Please enter title',
    },
    validation: {
      titleRequired: 'Please enter title',
    },
  },
  messages: {
    createSuccess: 'Created successfully',
  },
}
```

### 3.3 语言切换与运行时同步

语言切换入口必须统一收敛到 `src/i18n/index.ts`：

- 使用 `normalizeAppLocale()` 归一化任意语言标识。
- 使用 `setAppLocale()` 作为唯一对外切换入口。
- 使用 `syncAppLocaleSideEffects()` 同步 `document`、页面标题和 `localStorage`。
- 使用 `getCurrentIntlLocale()` 为非 React 格式化工具提供 locale。

允许写法：

```ts
const currentLocale = normalizeAppLocale(i18n.resolvedLanguage)

<Select<AppLocale>
  value={currentLocale}
  options={localeOptions}
  onChange={(locale) => {
    void setAppLocale(locale)
  }}
/>
```

约束如下：

- 业务组件中禁止直接调用 `i18n.changeLanguage()`。
- 业务组件中禁止直接写入 `localStorage` 保存语言偏好。
- 业务组件中禁止自行维护 Ant Design locale 或 HTML `lang` 同步逻辑。

### 3.4 React 组件接入规则

React 页面、布局、弹窗、搜索栏、表格列工厂、页面 Hook 和组件 Hook 统一使用 `useTranslation()`：

```tsx
const { t, i18n } = useTranslation()

const columns = useMemo(
  () => getMemberTableColumns(actions, t),
  [actions, i18n.resolvedLanguage, t],
)
```

规则如下：

- React 组件内的系统文案必须通过 `useTranslation()` 获取 `t`，不要把 `translate()` 当作 React 场景的主入口。
- 表格列、筛选项、弹窗配置、`Descriptions` 项、`Tabs` 项等如果通过 `useMemo()`、`useState()` 或工厂函数派生，依赖中必须包含 `i18n.resolvedLanguage`，确保切换语言后正确重算。
- 直接在 render 中使用 `t()` 的普通节点不需要额外补状态，但不能把翻译结果提前缓存到模块顶层常量。
- 表单标签、占位符、校验消息、按钮文案、`Popconfirm`、`Modal`、`Empty`、`Table locale.emptyText`、分页文案都属于必须国际化的系统文案。
- 页面 Hook 与组件 Hook 层允许使用 `useTranslation()`，负责成功提示、错误兜底、分页文案和列定义编排。
- `domain/*.domain.ts` 与 `api/*.api.ts` 不直接承担面向用户的国际化文案输出；Domain 层优先返回规则结果、编码或派生数据，由 Hook / UI 层完成翻译。

### 3.5 非 React 共享层接入规则

非 React 模块可以使用 `translate()` 或 `getCurrentIntlLocale()`，但必须限定在无法使用 Hook 的场景：

- `src/utils/request.ts` 这类请求拦截器、统一错误处理模块使用 `translate()`。
- `src/utils/datetime.ts` 这类格式化工具使用 `getCurrentIntlLocale()`。

补充约束：

- `translate()` 只用于非 React 模块或兼容历史常量映射，不用于替代组件内 `useTranslation()` 订阅。
- `domain/*.domain.ts` 默认不直接使用 `translate()` 生成界面文案；纯逻辑层应返回编码、枚举、布尔值或派生结果。
- `api/*.api.ts` 禁止使用 `translate()` 拼接接口层业务提示文案。
- 不要把 `translate()` 的返回值缓存成模块初始化常量，否则语言切换后不会自动刷新。

### 3.6 翻译边界与内容口径

前端国际化只处理系统文案，不处理业务数据本体。

应翻译内容：

- 页面标题、模块标题、搜索栏文案。
- 新增、编辑、删除、启停、发布、确认、取消等按钮与提示。
- 表单标签、占位符、校验提示、空状态、列表列标题、分页文案。
- 前端兜底错误提示、会话失效提示、网络错误提示。
- 基于稳定枚举编码的系统标签，例如状态、性别、置顶状态、结果状态。

不应翻译内容：

- 后端返回的业务数据值，例如用户名、昵称、公告标题、公告内容、日志描述、日志异常栈。
- 用户输入内容或上传内容。
- 菜单展示名来源由业务前端工程约定（配置文件 i18n key，或后端资源接口返回名）；脚手架不规定唯一来源。
- 服务端已经返回的业务错误消息正文；前端只负责提供缺省兜底提示。

判断原则：

- 如果文本由前端静态定义并直接面向用户展示，通常属于系统文案，应进入语言包。
- 如果文本来自后端数据、数据库记录或用户输入，通常不应在前端擅自翻译。

### 3.7 迁移与落地建议

新增或改造页面时，建议按以下顺序落地：

- 先在双语资源文件中补齐模块命名空间和 key。
- 再替换 React 页面、表单、弹窗、表格中的硬编码系统文案。
- 最后处理共享枚举标签、请求拦截器提示、时间格式化等非 React 场景。

落地过程中优先关注以下高频遗漏点：

- 表格列标题和 `useMemo()` 派生列配置。
- `Popconfirm`、`Tooltip`、`Modal` footer、`Empty`、分页 `showTotal`。
- 表单 `rules.message`、`placeholder`、`Select` 空选项说明、`RangePicker` 占位符。
- 顶部语言选择器、登录页、全局请求错误提示和日期格式化。

---

## 4. 检查清单

提交前请确认：

- [ ] 双语资源文件已同步新增相同结构的 key
- [ ] React 页面、组件、Hook 和布局中的系统文案均使用 `useTranslation()`
- [ ] 通过 `useMemo()` 或工厂函数生成的列、选项、描述项已包含 `i18n.resolvedLanguage` 依赖
- [ ] 非 React 工具模块仅在必要场景使用 `translate()` 或 `getCurrentIntlLocale()`
- [ ] 未对业务数据值、用户输入或菜单服务端字段做前端强制翻译
- [ ] 已手动验证语言切换后按钮、表头、空状态、日期格式和 Ant Design 内置文案同步变化
- [ ] 已完成前端项目构建验证，例如 `pnpm build`、`pnpm --dir <frontend-app> build` 或等效命令

---

## 5. 禁止项

- 禁止在 `src/pages/`、`src/layouts/`、`src/components/` 中直接写死面向用户的系统文案。
- 禁止在 React 组件中优先使用 `translate()`，绕过 `useTranslation()` 的重渲染订阅能力。
- 禁止在业务代码中分散调用 `i18n.changeLanguage()` 或直接写入语言偏好存储。
- 禁止让 `zh-CN` 与 `en-US` 资源结构不一致，造成运行时缺 key 或层级错位。
- 禁止在没有后端契约支持的情况下翻译菜单字段、业务数据值或服务端返回文案。
- 禁止把翻译结果缓存为模块初始化常量，导致切换语言后页面文案不刷新。