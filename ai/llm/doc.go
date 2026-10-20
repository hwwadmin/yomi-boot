// Package llm 提供协议无关的大模型对话门面，支持 OpenAI Chat Completions 与 Anthropic Messages。
//
// 本包不对内泄漏官方客户端类型；业务只依赖 New / MustNew / Client 与本包哨兵。
// 不进 bootstrap、不读 YAML、不做配额限流与重试。配置来源由调用方自行组装。
//
// # 基本用法
//
//	cli, err := llm.New(llm.Config{
//	    Protocol: llm.ProtocolOpenAI,
//	    APIURL:   "https://api.openai.com/v1",
//	    APIKey:   apiKey,
//	    Model:    "gpt-4o",
//	})
//
// 模型由 Config.Model 锁定；换模型请另 New 一个实例。请求侧不再选型。
// Temperature / MaxTokens：Config 空则缺省 1.0 / 4096；请求指针非空则覆盖。
// OpenAI 协议会同时写入 max_tokens 与 max_completion_tokens，兼顾兼容网关与 reasoning 模型。
//
//	resp, err := cli.Chat(ctx, llm.ChatRequest{
//	    Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
//	})
//
// # 流式
//
//	stream, err := cli.ChatStream(ctx, req)
//	if err != nil {
//	    return err
//	}
//	defer stream.Close()
//	for {
//	    chunk, err := stream.Recv()
//	    if err == io.EOF {
//	        break
//	    }
//	    if err != nil {
//	        // 含 context.Canceled / DeadlineExceeded：截断，不得当成完整回复
//	        return err
//	    }
//	    _ = chunk.Delta
//	}
//
// # BaseURL（APIURL 原样注入，本包不拼接路径）
//
//   - OpenAI Chat：官方为 `https://api.openai.com/v1`。不要写成 `.../v1/v1`。
//     兼容网关按其文档填写（通常同样带 `/v1` 前缀）。
//   - Anthropic：官方为 `https://api.anthropic.com`（不要再加 `/v1`，SDK 会请求 `v1/messages`）。
//
// # HTTPClient
//
// 可选；仅提供 Transport / 代理。建议 Client.Timeout=0，由本包 Timeout（0 则 60s）
// 以请求级 context.WithTimeout 控制整段墙钟（含流式）。New 之后再改该 Client 视为未定义行为。
// 高强度调用建议自定义 Transport，提高 MaxIdleConnsPerHost 等连接池参数。本包不做限流。
//
// # Anthropic 轮次与 system
//
// 抽掉 system 后：messages 至少 1 条、第一条必须是 user、user/assistant 严格交替，否则
// ErrInvalidArgument。全部 system 文本按出现顺序换行拼入 SDK system 字段。
// 交错写在对话中间的 system 抽到顶部后，与 OpenAI「插在对话中间的 system」不完全等价；
// 换协议只保证仍调用 Chat/ChatStream，不保证任意 Messages 两协议都能发出。
//
// # 错误
//
// 包内哨兵：ErrInvalidArgument / ErrUnauthorized / ErrRateLimited / ErrBadResponse。
// ErrInvalidArgument 仅本包校验失败；上游 HTTP 400 与无状态码读失败走 ErrBadResponse。
// 正常结束的 Recv 返回未包装 io.EOF；当次超时/取消返回 ctx.Err()（可 Wrap），禁止改写成 EOF。
//
// @author zwei
package llm
