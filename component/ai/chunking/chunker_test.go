// 切块策略测试。
//
// @author zwei

package chunking

import (
	"strings"
	"testing"
	"unicode/utf8"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// TestNewDefault 覆盖零配置默认策略
func TestNewDefault(t *testing.T) {
	t.Parallel()
	chunker, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	recursive, ok := chunker.(*recursiveChunker)
	if !ok {
		t.Fatalf("默认策略 = %T，期望 recursiveChunker", chunker)
	}
	if recursive.chunkSize != defaultChunkSize || recursive.chunkOverlap != defaultChunkOverlap {
		t.Fatalf("默认切块参数 = %+v", recursive)
	}
	if len(recursive.separators) == 0 {
		t.Fatal("默认分隔符为空")
	}
}

// TestNewStrategies 覆盖各策略的独立配置与构造类型
func TestNewStrategies(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		cfg   Config
		check func(t *testing.T, chunker Chunker)
	}{
		{
			name: "recursive",
			cfg: Config{
				Strategy:  StrategyRecursive,
				Recursive: RecursiveConfig{ChunkSize: 100, ChunkOverlap: 10},
			},
			check: func(t *testing.T, chunker Chunker) {
				s, ok := chunker.(*recursiveChunker)
				if !ok || s.chunkSize != 100 || s.chunkOverlap != 10 || len(s.separators) == 0 {
					t.Fatalf("recursive 配置: %T %+v", chunker, chunker)
				}
			},
		},
		{
			name: "fixed",
			cfg: Config{
				Strategy: StrategyFixed,
				Fixed:    FixedConfig{ChunkSize: 100, ChunkOverlap: 10},
			},
			check: func(t *testing.T, chunker Chunker) {
				s, ok := chunker.(*fixedChunker)
				if !ok || s.chunkSize != 100 || s.chunkOverlap != 10 {
					t.Fatalf("fixed 配置: %T %+v", chunker, chunker)
				}
			},
		},
		{
			name: "sentence",
			cfg: Config{
				Strategy: StrategySentence,
				Sentence: SentenceConfig{
					ChunkSize:    100,
					ChunkOverlap: 10,
					Separators:   []string{"。"},
				},
			},
			check: func(t *testing.T, chunker Chunker) {
				s, ok := chunker.(*sentenceChunker)
				if !ok || s.chunkSize != 100 || len(s.separators) != 1 || s.separators[0] != "。" {
					t.Fatalf("sentence 配置: %T %+v", chunker, chunker)
				}
			},
		},
		{
			name: "markdown",
			cfg: Config{
				Strategy: StrategyMarkdown,
				Markdown: MarkdownConfig{ChunkSize: 100, ChunkOverlap: 10, MaxHeadingLevel: 3},
			},
			check: func(t *testing.T, chunker Chunker) {
				s, ok := chunker.(*markdownChunker)
				if !ok || s.chunkSize != 100 || s.maxHeadingLevel != 3 {
					t.Fatalf("markdown 配置: %T %+v", chunker, chunker)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunker, err := New(tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, chunker)
		})
	}
}

// TestNewIgnoresOtherStrategies 覆盖非当前策略的子配置被忽略
func TestNewIgnoresOtherStrategies(t *testing.T) {
	t.Parallel()
	chunker, err := New(Config{
		Strategy:  StrategyFixed,
		Recursive: RecursiveConfig{ChunkSize: -1},
		Fixed:     FixedConfig{ChunkSize: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := chunker.(*fixedChunker); !ok {
		t.Fatalf("策略 = %T，期望 fixedChunker", chunker)
	}
}

// TestNewInvalidConfig 覆盖策略专属配置校验
func TestNewInvalidConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
	}{
		{"unknown strategy", Config{Strategy: "bogus"}},
		{"recursive negative size", Config{Recursive: RecursiveConfig{ChunkSize: -1}}},
		{"recursive size too large", Config{
			Recursive: RecursiveConfig{ChunkSize: MaxChunkSize + 1},
		}},
		{"recursive negative overlap", Config{Recursive: RecursiveConfig{ChunkOverlap: -1}}},
		{"recursive overlap equals size", Config{
			Recursive: RecursiveConfig{ChunkSize: 10, ChunkOverlap: 10},
		}},
		{"recursive empty separator", Config{
			Recursive: RecursiveConfig{Separators: []string{""}},
		}},
		{"fixed negative size", Config{
			Strategy: StrategyFixed,
			Fixed:    FixedConfig{ChunkSize: -1},
		}},
		{"fixed overlap equals size", Config{
			Strategy: StrategyFixed,
			Fixed:    FixedConfig{ChunkSize: 10, ChunkOverlap: 10},
		}},
		{"sentence negative overlap", Config{
			Strategy: StrategySentence,
			Sentence: SentenceConfig{ChunkOverlap: -1},
		}},
		{"sentence empty separator", Config{
			Strategy: StrategySentence,
			Sentence: SentenceConfig{Separators: []string{""}},
		}},
		{"markdown negative size", Config{
			Strategy: StrategyMarkdown,
			Markdown: MarkdownConfig{ChunkSize: -1},
		}},
		{"markdown level too high", Config{
			Strategy: StrategyMarkdown,
			Markdown: MarkdownConfig{MaxHeadingLevel: 7},
		}},
		{"markdown negative level", Config{
			Strategy: StrategyMarkdown,
			Markdown: MarkdownConfig{MaxHeadingLevel: -1},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("期望 ErrInvalidArgument，实际 %v", err)
			}
		})
	}
}

// TestChunkTextDefaults 覆盖零配置、单块文本与空白文本
func TestChunkTextDefaults(t *testing.T) {
	t.Parallel()
	chunks, err := ChunkText("hello world", Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0] != "hello world" {
		t.Fatalf("单块结果: %q", chunks)
	}
	if _, err := ChunkText(" \n\t ", Config{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("空白文本: %v", err)
	}
}

// TestChunkTextRecursiveChineseSentence 覆盖中文句末切分与块间重叠
func TestChunkTextRecursiveChineseSentence(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("这是一个用于测试的句子。", 40)
	cfg := Config{
		Strategy:  StrategyRecursive,
		Recursive: RecursiveConfig{ChunkSize: 50, ChunkOverlap: 10},
	}
	chunks, err := ChunkText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("块数过少: %d", len(chunks))
	}
	for i, chunk := range chunks {
		if chunk == "" {
			t.Fatalf("第 %d 块为空", i)
		}
		if n := utf8.RuneCountInString(chunk); n > cfg.Recursive.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
		if !strings.HasSuffix(chunk, "。") {
			t.Fatalf("第 %d 块未落在句末: %q", i, chunk)
		}
	}
	prev := []rune(chunks[0])
	tail := string(prev[len(prev)-cfg.Recursive.ChunkOverlap:])
	if !strings.HasPrefix(chunks[1], tail) {
		t.Fatalf("缺少重叠: tail=%q next=%q", tail, chunks[1])
	}
}

// TestChunkTextRecursiveHardChunk 覆盖无分隔符文本的硬切与窗口重叠
func TestChunkTextRecursiveHardChunk(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", 1000)
	cfg := Config{
		Strategy:  StrategyRecursive,
		Recursive: RecursiveConfig{ChunkSize: 100, ChunkOverlap: 20},
	}
	chunks, err := ChunkText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 13 {
		t.Fatalf("块数 = %d，期望 13", len(chunks))
	}
	for i, chunk := range chunks {
		if n := utf8.RuneCountInString(chunk); n > cfg.Recursive.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
	}
	prev := []rune(chunks[0])
	tail := string(prev[len(prev)-cfg.Recursive.ChunkOverlap:])
	if !strings.HasPrefix(chunks[1], tail) {
		t.Fatalf("硬切缺少重叠: tail=%q next=%q", tail, chunks[1])
	}
}

// TestChunkTextRecursiveParagraph 覆盖按段落回退且块长受控
func TestChunkTextRecursiveParagraph(t *testing.T) {
	t.Parallel()
	paras := []string{"第一段内容。", "第二段内容。", "第三段内容。"}
	text := strings.Join(paras, "\n\n")
	cfg := Config{
		Strategy:  StrategyRecursive,
		Recursive: RecursiveConfig{ChunkSize: 12, ChunkOverlap: 2},
	}
	chunks, err := ChunkText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, chunk := range chunks {
		if n := utf8.RuneCountInString(chunk); n > cfg.Recursive.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限: %q", i, n, chunk)
		}
	}
	joined := strings.Join(chunks, "")
	for _, para := range paras {
		if !strings.Contains(joined, para) {
			t.Fatalf("切分后丢失内容 %q: %q", para, chunks)
		}
	}
}

