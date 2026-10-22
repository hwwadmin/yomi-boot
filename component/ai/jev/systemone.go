// System One 兼容 HTTP 适配（包私有）。
//
// @author zwei

package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	// maxResponseBytes 是单次响应体读取上限
	maxResponseBytes = 8 << 20
	// maxErrorBodyRunes 是非 2xx 错误正文进入错误链的最大字符数
	maxErrorBodyRunes = 512
)

// systemOneClient System One 协议的 Client 实现
type systemOneClient struct {
	cfg        Config
	httpClient *http.Client
	timeout    time.Duration
}

var _ Client = (*systemOneClient)(nil)

// newSystemOneClient 构造 System One 协议客户端
func newSystemOneClient(cfg Config) *systemOneClient {
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 0}
	}
	return &systemOneClient{
		cfg:        cfg,
		httpClient: hc,
		timeout:    timeoutOrDefault(cfg.Timeout),
	}
}

// Decide 发起同步决策请求
func (c *systemOneClient) Decide(ctx context.Context, req DecideRequest) (*DecideResponse, error) {
	if err := validateDecideRequest(req); err != nil {
		return nil, err
	}
	body, err := json.Marshal(c.buildWireRequest(req))
	if err != nil {
		return nil, wrapSentinel(ErrBadResponse, err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, wrapSentinel(ErrBadResponse, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	for k, v := range c.cfg.ExtraHeaders {
		httpReq.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, mapHTTPDoErr(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, mapStatusError(0, err)
	}
	if len(raw) > maxResponseBytes {
		return nil, wrapSentinel(ErrBadResponse, errors.New("ai/jev: 响应体超过 8MB"))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapStatusError(resp.StatusCode, errors.New(formatHTTPErrorBody(resp.StatusCode, raw)))
	}
	return parseSystemOneResponse(raw, req.Questions)
}

// buildWireRequest 映射为上游 JSON 请求体
func (c *systemOneClient) buildWireRequest(req DecideRequest) systemOneDecideRequest {
	wire := systemOneDecideRequest{
		Model:     c.cfg.Model,
		State:     req.State,
		Questions: make(map[string]systemOneQuestion, len(req.Questions)),
	}
	for id, q := range req.Questions {
		wq := systemOneQuestion{
			Type:         string(q.Type),
			Instructions: q.Instructions,
		}
		if len(q.Criteria) > 0 {
			wq.Criteria = q.Criteria
		}
		wire.Questions[id] = wq
	}
	return wire
}

type systemOneDecideRequest struct {
	Model     string                       `json:"model"`
	State     json.RawMessage              `json:"state"`
	Questions map[string]systemOneQuestion `json:"questions"`
}

type systemOneQuestion struct {
	Type         string          `json:"type"`
	Instructions string          `json:"instructions"`
	Criteria     json.RawMessage `json:"criteria,omitempty"`
}

type systemOneDecideResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   *systemOneUsage            `json:"usage"`
}

type systemOneUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type systemOneAnswer struct {
	Type          string          `json:"type"`
	Noul          *float64        `json:"noul"`
	Choice        *string         `json:"choice"`
	Score         *float64        `json:"score"`
	Confidence    *float64        `json:"confidence"`
	Legend        json.RawMessage `json:"legend"`
	Probabilities json.RawMessage `json:"probabilities"`
}

// parseSystemOneResponse 解析成功响应；缺失答案、类型不符或结构非法返回 ErrBadResponse
func parseSystemOneResponse(raw []byte, questions map[string]Question) (*DecideResponse, error) {
	var wire systemOneDecideResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, wrapSentinel(ErrBadResponse, err)
	}
	if len(wire.Answers) == 0 {
		return nil, wrapSentinel(ErrBadResponse, errors.New("ai/jev: 空 answers"))
	}
	// 请求中的每个问题都必须有答案；缺答时不能让调用方拿到零值 Answer
	for id := range questions {
		if _, ok := wire.Answers[id]; !ok {
			return nil, wrapSentinel(ErrBadResponse, errors.Newf("ai/jev: 缺少问题 %q 的答案", id))
		}
	}
	out := &DecideResponse{
		Model:   wire.Model,
		Answers: make(map[string]Answer, len(questions)),
	}
	for id, rawAns := range wire.Answers {
		q, ok := questions[id]
		// 未请求的答案不进入结果，避免调用方收到意外 key
		if !ok {
			continue
		}
		var a systemOneAnswer
		if err := json.Unmarshal(rawAns, &a); err != nil {
			return nil, wrapSentinel(ErrBadResponse, err)
		}
		if strings.TrimSpace(a.Type) == "" {
			return nil, wrapSentinel(ErrBadResponse, errors.New("ai/jev: 答案缺少 type"))
		}
		answerType := QuestionType(a.Type)
		// 答案类型必须与请求的问题类型一致
		if answerType != q.Type {
			return nil, wrapSentinel(ErrBadResponse, errors.Newf("ai/jev: 问题 %q 的答案类型 %q 与请求类型 %q 不一致", id, a.Type, q.Type))
		}
		out.Answers[id] = Answer{
			Type:          answerType,
			Noul:          a.Noul,
			Choice:        a.Choice,
			Score:         a.Score,
			Confidence:    a.Confidence,
			Legend:        a.Legend,
			Probabilities: a.Probabilities,
		}
	}
	if wire.Usage != nil && (wire.Usage.InputTokens > 0 || wire.Usage.OutputTokens > 0) {
		out.Usage = &TokenUsage{
			PromptTokens:     wire.Usage.InputTokens,
			CompletionTokens: wire.Usage.OutputTokens,
			TotalTokens:      wire.Usage.InputTokens + wire.Usage.OutputTokens,
		}
	}
	return out, nil
}

// formatHTTPErrorBody 生成非 2xx 错误正文；空正文回退状态码，超长按 rune 截断
func formatHTTPErrorBody(status int, raw []byte) string {
	body := strings.TrimSpace(string(raw))
	if body == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	if offset, ok := runeCutOffset(body, maxErrorBodyRunes); ok {
		return body[:offset] + fmt.Sprintf("（已截断，原文 %d 字节）", len(raw))
	}
	return body
}

// runeCutOffset 返回第 n 个 rune 之前的字节偏移；不足 n 个 rune 时返回 false
func runeCutOffset(s string, n int) (int, bool) {
	count := 0
	for offset := range s {
		if count == n {
			return offset, true
		}
		count++
	}
	return 0, false
}

// mapHTTPDoErr 映射传输层错误；取消/超时保持上下文错误
func mapHTTPDoErr(err error) error {
	if err == nil {
		return nil
	}
	return mapStatusError(0, err)
}
