// 切块策略测试。
//
// @author zwei

package rag

import (
	"strings"
	"testing"
	"unicode/utf8"

	"gitee.com/hq_project/yomi-boot/core/errors"
)

// TestNewSplitterDefault 覆盖零配置默认策略
func TestNewSplitterDefault(t *testing.T) {
	t.Parallel()
	splitter, err := NewSplitter(SplitConfig{})
	if err != nil {
		t.Fatal(err)
	}
	recursive, ok := splitter.(*recursiveSplitter)
	if !ok {
		t.Fatalf("默认策略 = %T，期望 recursiveSplitter", splitter)
	}
	if recursive.chunkSize != defaultChunkSize || recursive.chunkOverlap != defaultChunkOverlap {
		t.Fatalf("默认切块参数 = %+v", recursive)
	}
	if len(recursive.separators) == 0 {
		t.Fatal("默认分隔符为空")
	}
}

// TestNewSplitterStrategies 覆盖各策略的独立配置与构造类型
func TestNewSplitterStrategies(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		cfg   SplitConfig
		check func(t *testing.T, splitter Splitter)
	}{
		{
			name: "recursive",
			cfg: SplitConfig{
				Strategy:  StrategyRecursive,
				Recursive: RecursiveConfig{ChunkSize: 100, ChunkOverlap: 10},
			},
			check: func(t *testing.T, splitter Splitter) {
				s, ok := splitter.(*recursiveSplitter)
				if !ok || s.chunkSize != 100 || s.chunkOverlap != 10 || len(s.separators) == 0 {
					t.Fatalf("recursive 配置: %T %+v", splitter, splitter)
				}
			},
		},
		{
			name: "fixed",
			cfg: SplitConfig{
				Strategy: StrategyFixed,
				Fixed:    FixedConfig{ChunkSize: 100, ChunkOverlap: 10},
			},
			check: func(t *testing.T, splitter Splitter) {
				s, ok := splitter.(*fixedSplitter)
				if !ok || s.chunkSize != 100 || s.chunkOverlap != 10 {
					t.Fatalf("fixed 配置: %T %+v", splitter, splitter)
				}
			},
		},
		{
			name: "sentence",
			cfg: SplitConfig{
				Strategy: StrategySentence,
				Sentence: SentenceConfig{
					ChunkSize:    100,
					ChunkOverlap: 10,
					Separators:   []string{"。"},
				},
			},
			check: func(t *testing.T, splitter Splitter) {
				s, ok := splitter.(*sentenceSplitter)
				if !ok || s.chunkSize != 100 || len(s.separators) != 1 || s.separators[0] != "。" {
					t.Fatalf("sentence 配置: %T %+v", splitter, splitter)
				}
			},
		},
		{
			name: "markdown",
			cfg: SplitConfig{
				Strategy: StrategyMarkdown,
				Markdown: MarkdownConfig{ChunkSize: 100, ChunkOverlap: 10, MaxHeadingLevel: 3},
			},
			check: func(t *testing.T, splitter Splitter) {
				s, ok := splitter.(*markdownSplitter)
				if !ok || s.chunkSize != 100 || s.maxHeadingLevel != 3 {
					t.Fatalf("markdown 配置: %T %+v", splitter, splitter)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			splitter, err := NewSplitter(tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, splitter)
		})
	}
}

