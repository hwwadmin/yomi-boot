// Markdown 标题切块策略。
//
// @author zwei

package rag

import (
	"strings"
	"unicode/utf8"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// markdownSplitter 按 Markdown 标题分节，超长小节再递归切分
type markdownSplitter struct {
	chunkSize       int
	chunkOverlap    int
	maxHeadingLevel int
}

var _ Splitter = (*markdownSplitter)(nil)

// Split 按标题分节；每节独立成块，超长节再按分隔符递归切分
func (s *markdownSplitter) Split(text string) ([]string, error) {
	src := normalizeText(text)
	if src == "" {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	sections := splitMarkdownSections(src, s.maxHeadingLevel)
	out := make([]string, 0, len(sections))
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		if utf8.RuneCountInString(section) <= s.chunkSize {
			out = append(out, section)
			continue
		}
		segments := splitSegments(section, defaultSeparators, s.chunkSize, s.chunkOverlap)
		out = append(out, packSegments(segments, s.chunkSize, s.chunkOverlap)...)
	}
	return out, nil
}

// splitMarkdownSections 按标题行分节；围栏代码块内的 # 不视为标题
func splitMarkdownSections(text string, maxHeadingLevel int) []string {
	lines := strings.Split(text, "\n")
	sections := make([]string, 0, 4)
	current := make([]string, 0, len(lines))
	inFence := false
	fence := ""
	for _, line := range lines {
		marker, fenceLine := markdownFence(line)
		if fenceLine {
			if !inFence {
				inFence = true
				fence = marker
			} else if marker == fence {
				inFence = false
				fence = ""
			}
		}
		if !inFence && !fenceLine && isMarkdownHeading(line, maxHeadingLevel) && len(current) > 0 {
			sections = append(sections, strings.Join(current, "\n"))
			current = current[:0]
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		sections = append(sections, strings.Join(current, "\n"))
	}
	return sections
}

// markdownFence 识别围栏代码块行；返回围栏标记与是否为围栏行
func markdownFence(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return "", false
	}
	if strings.HasPrefix(trimmed, "```") {
		return "```", true
	}
	if strings.HasPrefix(trimmed, "~~~") {
		return "~~~", true
	}
	return "", false
}

// isMarkdownHeading 判断是否为 1..maxHeadingLevel 级 Markdown 标题行
func isMarkdownHeading(line string, maxHeadingLevel int) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > maxHeadingLevel {
		return false
	}
	if level == len(trimmed) {
		return true
	}
	return trimmed[level] == ' ' || trimmed[level] == '\t'
}
