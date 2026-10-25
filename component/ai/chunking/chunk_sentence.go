// 句子切块策略。
//
// @author zwei

package chunking

import "gitee.com/hq_project/yomi-boot/component/core/errors"

// sentenceSeparators 句子策略的默认分隔符；只认段落、行与句末标点
var sentenceSeparators = []string{
	"\n\n", "\n",
	"。", "！", "？", "；", "：", "……",
	". ", "! ", "? ", "; ", ": ",
}

// sentenceChunker 按句子边界切分后装箱
type sentenceChunker struct {
	chunkSize    int
	chunkOverlap int
	separators   []string
}

var _ Chunker = (*sentenceChunker)(nil)

// Chunk 按句子边界切分并装箱
func (s *sentenceChunker) Chunk(text string) ([]string, error) {
	src := normalizeText(text)
	if src == "" {
		return nil, errors.Sentinel(ErrInvalidArgument)
	}
	segments := chunkSegments(src, s.separators, s.chunkSize, s.chunkOverlap)
	return packSegments(segments, s.chunkSize, s.chunkOverlap), nil
}