// TestNewSplitterIgnoresOtherStrategies 覆盖非当前策略的子配置被忽略
func TestNewSplitterIgnoresOtherStrategies(t *testing.T) {
	t.Parallel()
	splitter, err := NewSplitter(SplitConfig{
		Strategy:  StrategyFixed,
		Recursive: RecursiveConfig{ChunkSize: -1},
		Fixed:     FixedConfig{ChunkSize: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := splitter.(*fixedSplitter); !ok {
		t.Fatalf("策略 = %T，期望 fixedSplitter", splitter)
	}
}

// TestNewSplitterInvalidConfig 覆盖策略专属配置校验
func TestNewSplitterInvalidConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  SplitConfig
	}{
		{"unknown strategy", SplitConfig{Strategy: "bogus"}},
		{"recursive negative size", SplitConfig{Recursive: RecursiveConfig{ChunkSize: -1}}},
		{"recursive size too large", SplitConfig{
			Recursive: RecursiveConfig{ChunkSize: maxChunkSize + 1},
		}},
		{"recursive negative overlap", SplitConfig{Recursive: RecursiveConfig{ChunkOverlap: -1}}},
		{"recursive overlap equals size", SplitConfig{
			Recursive: RecursiveConfig{ChunkSize: 10, ChunkOverlap: 10},
		}},
		{"recursive empty separator", SplitConfig{
			Recursive: RecursiveConfig{Separators: []string{""}},
		}},
		{"fixed negative size", SplitConfig{
			Strategy: StrategyFixed,
			Fixed:    FixedConfig{ChunkSize: -1},
		}},
		{"fixed overlap equals size", SplitConfig{
			Strategy: StrategyFixed,
			Fixed:    FixedConfig{ChunkSize: 10, ChunkOverlap: 10},
		}},
		{"sentence negative overlap", SplitConfig{
			Strategy: StrategySentence,
			Sentence: SentenceConfig{ChunkOverlap: -1},
		}},
		{"sentence empty separator", SplitConfig{
			Strategy: StrategySentence,
			Sentence: SentenceConfig{Separators: []string{""}},
		}},
		{"markdown negative size", SplitConfig{
			Strategy: StrategyMarkdown,
			Markdown: MarkdownConfig{ChunkSize: -1},
		}},
		{"markdown level too high", SplitConfig{
			Strategy: StrategyMarkdown,
			Markdown: MarkdownConfig{MaxHeadingLevel: 7},
		}},
		{"markdown negative level", SplitConfig{
			Strategy: StrategyMarkdown,
			Markdown: MarkdownConfig{MaxHeadingLevel: -1},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewSplitter(tc.cfg); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("期望 ErrInvalidArgument，实际 %v", err)
			}
		})
	}
}

