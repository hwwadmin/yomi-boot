// Package rag 切块策略门面与构造。
//
// @author zwei
package rag

import (
	"strings"
	"unicode/utf8"

	"gitee.com/hq_project/yomi-boot/errors"
)

// Splitter 文本切块策略门面；实现须并发安全
type Splitter interface {
	// Split 将文本切成块
	//
	// 输入 trim 后非空时必须返回至少 1 个非空块；单块不得超过 maxChunkSize 个字符，
	// 否则 Engine 返回 ErrInvalidArgument。空文本返回 ErrInvalidArgument。
	Split(text string) ([]string, error)
}

// SplitStrategy 切块策略标识；空则 recursive
type SplitStrategy string

const (
	// StrategyRecursive 按分隔符优先级递归切分；默认策略
	StrategyRecursive SplitStrategy = "recursive"
	// StrategyFixed 按固定字符窗口切分，可能截断词语
	StrategyFixed SplitStrategy = "fixed"
	// StrategySentence 按段落与中英文句末切分后装箱
	StrategySentence SplitStrategy = "sentence"
	// StrategyMarkdown 按 Markdown 标题分节，超长小节再递归切分
	StrategyMarkdown SplitStrategy = "markdown"
)

// NewSplitter 校验切块配置并按策略构造 Splitter
func NewSplitter(cfg SplitConfig) (Splitter, error) {
	normalized, err := normalizeSplitConfig(cfg)
	if err != nil {
		return nil, err
	}
	switch normalized.Strategy {
	case StrategyFixed:
		return &fixedSplitter{chunkSize: normalized.ChunkSize, chunkOverlap: normalized.ChunkOverlap}, nil
	case StrategySentence:
		return &sentenceSplitter{chunkSize: normalized.ChunkSize, chunkOverlap: normalized.ChunkOverlap}, nil
	case StrategyMarkdown:
		return &markdownSplitter{chunkSize: normalized.ChunkSize, chunkOverlap: normalized.ChunkOverlap}, nil
	default:
		return &recursiveSplitter{
			chunkSize:    normalized.ChunkSize,
			chunkOverlap: normalized.ChunkOverlap,
			separators:   normalized.Separators,
		}, nil
	}
}

// SplitText 按配置切分文本；等价于 NewSplitter 后调用 Split
func SplitText(text string, cfg SplitConfig) ([]string, error) {
	splitter, err := NewSplitter(cfg)
	if err != nil {
		return nil, err
	}
	return splitter.Split(text)
}

// normalizeSplitConfig 填充缺省并校验切块配置
func normalizeSplitConfig(cfg SplitConfig) (SplitConfig, error) {
	out := cfg
	if out.Strategy == "" {
		out.Strategy = StrategyRecursive
	}
	switch out.Strategy {
	case StrategyRecursive, StrategyFixed, StrategySentence, StrategyMarkdown:
	default:
		return SplitConfig{}, errors.Sentinel(ErrInvalidArgument)
	}
	if out.ChunkSize == 0 {
		out.ChunkSize = defaultChunkSize
	}
	if out.ChunkSize < 0 || out.ChunkSize > maxChunkSize {
		return SplitConfig{}, errors.Sentinel(ErrInvalidArgument)
	}
	if out.ChunkOverlap < 0 {
		return SplitConfig{}, errors.Sentinel(ErrInvalidArgument)
	}
	if out.ChunkOverlap == 0 {
		out.ChunkOverlap = defaultChunkOverlap
		if auto := out.ChunkSize / 4; auto < out.ChunkOverlap {
			out.ChunkOverlap = auto
		}
	}
	if out.ChunkOverlap >= out.ChunkSize {
		return SplitConfig{}, errors.Sentinel(ErrInvalidArgument)
	}
	// 自定义分隔符仅对 recursive 有意义，其他策略传入视为配置错误
	if out.Strategy != StrategyRecursive {
		if len(out.Separators) > 0 {
			return SplitConfig{}, errors.Sentinel(ErrInvalidArgument)
		}
		out.Separators = nil
		return out, nil
	}
	if len(out.Separators) == 0 {
		out.Separators = append([]string(nil), defaultSeparators...)
		return out, nil
	}
	seps := make([]string, len(out.Separators))
	for i, sep := range out.Separators {
		if sep == "" {
			return SplitConfig{}, errors.Sentinel(ErrInvalidArgument)
		}
		seps[i] = sep
	}
	out.Separators = seps
	return out, nil
}

// normalizeText 统一换行符并去掉首尾空白
func normalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

// normalizeChunks 修剪切块输出，丢弃空白块并校验单块上限
func normalizeChunks(chunks []string) ([]string, error) {
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		s := strings.TrimSpace(chunk)
		if s == "" {
			continue
		}
		if utf8.RuneCountInString(s) > maxChunkSize {
			return nil, errors.Sentinel(ErrInvalidArgument)
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	return out, nil
}
