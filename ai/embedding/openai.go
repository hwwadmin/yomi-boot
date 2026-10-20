// OpenAI Embeddings 适配（包私有）。
//
// @author zwei

package embedding

import (
	"context"
	stderrors "errors"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// maxEmbedBatch 是单次上游请求的输入条数上限（OpenAI Embeddings）
const maxEmbedBatch = 2048

// openaiClient OpenAI 协议的 Client 实现
type openaiClient struct {
	sdk     openai.Client // sdk 是官方客户端，选项不可变，可并发调用
	cfg     Config        // cfg 是构造期拷贝后的配置
	timeout time.Duration // timeout 是整段墙钟超时（0 已回退为 60s）
}

var _ Client = (*openaiClient)(nil)

// newOpenAIClient 构造 OpenAI 协议向量化客户端
//
// 官方客户端仅持有不可变选项，每次请求独立执行，同一实例可并发 Embed。
func newOpenAIClient(cfg Config) (*openaiClient, error) {
	// 地址原样注入；关闭官方默认重试
	opts := []option.RequestOption{
		option.WithBaseURL(cfg.APIURL),
		option.WithMaxRetries(0),
		option.WithRequestTimeout(timeoutOrDefault(cfg.Timeout)),
	}
	// 密钥可空：空也注入，避免官方客户端回落到环境变量；空则不带 Authorization
	opts = append(opts, option.WithAPIKey(cfg.APIKey))
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

// Embed 发起同步向量化；超过 2048 条时内部自动分批；空结果或下标不齐返回 ErrBadResponse
func (c *openaiClient) Embed(ctx context.Context, req Request) (*Response, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	// 整段请求绑定墙钟超时
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	vectors := make([][]float64, len(req.Inputs))
	model := ""
	var promptTokens, totalTokens int64
	for start := 0; start < len(req.Inputs); start += maxEmbedBatch {
		end := start + maxEmbedBatch
		if end > len(req.Inputs) {
			end = len(req.Inputs)
		}
		params, err := c.buildParams(Request{Inputs: req.Inputs[start:end], Dimensions: req.Dimensions})
		if err != nil {
			return nil, err
		}
		resp, err := c.sdk.Embeddings.New(ctx, params)
		if err != nil {
			return nil, mapOpenAIError(err)
		}
		if resp == nil {
			return nil, wrapSentinel(ErrBadResponse, errors.New("ai/embedding: 空响应"))
		}
		batchVectors, err := collectEmbeddingVectors(end-start, resp.Data)
		if err != nil {
			return nil, err
		}
		copy(vectors[start:end], batchVectors)
		model = resp.Model
		promptTokens += resp.Usage.PromptTokens
		totalTokens += resp.Usage.TotalTokens
	}
	return &Response{
		Model:   model,
		Vectors: vectors,
		Usage:   embeddingUsage(promptTokens, totalTokens),
	}, nil
}

// buildParams 将门面请求映射为官方客户端参数；入参非法返回 ErrInvalidArgument
func (c *openaiClient) buildParams(req Request) (openai.EmbeddingNewParams, error) {
	if err := validateRequest(req); err != nil {
		return openai.EmbeddingNewParams{}, err
	}
	params := openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(c.cfg.Model),
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: req.Inputs,
		},
	}
	if dim := resolveDimensions(req.Dimensions, c.cfg.Dimensions); dim != nil {
		params.Dimensions = openai.Int(int64(*dim))
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

// collectEmbeddingVectors 按上游 index 填回与输入等长的向量列表
func collectEmbeddingVectors(n int, data []openai.Embedding) ([][]float64, error) {
	if n <= 0 || len(data) == 0 {
		return nil, wrapSentinel(ErrBadResponse, errors.New("ai/embedding: 空 embeddings"))
	}
	out := make([][]float64, n)
	filled := make([]bool, n)
	for _, item := range data {
		i := int(item.Index)
		if i < 0 || i >= n || filled[i] || len(item.Embedding) == 0 {
			return nil, wrapSentinel(ErrBadResponse, errors.New("ai/embedding: embeddings 下标或向量异常"))
		}
		out[i] = item.Embedding
		filled[i] = true
	}
	for _, ok := range filled {
		if !ok {
			return nil, wrapSentinel(ErrBadResponse, errors.New("ai/embedding: embeddings 条数不齐"))
		}
	}
	return out, nil
}

// embeddingUsage 用量全 0 时返回空；向量化无 completion
func embeddingUsage(prompt, total int64) *TokenUsage {
	if prompt <= 0 && total <= 0 {
		return nil
	}
	return &TokenUsage{
		PromptTokens: int(prompt),
		TotalTokens:  int(total),
	}
}
