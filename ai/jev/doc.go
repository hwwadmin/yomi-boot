// Package jev 提供协议无关的 Jev 结构化决策门面，首版为 TypeSafe System One 兼容 HTTP。
//
// 本包与 ai/llm、ai/embedding 相互独立，配置与协议类型互不混用。
// 首版仅 ProtocolSystemOne。APIURL 为完整 POST 地址（非 Chat 的 BaseURL 惯例）。
// 模型由 Config.Model 锁定。无流式。不进 bootstrap、不读 YAML、不做限流。
//
// 千问决策模型示例：`https://maas.qianwenaiapi.com/compatible-mode/v1/systemone`，
// 模型 `decision-model-preview`。
//
// # 基本用法
//
//	cli, err := jev.New(jev.Config{
//	    Protocol: jev.ProtocolSystemOne,
//	    APIURL:   "https://maas.qianwenaiapi.com/compatible-mode/v1/systemone",
//	    APIKey:   apiKey,
//	    Model:    "decision-model-preview",
//	})
//	out, err := cli.Decide(ctx, jev.DecideRequest{
//	    State: jev.StateString("support note"),
//	    Questions: map[string]jev.Question{
//	        "urgent": {Type: jev.QuestionNoul, Instructions: "是否紧急？"},
//	    },
//	})
//
// # HTTPClient
//
// 可选；仅提供 Transport / 代理。建议 Client.Timeout=0，由本包 Timeout（0 则 60s）
// 以请求级 context.WithTimeout 控制整段墙钟。New 之后再改该 Client 视为未定义行为。
// 高强度调用建议自定义 Transport，提高 MaxIdleConnsPerHost 等连接池参数。本包不做限流。
//
// # 错误
//
// 包内哨兵：ErrInvalidArgument / ErrUnauthorized / ErrRateLimited / ErrBadResponse。
// ErrInvalidArgument 仅本包校验失败；上游 HTTP 400 与无状态码读失败走 ErrBadResponse。
//
// @author zwei
package jev
