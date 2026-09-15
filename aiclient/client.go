// Package aiclient 定义统一对话门面与请求响应结构。
//
// @author zwei
package aiclient

import (
	"context"
	"strings"

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

// AIClient 协议无关的同步/流式 Chat 门面；同一实例可并发调用
type AIClient interface {
	// Chat 发起同步对话
	//
	// 入参 Messages 至少 1 条；Model 空则回退 DefaultModel；二者皆空返回 ErrInvalidArgument。
	// 超时或取消返回上下文错误（可包装）；空助手文本返回 ErrBadResponse。
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	// ChatStream 开启流式对话；调用方须 defer Close
	//
	// 建连失败（含超时/取消）在返回前失败，不返回半开流。
	ChatStream(ctx context.Context, req ChatRequest) (ChatStream, error)
}

// ChatStream 流式增量读取器；Recv 须串行；Close 可与 Recv 并发且幂等
type ChatStream interface {
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

// ChatRequest 同步与流式共用入参
type ChatRequest struct {
	Model       string    // Model 空则使用 DefaultModel
	Messages    []Message // Messages 至少 1 条
	Temperature *float64  // Temperature 可选；本包不校验区间
	MaxTokens   *int      // MaxTokens 可选；若设置必须 >0
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

// resolveModel 解析最终模型名；请求与 DefaultModel 皆空则返回 ErrInvalidArgument
func resolveModel(req ChatRequest, defaultModel string) (string, error) {
	model := strings.TrimSpace(req.Model)
	// 请求未指定时回退默认模型
	if model == "" {
		model = strings.TrimSpace(defaultModel)
	}
	if model == "" {
		return "", errors.Sentinel(ErrInvalidArgument)
	}
	return model, nil
}

// validateChatRequest 校验消息列表、角色、最大令牌数，并解析最终模型名
//
// 返回值：
//   - string: 解析后的模型名
//   - error: 非法时为 ErrInvalidArgument
func validateChatRequest(req ChatRequest, defaultModel string) (string, error) {
	// 消息列表不可为空
	if len(req.Messages) == 0 {
		return "", errors.Sentinel(ErrInvalidArgument)
	}
	// 若设置最大令牌数则必须大于 0
	if req.MaxTokens != nil && *req.MaxTokens <= 0 {
		return "", errors.Sentinel(ErrInvalidArgument)
	}
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem, RoleUser, RoleAssistant:
		default:
			// 未知角色不得发往上游
			return "", errors.Sentinel(ErrInvalidArgument)
		}
	}
	return resolveModel(req, defaultModel)
}

// validateAnthropicTurns 校验抽掉系统提示后的轮次：非空、用户开头、用户与助手严格交替
func validateAnthropicTurns(messages []Message) error {
	var turns []Role
	for _, m := range messages {
		if m.Role == RoleSystem {
			continue
		}
		turns = append(turns, m.Role)
	}
	// 仅系统提示或非用户开头不得发往供应商
	if len(turns) == 0 || turns[0] != RoleUser {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for i := 1; i < len(turns); i++ {
		if turns[i] == turns[i-1] {
			return errors.Sentinel(ErrInvalidArgument)
		}
		if turns[i] != RoleUser && turns[i] != RoleAssistant {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}

// joinSystemPrompts 按出现顺序换行拼接全部系统提示；无系统提示时返回空串
func joinSystemPrompts(messages []Message) string {
	var parts []string
	for _, m := range messages {
		if m.Role == RoleSystem {
			parts = append(parts, m.Content)
		}
	}
	return strings.Join(parts, "\n")
}
