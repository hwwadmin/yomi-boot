// Package aiclient Anthropic Messages 适配（包私有）。
//
// @author zwei
package aiclient

import (
	"context"
	stderrors "errors"
	"io"
	"strings"
	"sync"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

// anthropicClient Anthropic 协议的 AIClient 实现
type anthropicClient struct {
	sdk     anthropic.Client // sdk 是官方客户端，选项不可变，可并发调用
	cfg     Config           // cfg 是构造期拷贝后的配置
	timeout time.Duration    // timeout 是整段墙钟超时（0 已回退为 60s）
}

var _ AIClient = (*anthropicClient)(nil)

// newAnthropicClient 构造 Anthropic 协议客户端
//
// 官方客户端仅持有不可变选项，每次请求独立执行，同一实例可并发 Chat / ChatStream。
func newAnthropicClient(cfg Config) (*anthropicClient, error) {
	// 密钥与地址原样注入；关闭官方默认重试，并写入协议版本头
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.APIURL),
		option.WithMaxRetries(0),
		option.WithRequestTimeout(timeoutOrDefault(cfg.Timeout)),
		option.WithHeader("anthropic-version", cfg.anthropicVersionOrDefault()),
	}
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
	}
	for k, v := range cfg.ExtraHeaders {
		opts = append(opts, option.WithHeader(k, v))
	}
	return &anthropicClient{
		sdk:     anthropic.NewClient(opts...),
		cfg:     cfg,
		timeout: timeoutOrDefault(cfg.Timeout),
	}, nil
}

// Chat 发起同步对话；空响应或无助手文本返回 ErrBadResponse
func (c *anthropicClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	params, err := c.buildParams(req)
	if err != nil {
		return nil, err
	}
	// 整段请求绑定墙钟超时
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	resp, err := c.sdk.Messages.New(ctx, params)
	if err != nil {
		return nil, c.mapErr(err)
	}
	// 空响应视为协议异常
	if resp == nil {
		return nil, wrapSentinel(ErrBadResponse, errors.New("aiclient: 空响应"))
	}
	text := collectAnthropicText(resp.Content)
	out := &ChatResponse{
		ID:    resp.ID,
		Model: string(resp.Model),
		Message: Message{
			Role:    RoleAssistant,
			Content: text,
		},
		Usage: anthropicUsage(resp.Usage.InputTokens, resp.Usage.OutputTokens),
	}
	// 无助手文本视为协议异常
	if text == "" {
		return nil, wrapSentinel(ErrBadResponse, errors.New("aiclient: 无 assistant 文本"))
	}
	return out, nil
}

// ChatStream 开启流式对话；建连失败时关闭底层流并返回错误，不返回半开流
func (c *anthropicClient) ChatStream(ctx context.Context, req ChatRequest) (ChatStream, error) {
	params, err := c.buildParams(req)
	if err != nil {
		return nil, err
	}
	// 流式请求绑定墙钟超时，由返回的流负责释放
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	stream := c.sdk.Messages.NewStreaming(ctx, params)
	// 建连失败关闭底层流，不把半开流交给调用方
	if err := stream.Err(); err != nil {
		cancel()
		_ = stream.Close()
		return nil, c.mapErr(err)
	}
	return newChatStream(ctx, cancel, &anthropicPump{stream: stream, mapErr: c.mapErr}), nil
}

// buildParams 映射请求并校验 Anthropic 轮次；MaxTokens 请求覆盖配置缺省
func (c *anthropicClient) buildParams(req ChatRequest) (anthropic.MessageNewParams, error) {
	if err := validateChatRequest(req); err != nil {
		return anthropic.MessageNewParams{}, err
	}
	// 抽掉系统提示后的轮次必须由用户开头且严格交替
	if err := validateAnthropicTurns(req.Messages); err != nil {
		return anthropic.MessageNewParams{}, err
	}
	var msgs []anthropic.MessageParam
	for _, m := range req.Messages {
		switch m.Role {
		case RoleUser:
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case RoleAssistant:
			msgs = append(msgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
		}
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.cfg.Model),
		Messages:  msgs,
		MaxTokens: int64(defaultMaxTokens),
	}
	// 系统提示不进入轮次，按出现顺序拼到系统字段
	if sys := joinSystemPrompts(req.Messages); sys != "" {
		params.System = []anthropic.TextBlockParam{{Text: sys}}
	}
	if temp := resolveTemperature(req.Temperature, c.cfg.Temperature); temp != nil {
		params.Temperature = anthropic.Float(*temp)
	}
	if maxTok := resolveMaxTokens(req.MaxTokens, c.cfg.MaxTokens); maxTok != nil {
		params.MaxTokens = int64(*maxTok)
	}
	return params, nil
}

// collectAnthropicText 拼接文本类型内容块；无文本块时返回空串
func collectAnthropicText(blocks []anthropic.ContentBlockUnion) string {
	var b strings.Builder
	for _, block := range blocks {
		// 仅拼接文本块，忽略其它内容类型
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

// anthropicUsage 用量全 0 时返回空，与 OpenAI 同步口径对齐
func anthropicUsage(in, out int64) *TokenUsage {
	if in <= 0 && out <= 0 {
		return nil
	}
	return &TokenUsage{
		PromptTokens:     int(in),
		CompletionTokens: int(out),
		TotalTokens:      int(in + out),
	}
}

// mapErr 映射 Anthropic 官方客户端错误；能取出 HTTP 状态时走哨兵，取消或超时保持上下文错误
func (c *anthropicClient) mapErr(err error) error {
	if err == nil {
		return nil
	}
	// 取消或超时保持标准库错误，便于按错误链判断
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return wrapCtxErr(err)
	}
	var apiErr *anthropic.Error
	if stderrors.As(err, &apiErr) {
		return mapStatusError(apiErr.StatusCode, err)
	}
	return mapStatusError(0, err)
}

// anthropicPump 适配 Anthropic 服务端推送流
type anthropicPump struct {
	stream *ssestream.Stream[anthropic.MessageStreamEventUnion] // stream 是官方流读取器
	mapErr func(error) error                                    // mapErr 将官方错误映射为本包哨兵
	once   sync.Once                                            // once 保证 close 只执行一次
	id     string                                               // id 来自 message_start
	model  string                                               // model 来自 message_start
}

// next 读取文本增量；先记下会话标识与模型名再透出增量
func (p *anthropicPump) next() (*ChatChunk, error) {
	for p.stream.Next() {
		evt := p.stream.Current()
		switch v := evt.AsAny().(type) {
		case anthropic.MessageStartEvent:
			p.id = v.Message.ID
			p.model = string(v.Message.Model)
			continue
		case anthropic.ContentBlockDeltaEvent:
			delta, ok := v.Delta.AsAny().(anthropic.TextDelta)
			// 仅透出文本增量，其它增量类型跳过
			if !ok || delta.Text == "" {
				continue
			}
			return &ChatChunk{ID: p.id, Model: p.model, Delta: delta.Text}, nil
		default:
			continue
		}
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
func (p *anthropicPump) close() error {
	var err error
	p.once.Do(func() {
		err = p.stream.Close()
	})
	return err
}
