// Package aiclient 定义 Jev 决策门面与请求响应结构。
//
// @author zwei
package aiclient

import (
	"context"
	"encoding/json"
	"strings"

	"gitee.com/hq_project/yomi-boot/errors"
)

// JevQuestionType Jev 问题类型
type JevQuestionType string

const (
	// JevQuestionNoul 是/否概率（0–1）
	JevQuestionNoul JevQuestionType = "noul"
	// JevQuestionChoice 从预定义选项中选一项
	JevQuestionChoice JevQuestionType = "choice"
	// JevQuestionScore 有序量表上的加权位置
	JevQuestionScore JevQuestionType = "score"
)

// JevClient 协议无关的同步 Jev 决策门面；同一实例可并发调用
type JevClient interface {
	// Decide 对 state 并行评估全部 questions，返回类型化 answers
	//
	// State 须为非空 JSON（字符串、对象或数组）；Questions 至少 1 项。
	// 模型由 JevConfig.Model 锁定。无流式。超时或取消返回上下文错误（可包装）。
	Decide(ctx context.Context, req JevDecideRequest) (*JevDecideResponse, error)
}

// JevDecideRequest 决策入参；模型由 JevConfig.Model 锁定
type JevDecideRequest struct {
	State     json.RawMessage        // State 须为非空 JSON 值
	Questions map[string]JevQuestion // Questions 至少 1 项；key 为调用方 id
}

// JevQuestion 单条类型化问题；noul 不需 Criteria
type JevQuestion struct {
	Type         JevQuestionType // Type 为 noul / choice / score
	Instructions string          // Instructions 不得为空
	Criteria     json.RawMessage // Criteria choice 为对象、score 为数组；noul 省略
}

// JevDecideResponse 决策完整结果
type JevDecideResponse struct {
	Model   string               // Model 是实际上游模型名
	Answers map[string]JevAnswer // Answers 与 Questions 的 key 对齐
	Usage   *TokenUsage          // Usage 可选；上游未给用量时为 nil
}

// JevAnswer 单题类型化答案；未出现的字段为 nil
type JevAnswer struct {
	Type          string          // Type 与问题类型一致
	Noul          *float64        // Noul 为「是」的概率
	Choice        *string         // Choice 为选中选项 key
	Score         *float64        // Score 为量表得分
	Confidence    *float64        // Confidence 为 0–1，choice/score 常见
	Legend        []string        // Legend 为 score 等级说明
	Probabilities json.RawMessage // Probabilities 为对象或数组，依题型而定
}

// validateJevDecideRequest 校验 state 与 questions；非法返回 ErrInvalidArgument
func validateJevDecideRequest(req JevDecideRequest) error {
	if len(req.State) == 0 {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if !json.Valid(req.State) {
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
		case JevQuestionNoul, JevQuestionChoice, JevQuestionScore:
		default:
			return errors.Sentinel(ErrInvalidArgument)
		}
		if strings.TrimSpace(q.Instructions) == "" {
			return errors.Sentinel(ErrInvalidArgument)
		}
	}
	return nil
}

// JevStateString 将纯文本 state 编码为 JSON 字符串值
func JevStateString(text string) json.RawMessage {
	b, _ := json.Marshal(text)
	return b
}
