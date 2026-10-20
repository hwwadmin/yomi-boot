// Package llm 定义统一对话门面与请求响应结构。
//
// @author zwei
package llm

import (
	"context"

	"gitee.com/hq_project/yomi-boot/errors"
)

// Role 对话角色；首版仅 system / user / assistant
type Role string

const (
	// RoleSystem 系统提示
	RoleSystem Role = "system"
	// RoleUser 用户
	RoleUser Role = "user"
	// RoleAssistant 助手
	RoleAssistant Role = "assistant"
)

// Client 协议无关的同步/流式 Chat 门面；同一实例可并发调用
type Client interface {
	// Chat 发起同步对话
	//
	// 入参 Messages 至少 1 条；模型由构造配置锁定，本请求不再选型。
	// Temperature / MaxTokens 空则回退 Config 同名字段（构造期已填缺省 1.0 / 4096）。
	// 超时或取消返回上下文错误（可包装）；空助手文本返回 ErrBadResponse。
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	// ChatStream 开启流式对话；调用方须 defer Close
	//
	// 建连失败（含超时/取消）在返回前失败，不返回半开流。
	// Temperature / MaxTokens 回退规则与 Chat 相同。
	ChatStream(ctx context.Context, req ChatRequest) (Stream, error)
}

// Stream 流式增量读取器；Recv 须串行；Close 可与 Recv 并发且幂等
type Stream interface {
	// Recv 读取下一增量
	//
	// 正常结束、已 Close、已返回过非「并发 Recv」错误之后再 Recv：未包装 io.EOF。
	// 当次超时或取消：返回上下文错误（可包装），禁止改写为 io.EOF。
	// 同一流并发 Recv：ErrInvalidArgument（不关闭流）。
	Recv() (*ChatChunk, error)
	// Close 释放流资源；幂等；成功返回 nil；可与 Recv 并发
	Close() error
}

// Message 单条对话消息；Content 允许空串
type Message struct {
	Role    Role   // Role 仅 system / user / assistant
	Content string // Content 允许空串
}

// ChatRequest 同步与流式共用入参；模型由 Config.Model 锁定
type ChatRequest struct {
	Messages    []Message // Messages 至少 1 条
	Temperature *float64  // Temperature 可选；空则回退 Config.Temperature（缺省 1.0）；本包不校验区间
	MaxTokens   *int      // MaxTokens 可选；空则回退 Config.MaxTokens（缺省 4096）；若设置必须 >0
}

// ChatResponse 同步完整回复；Message.Role 恒为助手
type ChatResponse struct {
	ID      string      // ID 是上游响应标识，可能为空
	Model   string      // Model 是实际上游模型名
	Message Message     // Message 是助手回复；Role 恒为 assistant
	Usage   *TokenUsage // Usage 可选；上游未给用量时为 nil
}

// TokenUsage 用量统计；可选
type TokenUsage struct {
	PromptTokens     int // PromptTokens 是输入令牌数
	CompletionTokens int // CompletionTokens 是输出令牌数
	TotalTokens      int // TotalTokens 是合计令牌数
}

// ChatChunk 流式增量；Delta 在协议控制帧上可为空
type ChatChunk struct {
	ID    string // ID 是流会话标识，控制帧上可能为空
	Model string // Model 是实际上游模型名，控制帧上可能为空
	Delta string // Delta 是增量文本；调用方可忽略空增量
}

// validateChatRequest 校验消息列表、角色与请求侧最大令牌数；非法返回 ErrInvalidArgument
func validateChatRequest(req ChatRequest) error {
	// 消息列表不可为空
	if len(req.Messages) == 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	// 若设置最大令牌数则必须大于 0
	if req.MaxTokens != nil && *req.MaxTokens <= 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem, RoleUser, RoleAssistant:
		default:
			// 未知角色不得发往上游
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}

// resolveTemperature 请求非空则用之，否则回退配置默认（构造期已填 1.0）
func resolveTemperature(req, def *float64) *float64 {
	if req != nil {
		return req
	}
	return def
}

// resolveMaxTokens 请求非空则用之，否则回退配置默认（构造期已填 4096）
func resolveMaxTokens(req, def *int) *int {
	if req != nil {
		return req
	}
	return def
}
