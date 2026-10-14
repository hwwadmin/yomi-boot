// Package aiclient System One 兼容 Jev HTTP 适配（包私有）。
//
// @author zwei
package aiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// systemoneJevClient System One 协议的 JevClient 实现
type systemoneJevClient struct {
	cfg        JevConfig
	httpClient *http.Client
	timeout    time.Duration
}

var _ JevClient = (*systemoneJevClient)(nil)

// newSystemOneJevClient 构造 System One 协议客户端
func newSystemOneJevClient(cfg JevConfig) (*systemoneJevClient, error) {
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 0}
	}
	return &systemoneJevClient{
		cfg:        cfg,
		httpClient: hc,
		timeout:    timeoutOrDefault(cfg.Timeout),
	}, nil
}

// Decide 发起同步决策请求
func (c *systemoneJevClient) Decide(ctx context.Context, req JevDecideRequest) (*JevDecideResponse, error) {
	if err := validateJevDecideRequest(req); err != nil {
		return nil, err
	}
	body, err := json.Marshal(c.buildWireRequest(req))
	if err != nil {
		return nil, wrapSentinel(ErrBadResponse, err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.APIURL, bytes.NewReader(body))
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
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, mapStatusError(0, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapStatusError(resp.StatusCode, errors.New(strings.TrimSpace(string(raw))))
	}
	return parseSystemOneResponse(raw)
}

// buildWireRequest 映射为上游 JSON 请求体
func (c *systemoneJevClient) buildWireRequest(req JevDecideRequest) systemoneDecideRequest {
	wire := systemoneDecideRequest{
		Model:     c.cfg.Model,
		State:     req.State,
		Questions: make(map[string]systemoneQuestion, len(req.Questions)),
	}
	for id, q := range req.Questions {
		wq := systemoneQuestion{
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

type systemoneDecideRequest struct {
	Model     string                       `json:"model"`
	State     json.RawMessage              `json:"state"`
	Questions map[string]systemoneQuestion `json:"questions"`
}

type systemoneQuestion struct {
	Type         string          `json:"type"`
	Instructions string          `json:"instructions"`
	Criteria     json.RawMessage `json:"criteria,omitempty"`
}

type systemoneDecideResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   *systemoneUsage            `json:"usage"`
}

type systemoneUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type systemoneAnswer struct {
	Type          string          `json:"type"`
	Noul          *float64        `json:"noul"`
	Choice        *string         `json:"choice"`
	Score         *float64        `json:"score"`
	Confidence    *float64        `json:"confidence"`
	Legend        json.RawMessage `json:"legend"`
	Probabilities json.RawMessage `json:"probabilities"`
}

// parseSystemOneResponse 解析成功响应；结构非法返回 ErrBadResponse
func parseSystemOneResponse(raw []byte) (*JevDecideResponse, error) {
	var wire systemoneDecideResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, wrapSentinel(ErrBadResponse, err)
	}
	if len(wire.Answers) == 0 {
		return nil, wrapSentinel(ErrBadResponse, errors.New("aiclient: 空 answers"))
	}
	out := &JevDecideResponse{
		Model:   wire.Model,
		Answers: make(map[string]JevAnswer, len(wire.Answers)),
	}
	for id, rawAns := range wire.Answers {
		var a systemoneAnswer
		if err := json.Unmarshal(rawAns, &a); err != nil {
			return nil, wrapSentinel(ErrBadResponse, err)
		}
		if strings.TrimSpace(a.Type) == "" {
			return nil, wrapSentinel(ErrBadResponse, errors.New("aiclient: 答案缺少 type"))
		}
		out.Answers[id] = JevAnswer{
			Type:          a.Type,
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

// mapHTTPDoErr 映射传输层错误；取消/超时保持上下文错误
func mapHTTPDoErr(err error) error {
	if err == nil {
		return nil
	}
	return mapStatusError(0, err)
}
