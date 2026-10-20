// 定义 Jev 决策门面与请求响应结构。
//
// @author zwei

package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"
)

// QuestionType Jev 问题类型
type QuestionType string

const (
	// QuestionNoul 是/否概率（0–1）
	QuestionNoul QuestionType = "noul"
	// QuestionChoice 从预定义选项中选一项
	QuestionChoice QuestionType = "choice"
	// QuestionScore 有序量表上的加权位置
	QuestionScore QuestionType = "score"
)

// Client 协议无关的同步 Jev 决策门面；同一实例可并发调用
type Client interface {
	// Decide 对 state 并行评估全部 questions，返回类型化 answers
	//
	// State 须为非空 JSON（字符串、对象或数组）；Questions 至少 1 项。
	// 模型由 Config.Model 锁定。无流式。超时或取消返回上下文错误（可包装）。
	Decide(ctx context.Context, req DecideRequest) (*DecideResponse, error)
}

// DecideRequest 决策入参；模型由 Config.Model 锁定
type DecideRequest struct {
	State     json.RawMessage     // State 须为非空 JSON 值
	Questions map[string]Question // Questions 至少 1 项；key 为调用方 id
}

// Question 单条类型化问题；noul 不得携带 Criteria
type Question struct {
	Type         QuestionType    // Type 为 noul / choice / score
	Instructions string          // Instructions 不得为空
	Criteria     json.RawMessage // Criteria choice 为对象、score 为数组；noul 不得携带
}

// DecideResponse 决策完整结果
type DecideResponse struct {
	Model   string            // Model 是实际上游模型名
	Answers map[string]Answer // Answers 与 Questions 的 key 对齐
	Usage   *TokenUsage       // Usage 可选；上游未给用量时为 nil
}

// TokenUsage 用量统计；可选
type TokenUsage struct {
	PromptTokens     int // PromptTokens 是输入令牌数
	CompletionTokens int // CompletionTokens 是输出令牌数
	TotalTokens      int // TotalTokens 是合计令牌数
}

// Answer 单题类型化答案；未出现的字段为 nil
type Answer struct {
	Type          QuestionType    // Type 与问题类型一致；解析时校验
	Noul          *float64        // Noul 为「是」的概率
	Choice        *string         // Choice 为选中选项 key
	Score         *float64        // Score 为量表得分
	Confidence    *float64        // Confidence 为 0–1，choice/score 常见
	Legend        json.RawMessage // Legend 为 score 等级说明（数组或对象，依上游而定）
	Probabilities json.RawMessage // Probabilities 为对象或数组，依题型而定
}

// validateDecideRequest 校验 state、questions 与 criteria；非法返回 ErrInvalidArgument
func validateDecideRequest(req DecideRequest) error {
	state := bytes.TrimSpace(req.State)
	if len(state) == 0 || !json.Valid(state) || bytes.Equal(state, []byte("null")) {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if len(req.Questions) == 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	for id, q := range req.Questions {
		if strings.TrimSpace(id) == "" {
			return errors.Sentinel(ErrInvalidArgument)
		}
		switch q.Type {
		case QuestionNoul, QuestionChoice, QuestionScore:
		default:
			return errors.Sentinel(ErrInvalidArgument)
		}
		if strings.TrimSpace(q.Instructions) == "" {
			return errors.Sentinel(ErrInvalidArgument)
		}
		criteria := bytes.TrimSpace(q.Criteria)
		// noul 不得携带 Criteria；choice/score 可选，携带时必须是约定形状
		if q.Type == QuestionNoul {
			if len(criteria) > 0 {
				return errors.Sentinel(ErrInvalidArgument)
			}
		} else if len(criteria) > 0 {
			if !json.Valid(criteria) {
				return errors.Sentinel(ErrInvalidArgument)
			}
			if q.Type == QuestionChoice && criteria[0] != '{' {
				return errors.Sentinel(ErrInvalidArgument)
			}
			if q.Type == QuestionScore && criteria[0] != '[' {
				return errors.Sentinel(ErrInvalidArgument)
			}
		}
	}
	return nil
}

// StateString 将纯文本 state 编码为 JSON 字符串值
func StateString(text string) json.RawMessage {
	b, _ := json.Marshal(text)
	return b
}
