// 固定窗口切块策略。
//
// @author zwei

package chunking

import "gitee.com/hq_project/yomi-boot/component/core/errors"

// fixedChunker 按固定 rune 窗口切分，不识别词语或句末
type fixedChunker struct {
	chunkSize    int
	chunkOverlap int
}

var _ Chunker = (*fixedChunker)(nil)

// Chunk 按固定窗口硬切
func (s *fixedChunker) Chunk(text string) ([]string, error) {
	src := normalizeText(text)
	if src == "" {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	return NormalizeChunks(hardChunk(src, s.chunkSize, s.chunkOverlap))
}