// TestChunkTextRecursiveLongWords 覆盖按空格切分与贪心装箱
func TestChunkTextRecursiveLongWords(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("word ", 1000)
	cfg := Config{
		Strategy:  StrategyRecursive,
		Recursive: RecursiveConfig{ChunkSize: 100, ChunkOverlap: 20},
	}
	chunks, err := ChunkText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("块数过少: %d", len(chunks))
	}
	for i, chunk := range chunks {
		if n := utf8.RuneCountInString(chunk); n > cfg.Recursive.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
	}
}

// TestChunkTextRecursiveCustomSeparator 覆盖 recursive 自定义分隔符
func TestChunkTextRecursiveCustomSeparator(t *testing.T) {
	t.Parallel()
	chunks, err := ChunkText("apple|banana|cherry", Config{
		Strategy: StrategyRecursive,
		Recursive: RecursiveConfig{
			ChunkSize:  8,
			Separators: []string{"|"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("块数 = %d，期望 3: %q", len(chunks), chunks)
	}
	for i, want := range []string{"apple", "banana", "cherry"} {
		if !strings.Contains(chunks[i], want) {
			t.Fatalf("第 %d 块 = %q，期望包含 %q", i, chunks[i], want)
		}
		if n := utf8.RuneCountInString(chunks[i]); n > 8 {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
	}
}

// TestChunkTextFixed 覆盖固定窗口策略与重叠
func TestChunkTextFixed(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", 1000)
	cfg := Config{
		Strategy: StrategyFixed,
		Fixed:    FixedConfig{ChunkSize: 100, ChunkOverlap: 20},
	}
	chunks, err := ChunkText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 13 {
		t.Fatalf("块数 = %d，期望 13", len(chunks))
	}
	for i, chunk := range chunks {
		if n := utf8.RuneCountInString(chunk); n > cfg.Fixed.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
	}
	prev := []rune(chunks[0])
	tail := string(prev[len(prev)-cfg.Fixed.ChunkOverlap:])
	if !strings.HasPrefix(chunks[1], tail) {
		t.Fatalf("固定窗口缺少重叠: tail=%q next=%q", tail, chunks[1])
	}
}

// TestChunkTextSentence 覆盖按句末切分与装箱
func TestChunkTextSentence(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("这是一个句子。", 40)
	cfg := Config{
		Strategy: StrategySentence,
		Sentence: SentenceConfig{ChunkSize: 50, ChunkOverlap: 10},
	}
	chunks, err := ChunkText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("块数过少: %d", len(chunks))
	}
	for i, chunk := range chunks {
		if n := utf8.RuneCountInString(chunk); n > cfg.Sentence.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
		if !strings.HasSuffix(chunk, "。") {
			t.Fatalf("第 %d 块未落在句末: %q", i, chunk)
		}
	}
}

// TestChunkTextSentenceCustomSeparator 覆盖 sentence 自定义句末分隔符
func TestChunkTextSentenceCustomSeparator(t *testing.T) {
	t.Parallel()
	chunks, err := ChunkText("apple|banana|cherry", Config{
		Strategy: StrategySentence,
		Sentence: SentenceConfig{
			ChunkSize:  8,
			Separators: []string{"|"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("块数 = %d，期望 3: %q", len(chunks), chunks)
	}
	for i, want := range []string{"apple", "banana", "cherry"} {
		if !strings.Contains(chunks[i], want) {
			t.Fatalf("第 %d 块 = %q，期望包含 %q", i, chunks[i], want)
		}
		if n := utf8.RuneCountInString(chunks[i]); n > 8 {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
	}
}

// TestChunkTextMarkdownSections 覆盖标题分节与代码块内 # 的识别
func TestChunkTextMarkdownSections(t *testing.T) {
	t.Parallel()
	text := "# 标题一\n内容一\n\n## 标题二\n内容二\n\n```go\n# 不是标题\n```"
	cfg := Config{
		Strategy: StrategyMarkdown,
		Markdown: MarkdownConfig{ChunkSize: 100, ChunkOverlap: 10},
	}
	chunks, err := ChunkText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("块数 = %d，期望 2: %q", len(chunks), chunks)
	}
	if chunks[0] != "# 标题一\n内容一" {
		t.Fatalf("第一块 = %q", chunks[0])
	}
	if !strings.Contains(chunks[1], "## 标题二") || !strings.Contains(chunks[1], "# 不是标题") {
		t.Fatalf("第二块 = %q", chunks[1])
	}
}

// TestChunkTextMarkdownMaxHeadingLevel 覆盖 markdown 最大标题层级
func TestChunkTextMarkdownMaxHeadingLevel(t *testing.T) {
	t.Parallel()
	text := "# A\na\n## B\nb\n### C\nc"
	cfg := Config{
		Strategy: StrategyMarkdown,
		Markdown: MarkdownConfig{ChunkSize: 100, MaxHeadingLevel: 2},
	}
	chunks, err := ChunkText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("块数 = %d，期望 2: %q", len(chunks), chunks)
	}
	if !strings.Contains(chunks[1], "### C") {
		t.Fatalf("三级标题应留在上一节: %q", chunks[1])
	}
	cfg.Markdown.MaxHeadingLevel = 6
	chunks, err = ChunkText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("全层级块数 = %d，期望 3: %q", len(chunks), chunks)
	}
}

// TestChunkTextMarkdownLongSection 覆盖超长小节递归切分
func TestChunkTextMarkdownLongSection(t *testing.T) {
	t.Parallel()
	text := "# 标题\n" + strings.Repeat("这是一个句子。", 30)
	cfg := Config{
		Strategy: StrategyMarkdown,
		Markdown: MarkdownConfig{ChunkSize: 50, ChunkOverlap: 10},
	}
	chunks, err := ChunkText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("块数过少: %d", len(chunks))
	}
	if !strings.Contains(chunks[0], "# 标题") {
		t.Fatalf("首块未保留标题: %q", chunks[0])
	}
	for i, chunk := range chunks {
		if n := utf8.RuneCountInString(chunk); n > cfg.Markdown.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
	}
}

// TestChunkerKeepsText 覆盖各策略均保留原文
func TestChunkerKeepsText(t *testing.T) {
	t.Parallel()
	text := "第一段。\n\n第二段，包含更多内容。\n\n第三段。"
	cases := []struct {
		name string
		cfg  Config
	}{
		{
			name: "recursive",
			cfg: Config{
				Strategy:  StrategyRecursive,
				Recursive: RecursiveConfig{ChunkSize: 8, ChunkOverlap: 2},
			},
		},
		{
			name: "fixed",
			cfg: Config{
				Strategy: StrategyFixed,
				Fixed:    FixedConfig{ChunkSize: 8, ChunkOverlap: 2},
			},
		},
		{
			name: "sentence",
			cfg: Config{
				Strategy: StrategySentence,
				Sentence: SentenceConfig{ChunkSize: 8, ChunkOverlap: 2},
			},
		},
		{
			name: "markdown",
			cfg: Config{
				Strategy: StrategyMarkdown,
				Markdown: MarkdownConfig{ChunkSize: 8, ChunkOverlap: 2},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunks, err := ChunkText(text, tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(chunks, "")
			for _, part := range []string{"第一段", "第二段", "第三段"} {
				if !strings.Contains(joined, part) {
					t.Fatalf("策略 %s 丢失内容 %q: %q", tc.name, part, chunks)
				}
			}
		})
	}
}

// TestNormalizeChunks 覆盖契约兜底校验：修剪、丢空白块、超长与空输出
func TestNormalizeChunks(t *testing.T) {
	t.Parallel()
	chunks, err := NormalizeChunks([]string{"  a  ", "", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0] != "a" || chunks[1] != "b" {
		t.Fatalf("修剪结果: %q", chunks)
	}
	if _, err := NormalizeChunks([]string{"  "}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("空白块: %v", err)
	}
	if _, err := NormalizeChunks(nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("空输出: %v", err)
	}
	if _, err := NormalizeChunks([]string{strings.Repeat("a", MaxChunkSize+1)}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("超长块: %v", err)
	}
}

// stubChunker 自定义切块测试替身
type stubChunker struct {
	chunks []string
	err    error
}

// Chunk 实现 Chunker
func (s stubChunker) Chunk(string) ([]string, error) {
	return s.chunks, s.err
}

// TestNormalize 覆盖包装器：输出契约校验、错误透传与内置实现兼容
func TestNormalize(t *testing.T) {
	t.Parallel()
	if Normalize(nil) != nil {
		t.Fatal("nil 包装应返回 nil")
	}
	// 修剪空白块并保序
	wrapped := Normalize(stubChunker{chunks: []string{" a ", "", "b"}})
	chunks, err := wrapped.Chunk("x")
	if err != nil || len(chunks) != 2 || chunks[0] != "a" || chunks[1] != "b" {
		t.Fatalf("修剪结果: %q err=%v", chunks, err)
	}
	// 违约输出被拒绝
	if _, err := Normalize(stubChunker{chunks: []string{"  "}}).Chunk("x"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("全空白: %v", err)
	}
	if _, err := Normalize(stubChunker{chunks: []string{strings.Repeat("a", MaxChunkSize+1)}}).Chunk("x"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("超长块: %v", err)
	}
	// 内部错误原样透传
	if _, err := Normalize(stubChunker{err: ErrInvalidArgument}).Chunk("x"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("透传错误: %v", err)
	}
	// 内置实现包装后行为不变
	builtin, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err = Normalize(builtin).Chunk("苹果。香蕉。")
	if err != nil || len(chunks) == 0 {
		t.Fatalf("内置实现: %q err=%v", chunks, err)
	}
}
