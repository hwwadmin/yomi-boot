# API编码适配规范

---

## 0. 文档信息
- **文档名称**: API编码适配规范
- **描述**: 定义 HTTP 出口如何通过可插拔 Encoder 完成协议适配；不规定任何具体 JSON 字段形状。
- **作者**: zwei
- **创建日期**: 2026-04-10

---

## 1. 文档定位

- **用途**：约束 **BFF / 领域服务 HTTP 出口** 的编码适配方式：业务结果与 error 如何交给 Encoder、如何显式注册、handler 边界如何收口。
- **边界**：**不**定义某一套固定响应 JSON（字段名、成功码字面量、分页 `total` 位置、HTTP 状态等均由**已注册的 Encoder 实现**决定）。**不**规定 HTTP 方法（入口编排对外方法见 [Go架构规范](./Go架构规范.md) §3.6；微服务间见 [微服务调用规范](./微服务调用规范.md) §3）。微服务间请求信封见微服务调用规范；异常创建与哨兵见 [Go异常规范](./Go异常规范.md)。
- **适用对象**：后端协议适配、脚手架接入、前后端契约对齐（契约细节以各服务 Encoder 为准）、代码评审。

---

## 2. 适用范围与关联

### 2.1 适用范围

- 适用于经 `gitee.com/hq_project/yomi-boot/component/common/handler` 写出的 HTTP 业务接口。
- 适用于业务服务启动时对 `response.Encoder` 的显式注册。
- **不**适用于 DTM Action/Compensate 回调、health 探活等明确走独立出口的路径。

### 2.2 关联文档

| 文档 | 关联点 | 说明 |
| ---- | ------ | ---- |
| [Go异常规范](./Go异常规范.md) | 错误上抛 | 哨兵 / BizError 如何到达 EncodeFailure |
| [Go业务分层规范](./Go业务分层规范.md) | handler 职责 | 仅协议适配，不组业务信封 |
| [Go架构规范](./Go架构规范.md) | `response` 包 | Encoder 注册与 MustReady |
| [微服务调用规范](./微服务调用规范.md) | 对内调用 | 与对外 Encoder 解耦；对内信封另有约定 |

---

## 3. 技术方案

### 3.1 核心原则

1. **脚手架不静默绑定响应格式**：未 `MustRegister` 不得写出业务响应。
2. **JSON 形状属于 Encoder 实现**：成功/失败/分页如何序列化、HTTP 状态码如何取值，全部由实现决定；**不得**把某一预制实现（如 `DefaultEncoder`）的字段写成全站强制契约。
3. **handler 只做适配调用**：把 usecase 的 `(data, nil)` / `error` 交给 Encoder，禁止业务路径手写 `c.JSON` 拼信封。
4. **错误不在 boot 内投影为固定码表后再编码**：框架抛简单 err；写出时把 err 交给 `EncodeFailure`。

> **边界**：对外 HTTP **方法**（入口编排可用 GET/POST）与 `router_prefix` 路由模式见 [Go架构规范](./Go架构规范.md) §3.6；微服务间 POST/JSON 与对内信封见 [微服务调用规范](./微服务调用规范.md) §3。本规范只约束 Encoder 注册与写出收口。

### 3.2 Encoder 接口（必选）

JSON **组装与解析**均由 Encoder 实现完成。解析结果核心是 `code` / `msg` / `data`；分页时实现可附带 `total`，不必额外复杂化。

```go
type Response struct {
    Code  string
    Msg   string
    Data  json.RawMessage
    Total int64 // 分页可选，无则为 0
}

type Encoder interface {
    EncodeSuccess(c *gin.Context, data any)
    EncodePage(c *gin.Context, items any, total int64)
    EncodeFailure(c *gin.Context, err error)
    Parse(body []byte) (resp Response, ok bool)
    IsSuccess(code string) bool
}
```

