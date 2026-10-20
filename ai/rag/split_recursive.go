// Package rag 递归切块策略。
//
// @author zwei
package rag

import (
	"strings"
	"unicode/utf8"

	"gitee.com/hq_project/yomi-boot/errors"
)

// defaultSeparators 默认分隔符；按段落、行、中英文句末、分句与空格逐级回退
var defaultSeparators = []string{
	"\n\n", "\n",
	"。", "！", "？", "；", "：",
	". ", "! ", "? ", "; ", ": ",
	"，", ", ", " ",
}

// recursiveSplitter 按分隔符优先级递归切分
type recursiveSplitter struct {
	chunkSize    int
	chunkOverlap int
	separators   []string
}

var _ Splitter = (*recursiveSplitter)(nil)

// Split 按分隔符优先级递归切分并装箱
func (s *recursiveSplitter) Split(text string) ([]string, error) {
	src := normalizeText(text)
	if src == "" {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	segments := splitSegments(src, s.separators, s.chunkSize, s.chunkOverlap)
	return normalizeChunks(packSegments(segments, s.chunkSize, s.chunkOverlap))
}

// splitSegments 递归按分隔符切出不超过 max 的原子段
func splitSegments(text string, seps []string, max, overlap int) []string {
	if text == "" {
		return nil
	}
	if utf8.RuneCountInString(text) <= max {
		return []string{text}
	}
	for i, sep := range seps {
		if !strings.Contains(text, sep) {
			continue
		}
		parts := strings.SplitAfter(text, sep)
		// 分隔符只在末尾时没有真正切开，继续尝试下一级分隔符
		nonEmpty := 0
		for _, part := range parts {
			if part != "" {
				nonEmpty++
			}
		}
		if nonEmpty <= 1 {
			continue
		}
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part == "" {
				continue
			}
			if utf8.RuneCountInString(part) <= max {
				out = append(out, part)
				continue
			}
			out = append(out, splitSegments(part, seps[i+1:], max, overlap)...)
		}
		return out
	}
	return hardSplit(text, max, overlap)
}

// hardSplit 无分隔符时按 rune 硬切；窗口间自带 overlap 个 rune 的重叠
func hardSplit(text string, max, overlap int) []string {
	runes := []rune(text)
	step := max - overlap
	if step <= 0 {
		step = max
	}
	out := make([]string, 0, len(runes)/step+1)
	for start := 0; start < len(runes); start += step {
		end := start + max
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[start:end]))
	}
	return out
}

// packSegments 将原子段贪心装箱，相邻块保留 overlap 个 rune 的重叠
func packSegments(segments []string, max, overlap int) []string {
	chunks := make([]string, 0, len(segments))
	current := make([]rune, 0, max)
	for _, seg := range segments {
		segRunes := []rune(seg)
		if len(segRunes) == 0 {
			continue
		}
		if len(current) > 0 && len(current)+len(segRunes) > max {
			if s := strings.TrimSpace(string(current)); s != "" {
				chunks = append(chunks, s)
			}
			current = tailRunes(current, overlap)
		}
		// 重叠加新段仍超长时截短重叠，保证块长不超过上限
		if len(current)+len(segRunes) > max {
			keep := max - len(segRunes)
			if keep < 0 {
				keep = 0
			}
			if len(current) > keep {
				current = current[len(current)-keep:]
			}
		}
		current = append(current, segRunes...)
	}
	if s := strings.TrimSpace(string(current)); s != "" {
		chunks = append(chunks, s)
	}
	return chunks
}

// tailRunes 返回末尾 n 个 rune 的副本
func tailRunes(src []rune, n int) []rune {
	if n <= 0 || len(src) == 0 {
		return nil
	}
	if n > len(src) {
		n = len(src)
	}
	out := make([]rune, n)
	copy(out, src[len(src)-n:])
	return out
}
