// Package llm OpenAI Chat Completions 适配（包私有）。
//
// @author zwei
package llm

import (
	"context"
	stderrors "errors"
	"io"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/shared"
)

// openaiClient OpenAI 协议的 Client 实现
type openaiClient struct {
	sdk     openai.Client // sdk 是官方客户端，选项不可变，可并发调用
	cfg     Config        // cfg 是构造期拷贝后的配置
	timeout time.Duration // timeout 是整段墙钟超时（0 已回退为 60s）
}

var _ Client = (*openaiClient)(nil)

// newOpenAIClient 构造 OpenAI 协议客户端
//
// 官方客户端仅持有不可变选项，每次请求独立执行，同一实例可并发 Chat / ChatStream。
func newOpenAIClient(cfg Config) (*openaiClient, error) {
	// 密钥与地址原样注入；关闭官方默认重试
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.APIURL),
		option.WithMaxRetries(0),
		option.WithRequestTimeout(timeoutOrDefault(cfg.Timeout)),
	}
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
	}
	for k, v := range cfg.ExtraHeaders {
		opts = append(opts, option.WithHeader(k, v))
	}
	return &openaiClient{
		sdk:     openai.NewClient(opts...),
		cfg:     cfg,
		timeout: timeoutOrDefault(cfg.Timeout),
	}, nil
}

// Chat 发起同步对话；空选项列表或空助手文本返回 ErrBadResponse
func (c *openaiClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	params, err := c.buildParams(req)
	if err != nil {
		return nil, err
	}
	// 整段请求绑定墙钟超时
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	resp, err := c.sdk.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, mapOpenAIError(err)
	}
	// 空选项列表视为协议异常
	if resp == nil || len(resp.Choices) == 0 {
		return nil, wrapSentinel(ErrBadResponse, errors.New("ai/llm: 空 choices"))
	}
	text := resp.Choices[0].Message.Content
	out := &ChatResponse{
		ID:    resp.ID,
		Model: resp.Model,
		Message: Message{
			Role:    RoleAssistant,
			Content: text,
		},
	}
	if resp.Usage.TotalTokens > 0 || resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
		out.Usage = &TokenUsage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		}
	}
	// 无助手文本视为协议异常
	if text == "" {
		return nil, wrapSentinel(ErrBadResponse, errors.New("ai/llm: 无 assistant 文本"))
	}
	return out, nil
}

// ChatStream 开启流式对话；建连失败时关闭底层流并返回错误，不返回半开流
func (c *openaiClient) ChatStream(ctx context.Context, req ChatRequest) (Stream, error) {
	params, err := c.buildParams(req)
	if err != nil {
		return nil, err
	}
	// 流式请求绑定墙钟超时，由返回的流负责释放
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	stream := c.sdk.Chat.Completions.NewStreaming(ctx, params)
	// 建连失败关闭底层流，不把半开流交给调用方
	if err := stream.Err(); err != nil {
		cancel()
		_ = stream.Close()
		return nil, mapOpenAIError(err)
	}
	return newChatStream(ctx, cancel, &openaiPump{stream: stream, mapErr: mapOpenAIError}), nil
}

// buildParams 将门面请求映射为官方客户端参数；入参非法返回 ErrInvalidArgument
func (c *openaiClient) buildParams(req ChatRequest) (openai.ChatCompletionNewParams, error) {
	if err := validateChatRequest(req); err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages))
	// 按角色映射消息；未知角色已在校验阶段拒绝
	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			msgs = append(msgs, openai.SystemMessage(m.Content))
		case RoleUser:
			msgs = append(msgs, openai.UserMessage(m.Content))
		case RoleAssistant:
			msgs = append(msgs, openai.AssistantMessage(m.Content))
		}
	}
	params := openai.ChatCompletionNewParams{
		Messages: msgs,
		Model:    shared.ChatModel(c.cfg.Model),
	}
	if temp := resolveTemperature(req.Temperature, c.cfg.Temperature); temp != nil {
		params.Temperature = openai.Float(*temp)
	}
	if maxTok := resolveMaxTokens(req.MaxTokens, c.cfg.MaxTokens); maxTok != nil {
		n := int64(*maxTok)
		// 同时写入新旧字段：兼容网关用 max_tokens，reasoning 模型用 max_completion_tokens
		params.MaxTokens = openai.Int(n)
		params.MaxCompletionTokens = openai.Int(n)
	}
	return params, nil
}

// mapOpenAIError 映射 OpenAI 官方客户端错误；能取出 HTTP 状态时走哨兵，取消或超时保持上下文错误
func mapOpenAIError(err error) error {
	if err == nil {
		return nil
	}
	// 取消或超时保持标准库错误，便于按错误链判断
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return wrapCtxErr(err)
	}
	var apiErr *openai.Error
	if stderrors.As(err, &apiErr) {
		return mapStatusError(apiErr.StatusCode, err)
	}
	return mapStatusError(0, err)
}

// openaiPump 适配 OpenAI 服务端推送流
type openaiPump struct {
	stream *ssestream.Stream[openai.ChatCompletionChunk] // stream 是官方流读取器
	mapErr func(error) error                             // mapErr 将官方错误映射为本包哨兵
	once   sync.Once                                     // once 保证 close 只执行一次
}

// next 读取下一增量；跳过空文本控制帧；流结束返回 io.EOF
func (p *openaiPump) next() (*ChatChunk, error) {
	for p.stream.Next() {
		cur := p.stream.Current()
		delta := ""
		if len(cur.Choices) > 0 {
			delta = cur.Choices[0].Delta.Content
		}
		// 跳过无文本增量的控制帧
		if delta == "" && cur.ID == "" {
			continue
		}
		if delta == "" {
			continue
		}
		return &ChatChunk{ID: cur.ID, Model: cur.Model, Delta: delta}, nil
	}
	// 流结束或读失败：正常结束保持未包装结束符
	if err := p.stream.Err(); err != nil {
		if stderrors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, p.mapErr(err)
	}
	return nil, io.EOF
}

// close 关闭底层流；幂等，重复调用返回首次结果
func (p *openaiPump) close() error {
	var err error
	p.once.Do(func() {
		err = p.stream.Close()
	})
	return err
}