| 方法 | 职责 |
| ---- | ---- |
| `EncodeSuccess` / `EncodePage` / `EncodeFailure` | 组装写出 JSON |
| `Parse` | 对称解析为 `code`/`msg`/`data`（及可选 `total`）；无法识别时 `ok=false` |
| `IsSuccess` | 判定 `code` 是否成功 |

包级无额外解包 API：调用方使用 `Enc().Parse` / `Enc().IsSuccess`。供 `httprecord` 观测与集成测试；**不**约束微服务间 `rpc`（对内信封见 [微服务调用规范](./微服务调用规范.md)）。

业务可自定义字段形状；**本规范不强制某一套 JSON 字段名。**

### 3.3 显式注册

```go
func main() {
    response.MustRegister(/* 业务选择的 Encoder 实现 */)
    // ... MustInitInfra / MustBuildApp（内含 MustReady）
}
```

| API | 说明 |
| --- | ---- |
| `MustRegister(enc)` | main 显式注册，进程内仅一次；禁止 `init` 静默注册 |
| `MustReady()` | 启动路径断言已注册 |
| `Enc()` | 取得全局 Encoder；未注册则 panic |

gitee.com/hq_project/yomi-boot 可提供预制实现（如 `response.DefaultEncoder`）供业务**显式选择**注册；**预制 ≠ 本规范定义的唯一契约**，也不得被理解为全站强制响应格式。

### 3.4 handler 适配收口

| 场景 | handler API | 底层调用 |
| ---- | ----------- | -------- |
| 单条成功 | `WriteSuccess(c, data)` | `Enc().EncodeSuccess` |
| 分页成功 | `WritePageSuccess(c, items, total)` | `Enc().EncodePage` |
| 失败 | `WriteBusinessError` / `WriteFailure` | `Enc().EncodeFailure` |

优先使用 `BindAndHandle` / `BindAndHandlePage` / `BindAndHandleVoid` / `BindAndHandleWithRequest`，避免手写绑定与写出样板。

失败路径仅做必要归一（如超时、body 过大 → 对应框架哨兵），**不**在 handler 内组装 JSON 字段。

### 3.5 与异常、前端的边界

- **异常**：通用哨兵 `gitee.com/hq_project/yomi-boot/core/shared/error`；库私有就近定义；域错误在业务 `bizerr`。详见 [Go异常规范](./Go异常规范.md)。
- **前端**：只消费**该服务已注册 Encoder** 产出的实际 JSON；对接文档/OpenAPI 必须以该实现为准，不得把预制 Encoder 的字段抄成全公司唯一标准。
- **观测 / 集成测试解析**：走 `Enc().Parse` / `IsSuccess`；实现须能解析本实现写出的 body，或明确 `ok=false`。微服务间读响应由 `rpc` 按对内信封自行解析，与本规范无关。

### 3.6 特例出口（不走业务 Encoder）

| 路径 | 说明 |
| ---- | ---- |
| DTM Action/Compensate | 独立 `dtm_result` 契约，见微服务调用规范 |
| health / 探活 | 可为裸状态 JSON |

---

## 4. 检查清单

- [ ] 业务 `main` 已 `MustRegister`，未依赖静默默认
- [ ] 业务接口经 `WriteSuccess` / `WritePageSuccess` / `WriteBusinessError`（或 BindAndHandle*）写出
- [ ] 未在 usecase/service/repo 中 `c.JSON` 拼业务响应
- [ ] 自定义 Encoder 实现了全部接口方法（含 `Parse`、`IsSuccess`）
- [ ] 前后端/联调文档描述的是**本服务 Encoder** 的实际契约，而非抄写预制实现字段当强制标准
- [ ] 异常哨兵定义位置符合 Go异常规范（通用 / 库私有 / 业务域）

---

## 5. 禁止项

- 禁止将某一预制 Encoder 的 JSON 字段写成「全站唯一强制响应格式」。
- 禁止包 `init` 或脚手架静默注册 Encoder。
- 禁止业务路径绕过 handler/Encoder 手写不一致信封（特例出口除外）。
- 禁止在本规范中依赖或扩写已删除的 `response/code.go` / `FromBizCode` 模型。
