// memory 实现的 BM25 词法评分与 RRF 融合。
//
// @author zwei

package vectordb

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// BM25 参数；与 Milvus 默认一致
const (
	bm25K1 = 1.2
	bm25B  = 0.75
	// rrfK 是 RRF 融合常数；与 milvusclient.NewRRFReranker 默认值一致
	rrfK = 60
)

// tokenizeText 切词：ASCII 字母数字连串为词（小写）；CJK 等 非 ASCII 字母串切单字与相邻二元词组
// （单字保召回、二元保短语精度，字符重合的文档得分低于完整含词的文档）；其余为分隔符
func tokenizeText(text string) []string {
	var tokens []string
	var sb strings.Builder
	var cjk []rune
	flushWord := func() {
		if sb.Len() > 0 {
			tokens = append(tokens, sb.String())
			sb.Reset()
		}
	}
	flushCJK := func() {
		if len(cjk) == 0 {
			return
		}
		for _, r := range cjk {
			tokens = append(tokens, string(r))
		}
		for i := 0; i+1 < len(cjk); i++ {
			tokens = append(tokens, string(cjk[i:i+2]))
		}
		cjk = cjk[:0]
	}
	for _, r := range text {
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			flushCJK()
			sb.WriteRune(unicode.ToLower(r))
		case r >= 128 && unicode.IsLetter(r):
			flushWord()
			cjk = append(cjk, r)
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return tokens
}

// termFreq 统计词频
func termFreq(tokens []string) map[string]int {
	if len(tokens) == 0 {
		return nil
	}
	tf := make(map[string]int, len(tokens))
	for _, tok := range tokens {
		tf[tok]++
	}
	return tf
}

// bm25Scorer 对一组文档预计算 BM25 语料统计后逐条评分
type bm25Scorer struct {
	docs  []map[string]int // docs 是各文档词频
	dls   []int            // dls 是各文档词数
	avgdl float64          // avgdl 是平均文档长度（按词数）
	idf   map[string]float64
}

// newBM25Scorer 按语料构建评分器；texts 为空语料时所有查询得 0 分
func newBM25Scorer(texts []string) *bm25Scorer {
	s := &bm25Scorer{
		docs: make([]map[string]int, len(texts)),
		dls:  make([]int, len(texts)),
		idf:  make(map[string]float64),
	}
	totalLen := 0
	df := make(map[string]int)
	for i, text := range texts {
		tokens := tokenizeText(text)
		s.docs[i] = termFreq(tokens)
		s.dls[i] = len(tokens)
		totalLen += len(tokens)
		for tok := range s.docs[i] {
			df[tok]++
		}
	}
	if len(texts) > 0 {
		s.avgdl = float64(totalLen) / float64(len(texts))
	}
	n := float64(len(texts))
	for tok, dfTok := range df {
		s.idf[tok] = math.Log(1 + (n-float64(dfTok)+0.5)/(float64(dfTok)+0.5))
	}
	return s
}

// score 返回第 i 个文档对查询词的 BM25 得分；未命中任何词返回 0
func (s *bm25Scorer) score(i int, queryTokens []string) float64 {
	tf := s.docs[i]
	if len(tf) == 0 || len(queryTokens) == 0 {
		return 0
	}
	dl := float64(s.dls[i])
	var sum float64
	for _, tok := range queryTokens {
		freq, ok := tf[tok]
		if !ok {
			continue
		}
		idf := s.idf[tok]
		norm := 1 - bm25B + bm25B*dl/s.avgdl
		sum += idf * (float64(freq) * (bm25K1 + 1)) / (float64(freq) + bm25K1*norm)
	}
	return sum
}

// rankedID 是一路检索的排名条目
type rankedID struct {
	id    string
	score float64
}

// rankDescending 按分降序、同分按 ID 升序排名
func rankDescending(items []rankedID) []rankedID {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].id < items[j].id
		}
		return items[i].score > items[j].score
	})
	return items
}

// rrfFuse 按 RRF 融合多路排名：fused = Σ 1/(rrfK+rank)，rank 从 1 计；
// 各路条目全部参与融合（稠密向量分可为负但仍是有效命中），是否过滤零分由调用方决定；
// 返回融合分降序、同分按 ID 升序的排名
func rrfFuse(lists [][]rankedID) []rankedID {
	fused := make(map[string]float64, 64)
	for _, list := range lists {
		for rank, item := range list {
			fused[item.id] += 1 / float64(rrfK+rank+1)
		}
	}
	out := make([]rankedID, 0, len(fused))
	for id, score := range fused {
		out = append(out, rankedID{id: id, score: score})
	}
	return rankDescending(out)
}
