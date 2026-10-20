// Package rag 切块策略测试。
//
// @author zwei
package rag

import (
	"strings"
	"testing"
	"unicode/utf8"

	"gitee.com/hq_project/yomi-boot/errors"
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
		t.Fatal("默认可分隔符为空")
	}
}

// TestNewSplitterStrategies 覆盖各策略的构造类型
func TestNewSplitterStrategies(t *testing.T) {
	t.Parallel()
	cases := []struct {
		strategy SplitStrategy
		check    func(t *testing.T, splitter Splitter)
	}{
		{StrategyRecursive, func(t *testing.T, splitter Splitter) {
			if _, ok := splitter.(*recursiveSplitter); !ok {
				t.Fatalf("recursive 类型错误: %T", splitter)
			}
		}},
		{StrategyFixed, func(t *testing.T, splitter Splitter) {
			if _, ok := splitter.(*fixedSplitter); !ok {
				t.Fatalf("fixed 类型错误: %T", splitter)
			}
		}},
		{StrategySentence, func(t *testing.T, splitter Splitter) {
			if _, ok := splitter.(*sentenceSplitter); !ok {
				t.Fatalf("sentence 类型错误: %T", splitter)
			}
		}},
		{StrategyMarkdown, func(t *testing.T, splitter Splitter) {
			if _, ok := splitter.(*markdownSplitter); !ok {
				t.Fatalf("markdown 类型错误: %T", splitter)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(string(tc.strategy), func(t *testing.T) {
			splitter, err := NewSplitter(SplitConfig{Strategy: tc.strategy})
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, splitter)
		})
	}
}

// TestNewSplitterInvalidConfig 覆盖策略与切块配置边界
func TestNewSplitterInvalidConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  SplitConfig
	}{
		{"unknown strategy", SplitConfig{Strategy: "bogus"}},
		{"negative chunk size", SplitConfig{ChunkSize: -1}},
		{"chunk size too large", SplitConfig{ChunkSize: maxChunkSize + 1}},
		{"negative overlap", SplitConfig{ChunkOverlap: -1}},
		{"overlap equals size", SplitConfig{ChunkSize: 10, ChunkOverlap: 10}},
		{"empty separator", SplitConfig{Separators: []string{""}}},
		{"separators with fixed", SplitConfig{Strategy: StrategyFixed, Separators: []string{"。"}}},
		{"separators with sentence", SplitConfig{Strategy: StrategySentence, Separators: []string{"。"}}},
		{"separators with markdown", SplitConfig{Strategy: StrategyMarkdown, Separators: []string{"。"}}},
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
	cfg := SplitConfig{Strategy: StrategyRecursive, ChunkSize: 50, ChunkOverlap: 10}
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
		if n := utf8.RuneCountInString(chunk); n > cfg.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
		if !strings.HasSuffix(chunk, "。") {
			t.Fatalf("第 %d 块未落在句末: %q", i, chunk)
		}
	}
	prev := []rune(chunks[0])
	tail := string(prev[len(prev)-cfg.ChunkOverlap:])
	if !strings.HasPrefix(chunks[1], tail) {
		t.Fatalf("缺少重叠: tail=%q next=%q", tail, chunks[1])
	}
}

// TestSplitTextRecursiveHardSplit 覆盖无分隔符文本的硬切与窗口重叠
func TestSplitTextRecursiveHardSplit(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", 1000)
	cfg := SplitConfig{Strategy: StrategyRecursive, ChunkSize: 100, ChunkOverlap: 20}
	chunks, err := SplitText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 13 {
		t.Fatalf("块数 = %d，期望 13", len(chunks))
	}
	for i, chunk := range chunks {
		if n := utf8.RuneCountInString(chunk); n > cfg.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
	}
	prev := []rune(chunks[0])
	tail := string(prev[len(prev)-cfg.ChunkOverlap:])
	if !strings.HasPrefix(chunks[1], tail) {
		t.Fatalf("硬切缺少重叠: tail=%q next=%q", tail, chunks[1])
	}
}

