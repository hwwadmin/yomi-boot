// 固定窗口切块策略。
//
// @author zwei

package rag

import "gitee.com/hq_project/yomi-boot/boot/errors"

// fixedSplitter 按固定 rune 窗口切分，不识别词语或句末
type fixedSplitter struct {
	chunkSize    int
	chunkOverlap int
}

var _ Splitter = (*fixedSplitter)(nil)

// Split 按固定窗口硬切
func (s *fixedSplitter) Split(text string) ([]string, error) {
	src := normalizeText(text)
	if src == "" {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	return normalizeChunks(hardSplit(src, s.chunkSize, s.chunkOverlap))
}
