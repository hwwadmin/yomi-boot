// 定义填槽模型调用端口。
//
// @author zwei

package tool

import (
	"context"
	"encoding/json"
)

// Completer 填槽模型端口；业务用 llm.Client 等实现适配，本包不依赖具体模型。
// 实现须并发安全；一次 Complete 只处理一个请求。
type Completer interface {
	// Complete 执行一次填槽模型调用；须尊重 ctx
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

// CompletionRequest 填槽请求；System/User 已由本包拼好
type CompletionRequest struct {
	Tool   string          // Tool 是工具名，便于按工具路由模型
	System string          // System 是 system 提示词
	User   string          // User 是 user 提示词
	Schema json.RawMessage // Schema 是工具参数 Schema，供 structured output 适配器使用
}

// CompletionResponse 填槽响应
type CompletionResponse struct {
	Content string // Content 是模型原始输出
}
