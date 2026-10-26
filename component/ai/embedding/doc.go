// Package embedding 提供协议无关的文本向量化门面，首版仅 OpenAI Embeddings。
//
// 本包与 ai/llm、ai/jev 相互独立，配置与协议类型互不混用。
// 模型由 Config.Model 锁定；换模型请另 New 一个实例。
// APIKey 可空，供本地或免费、免鉴权网关；空则不带 Authorization。
// Dimensions 是每次 Embed 调用的必填入参（输出维度，必须 >0），返回向量按它校验长度。
// SendDimensions 开启时随请求下发上游 dimensions 参数（OpenAI text-embedding-3 系列等支持）；
// 不支持该参数的网关会返回 400，此时仅本地校验维度。
// 一次 Embed 整批成功或整包失败；输入超过 MaxBatch（默认 2048）时按该值分批。不进插件链、不读 YAML、不做限流。
//
// # 基本用法
//
//	emb, err := embedding.New(embedding.Config{
//	    Protocol: embedding.ProtocolOpenAI,
//	    APIURL:   "https://api.openai.com/v1",
//	    APIKey:   apiKey,
//	    Model:    "text-embedding-3-small",
//	})
//	vec, err := emb.Embed(ctx, embedding.Request{
//	    Inputs:     []string{"hello", "world"},
//	    Dimensions: 1536,
//	})
//
// # BaseURL（APIURL 原样注入，本包不拼接路径）
//
// OpenAI Embeddings 官方为 `https://api.openai.com/v1`。不要写成 `.../v1/v1`。
// 兼容网关按其文档填写（通常同样带 `/v1` 前缀）。
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
package embedding