// TestSplitTextDefaults 覆盖零配置、单块文本与空白文本
func TestSplitTextDefaults(t *testing.T) {
	t.Parallel()
	chunks, err := SplitText("hello world", SplitConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0] != "hello world" {
		t.Fatalf("单块结果: %q", chunks)
	}
	if _, err := SplitText(" \n\t ", SplitConfig{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("空白文本: %v", err)
	}
}

// TestSplitTextRecursiveChineseSentence 覆盖中文句末切分与块间重叠
func TestSplitTextRecursiveChineseSentence(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("这是一个用于测试的句子。", 40)
	cfg := SplitConfig{
		Strategy:  StrategyRecursive,
		Recursive: RecursiveConfig{ChunkSize: 50, ChunkOverlap: 10},
	}
	chunks, err := SplitText(text, cfg)
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

// TestSplitTextRecursiveHardSplit 覆盖无分隔符文本的硬切与窗口重叠
func TestSplitTextRecursiveHardSplit(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", 1000)
	cfg := SplitConfig{
		Strategy:  StrategyRecursive,
		Recursive: RecursiveConfig{ChunkSize: 100, ChunkOverlap: 20},
	}
	chunks, err := SplitText(text, cfg)
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

// TestSplitTextRecursiveParagraph 覆盖按段落回退且块长受控
func TestSplitTextRecursiveParagraph(t *testing.T) {
	t.Parallel()
	paras := []string{"第一段内容。", "第二段内容。", "第三段内容。"}
	text := strings.Join(paras, "\n\n")
	cfg := SplitConfig{
		Strategy:  StrategyRecursive,
		Recursive: RecursiveConfig{ChunkSize: 12, ChunkOverlap: 2},
	}
	chunks, err := SplitText(text, cfg)
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

// TestSplitTextRecursiveLongWords 覆盖按空格切分与贪心装箱
func TestSplitTextRecursiveLongWords(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("word ", 1000)
	cfg := SplitConfig{
		Strategy:  StrategyRecursive,
		Recursive: RecursiveConfig{ChunkSize: 100, ChunkOverlap: 20},
	}
	chunks, err := SplitText(text, cfg)
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

// TestSplitTextRecursiveCustomSeparator 覆盖 recursive 自定义分隔符
func TestSplitTextRecursiveCustomSeparator(t *testing.T) {
	t.Parallel()
	chunks, err := SplitText("apple|banana|cherry", SplitConfig{
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

// TestSplitTextFixed 覆盖固定窗口策略与重叠
func TestSplitTextFixed(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", 1000)
	cfg := SplitConfig{
		Strategy: StrategyFixed,
		Fixed:    FixedConfig{ChunkSize: 100, ChunkOverlap: 20},
	}
	chunks, err := SplitText(text, cfg)
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

// TestSplitTextSentence 覆盖按句末切分与装箱
func TestSplitTextSentence(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("这是一个句子。", 40)
	cfg := SplitConfig{
		Strategy: StrategySentence,
		Sentence: SentenceConfig{ChunkSize: 50, ChunkOverlap: 10},
	}
	chunks, err := SplitText(text, cfg)
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

// TestSplitTextSentenceCustomSeparator 覆盖 sentence 自定义句末分隔符
func TestSplitTextSentenceCustomSeparator(t *testing.T) {
	t.Parallel()
	chunks, err := SplitText("apple|banana|cherry", SplitConfig{
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

// TestSplitTextMarkdownSections 覆盖标题分节与代码块内 # 的识别
func TestSplitTextMarkdownSections(t *testing.T) {
	t.Parallel()
	text := "# 标题一\n内容一\n\n## 标题二\n内容二\n\n```go\n# 不是标题\n```"
	cfg := SplitConfig{
		Strategy: StrategyMarkdown,
		Markdown: MarkdownConfig{ChunkSize: 100, ChunkOverlap: 10},
	}
	chunks, err := SplitText(text, cfg)
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

// TestSplitTextMarkdownMaxHeadingLevel 覆盖 markdown 最大标题层级
func TestSplitTextMarkdownMaxHeadingLevel(t *testing.T) {
	t.Parallel()
	text := "# A\na\n## B\nb\n### C\nc"
	cfg := SplitConfig{
		Strategy: StrategyMarkdown,
		Markdown: MarkdownConfig{ChunkSize: 100, MaxHeadingLevel: 2},
	}
	chunks, err := SplitText(text, cfg)
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
	chunks, err = SplitText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("全层级块数 = %d，期望 3: %q", len(chunks), chunks)
	}
}

// TestSplitTextMarkdownLongSection 覆盖超长小节递归切分
func TestSplitTextMarkdownLongSection(t *testing.T) {
	t.Parallel()
	text := "# 标题\n" + strings.Repeat("这是一个句子。", 30)
	cfg := SplitConfig{
		Strategy: StrategyMarkdown,
		Markdown: MarkdownConfig{ChunkSize: 50, ChunkOverlap: 10},
	}
	chunks, err := SplitText(text, cfg)
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

// TestSplitterKeepsText 覆盖各策略均保留原文
func TestSplitterKeepsText(t *testing.T) {
	t.Parallel()
	text := "第一段。\n\n第二段，包含更多内容。\n\n第三段。"
	cases := []struct {
		name string
		cfg  SplitConfig
	}{
		{
			name: "recursive",
			cfg: SplitConfig{
				Strategy:  StrategyRecursive,
				Recursive: RecursiveConfig{ChunkSize: 8, ChunkOverlap: 2},
			},
		},
		{
			name: "fixed",
			cfg: SplitConfig{
				Strategy: StrategyFixed,
				Fixed:    FixedConfig{ChunkSize: 8, ChunkOverlap: 2},
			},
		},
		{
			name: "sentence",
			cfg: SplitConfig{
				Strategy: StrategySentence,
				Sentence: SentenceConfig{ChunkSize: 8, ChunkOverlap: 2},
			},
		},
		{
			name: "markdown",
			cfg: SplitConfig{
				Strategy: StrategyMarkdown,
				Markdown: MarkdownConfig{ChunkSize: 8, ChunkOverlap: 2},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunks, err := SplitText(text, tc.cfg)
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