// TestSplitTextRecursiveParagraph 覆盖按段落回退且块长受控
func TestSplitTextRecursiveParagraph(t *testing.T) {
	t.Parallel()
	paras := []string{"第一段内容。", "第二段内容。", "第三段内容。"}
	text := strings.Join(paras, "\n\n")
	cfg := SplitConfig{Strategy: StrategyRecursive, ChunkSize: 12, ChunkOverlap: 2}
	chunks, err := SplitText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, chunk := range chunks {
		if n := utf8.RuneCountInString(chunk); n > cfg.ChunkSize {
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
	cfg := SplitConfig{Strategy: StrategyRecursive, ChunkSize: 100, ChunkOverlap: 20}
	chunks, err := SplitText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("块数过少: %d", len(chunks))
	}
	for i, chunk := range chunks {
		if n := utf8.RuneCountInString(chunk); n > cfg.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
	}
}

// TestSplitTextRecursiveCustomSeparator 覆盖自定义分隔符
func TestSplitTextRecursiveCustomSeparator(t *testing.T) {
	t.Parallel()
	chunks, err := SplitText("apple|banana|cherry", SplitConfig{
		Strategy:   StrategyRecursive,
		ChunkSize:  8,
		Separators: []string{"|"},
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
	cfg := SplitConfig{Strategy: StrategyFixed, ChunkSize: 100, ChunkOverlap: 20}
	chunks, err := SplitText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 13 {
		t.Fatalf("块数 = %d，期望 13", len(chunks))
	}
	for i, chunk := range chunks {
		if n := utf8.RuneCountInString(chunk); n > cfg.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
	}
	prev := []rune(chunks[0])
	tail := string(prev[len(prev)-cfg.ChunkOverlap:])
	if !strings.HasPrefix(chunks[1], tail) {
		t.Fatalf("固定窗口缺少重叠: tail=%q next=%q", tail, chunks[1])
	}
}

// TestSplitTextSentence 覆盖按句末切分与装箱
func TestSplitTextSentence(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("这是一个句子。", 40)
	cfg := SplitConfig{Strategy: StrategySentence, ChunkSize: 50, ChunkOverlap: 10}
	chunks, err := SplitText(text, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("块数过少: %d", len(chunks))
	}
	for i, chunk := range chunks {
		if n := utf8.RuneCountInString(chunk); n > cfg.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
		if !strings.HasSuffix(chunk, "。") {
			t.Fatalf("第 %d 块未落在句末: %q", i, chunk)
		}
	}
}

// TestSplitTextMarkdownSections 覆盖标题分节与代码块内 # 的识别
func TestSplitTextMarkdownSections(t *testing.T) {
	t.Parallel()
	text := "# 标题一\n内容一\n\n## 标题二\n内容二\n\n```go\n# 不是标题\n```"
	chunks, err := SplitText(text, SplitConfig{
		Strategy:     StrategyMarkdown,
		ChunkSize:    100,
		ChunkOverlap: 10,
	})
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

// TestSplitTextMarkdownLongSection 覆盖超长小节递归切分
func TestSplitTextMarkdownLongSection(t *testing.T) {
	t.Parallel()
	text := "# 标题\n" + strings.Repeat("这是一个句子。", 30)
	cfg := SplitConfig{Strategy: StrategyMarkdown, ChunkSize: 50, ChunkOverlap: 10}
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
		if n := utf8.RuneCountInString(chunk); n > cfg.ChunkSize {
			t.Fatalf("第 %d 块长度 %d 超过上限", i, n)
		}
	}
}

// TestSplitterKeepsText 覆盖各策略均保留原文
func TestSplitterKeepsText(t *testing.T) {
	t.Parallel()
	text := "第一段。\n\n第二段，包含更多内容。\n\n第三段。"
	strategies := []SplitStrategy{StrategyRecursive, StrategyFixed, StrategySentence, StrategyMarkdown}
	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			chunks, err := SplitText(text, SplitConfig{Strategy: strategy, ChunkSize: 8, ChunkOverlap: 2})
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(chunks, "")
			for _, part := range []string{"第一段", "第二段", "第三段"} {
				if !strings.Contains(joined, part) {
					t.Fatalf("策略 %s 丢失内容 %q: %q", strategy, part, chunks)
				}
			}
		})
	}
}
