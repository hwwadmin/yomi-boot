// 切块策略门面与构造。
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
	// StrategySentence 按段落与句末分隔符切分后装箱
	StrategySentence SplitStrategy = "sentence"
	// StrategyMarkdown 按 Markdown 标题分节，超长小节再递归切分
	StrategyMarkdown SplitStrategy = "markdown"
)

// NewSplitter 校验配置并按 Strategy 构造 Splitter；只有对应策略的子配置生效
func NewSplitter(cfg SplitConfig) (Splitter, error) {
	strategy, err := normalizeStrategy(cfg.Strategy)
	if err != nil {
		return nil, err
	}
	switch strategy {
	case StrategyFixed:
		fixed, err := normalizeFixedConfig(cfg.Fixed)
		if err != nil {
			return nil, err
		}
		return &fixedSplitter{chunkSize: fixed.ChunkSize, chunkOverlap: fixed.ChunkOverlap}, nil
	case StrategySentence:
		sentence, err := normalizeSentenceConfig(cfg.Sentence)
		if err != nil {
			return nil, err
		}
		return &sentenceSplitter{
			chunkSize:    sentence.ChunkSize,
			chunkOverlap: sentence.ChunkOverlap,
			separators:   sentence.Separators,
		}, nil
	case StrategyMarkdown:
		markdown, err := normalizeMarkdownConfig(cfg.Markdown)
		if err != nil {
			return nil, err
		}
		return &markdownSplitter{
			chunkSize:       markdown.ChunkSize,
			chunkOverlap:    markdown.ChunkOverlap,
			maxHeadingLevel: markdown.MaxHeadingLevel,
		}, nil
	default:
		recursive, err := normalizeRecursiveConfig(cfg.Recursive)
		if err != nil {
			return nil, err
		}
		return &recursiveSplitter{
			chunkSize:    recursive.ChunkSize,
			chunkOverlap: recursive.ChunkOverlap,
			separators:   recursive.Separators,
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

// normalizeStrategy 空策略回退 recursive，并校验白名单
func normalizeStrategy(strategy SplitStrategy) (SplitStrategy, error) {
	if strategy == "" {
		return StrategyRecursive, nil
	}
	switch strategy {
	case StrategyRecursive, StrategyFixed, StrategySentence, StrategyMarkdown:
		return strategy, nil
	default:
		return "", errors.Sentinel(ErrInvalidArgument)
	}
}

// normalizeChunkConfig 填充块长与重叠缺省，并校验范围
func normalizeChunkConfig(chunkSize, chunkOverlap int) (int, int, error) {
	if chunkSize == 0 {
		chunkSize = defaultChunkSize
	}
	if chunkSize < 0 || chunkSize > maxChunkSize {
		return 0, 0, errors.Sentinel(ErrInvalidArgument)
	}
	if chunkOverlap < 0 {
		return 0, 0, errors.Sentinel(ErrInvalidArgument)
	}
	if chunkOverlap == 0 {
		chunkOverlap = defaultChunkOverlap
		if auto := chunkSize / 4; auto < chunkOverlap {
			chunkOverlap = auto
		}
	}
	if chunkOverlap >= chunkSize {
		return 0, 0, errors.Sentinel(ErrInvalidArgument)
	}
	return chunkSize, chunkOverlap, nil
}

// normalizeSeparators 填充分隔符缺省并校验元素非空
func normalizeSeparators(separators, defaults []string) ([]string, error) {
	if len(separators) == 0 {
		return append([]string(nil), defaults...), nil
	}
	out := make([]string, len(separators))
	for i, sep := range separators {
		if sep == "" {
			return nil, errors.Sentinel(ErrInvalidArgument)
		}
		out[i] = sep
	}
	return out, nil
}

// normalizeRecursiveConfig 填充并校验 recursive 配置
func normalizeRecursiveConfig(cfg RecursiveConfig) (RecursiveConfig, error) {
	size, overlap, err := normalizeChunkConfig(cfg.ChunkSize, cfg.ChunkOverlap)
	if err != nil {
		return RecursiveConfig{}, err
	}
	separators, err := normalizeSeparators(cfg.Separators, defaultSeparators)
	if err != nil {
		return RecursiveConfig{}, err
	}
	return RecursiveConfig{ChunkSize: size, ChunkOverlap: overlap, Separators: separators}, nil
}

// normalizeFixedConfig 填充并校验 fixed 配置
func normalizeFixedConfig(cfg FixedConfig) (FixedConfig, error) {
	size, overlap, err := normalizeChunkConfig(cfg.ChunkSize, cfg.ChunkOverlap)
	if err != nil {
		return FixedConfig{}, err
	}
	return FixedConfig{ChunkSize: size, ChunkOverlap: overlap}, nil
}

// normalizeSentenceConfig 填充并校验 sentence 配置
func normalizeSentenceConfig(cfg SentenceConfig) (SentenceConfig, error) {
	size, overlap, err := normalizeChunkConfig(cfg.ChunkSize, cfg.ChunkOverlap)
	if err != nil {
		return SentenceConfig{}, err
	}
	separators, err := normalizeSeparators(cfg.Separators, sentenceSeparators)
	if err != nil {
		return SentenceConfig{}, err
	}
	return SentenceConfig{ChunkSize: size, ChunkOverlap: overlap, Separators: separators}, nil
}

// normalizeMarkdownConfig 填充并校验 markdown 配置
func normalizeMarkdownConfig(cfg MarkdownConfig) (MarkdownConfig, error) {
	size, overlap, err := normalizeChunkConfig(cfg.ChunkSize, cfg.ChunkOverlap)
	if err != nil {
		return MarkdownConfig{}, err
	}
	level := cfg.MaxHeadingLevel
	if level == 0 {
		level = 6
	}
	if level < 1 || level > 6 {
		return MarkdownConfig{}, errors.Sentinel(ErrInvalidArgument)
	}
	return MarkdownConfig{ChunkSize: size, ChunkOverlap: overlap, MaxHeadingLevel: level}, nil
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
