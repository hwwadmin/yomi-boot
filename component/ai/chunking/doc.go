// Package chunking 提供独立于检索流程的文本切块策略库。
//
// 内置四种策略，也可实现 Chunker 注入自定义策略。
// 本包与 ai 其他组件零依赖，不读 YAML、不进插件链、不做限流。
// 块长按 rune 计，单块不得超过 MaxChunkSize 个字符。
//
// # 基本用法
//
//	chunker, err := chunking.New(chunking.Config{
//	    Strategy: chunking.StrategyMarkdown,
//	    Markdown: chunking.MarkdownConfig{ChunkSize: 800, MaxHeadingLevel: 3},
//	})
//	chunks, err := chunker.Chunk(text)
//	chunks, err = chunking.ChunkText(text, chunking.Config{})
//
// # 配置
//
// Config 只有 Strategy 对应的子配置生效，其余子配置忽略：
//
//   - recursive（默认）：RecursiveConfig，块长、重叠与分隔符优先级
//   - fixed：FixedConfig，块长与重叠
//   - sentence：SentenceConfig，块长、重叠与句末分隔符
//   - markdown：MarkdownConfig，块长、重叠与最大标题层级
//
// 块长与重叠缺省：ChunkSize 为 0 则 800；ChunkOverlap 为 0 则 min(100, ChunkSize/4)；
// ChunkOverlap 必须小于 ChunkSize，ChunkSize 不得超过 MaxChunkSize。
//
// # 契约
//
// Chunker 实现须并发安全：输入 trim 后非空时必须返回至少 1 个非空块，
// 单块不得超过 MaxChunkSize 个字符，否则返回 ErrInvalidArgument；空文本返回 ErrInvalidArgument。
// 调用方可用 NormalizeChunks 对自定义实现的输出做契约校验兜底。
//
// # 错误
//
// 包内哨兵：ErrInvalidArgument。
//
// @author zwei
package chunking
