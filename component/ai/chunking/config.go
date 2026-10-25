// 切块配置与默认值；本包不读 YAML。
//
// @author zwei

package chunking

const (
	defaultChunkSize    = 800
	defaultChunkOverlap = 100
	// MaxChunkSize 单块最大字符数（按 rune 计）上限；超过即返回 ErrInvalidArgument
	MaxChunkSize = 8192
)

// Config 切块策略配置；只有 Strategy 对应的子配置生效，其余忽略
type Config struct {
	Strategy  Strategy        // Strategy 是切块策略；空则 recursive
	Recursive RecursiveConfig // Recursive 仅在 Strategy 为 recursive 时生效
	Fixed     FixedConfig     // Fixed 仅在 Strategy 为 fixed 时生效
	Sentence  SentenceConfig  // Sentence 仅在 Strategy 为 sentence 时生效
	Markdown  MarkdownConfig  // Markdown 仅在 Strategy 为 markdown 时生效
}

// RecursiveConfig recursive 策略配置
type RecursiveConfig struct {
	ChunkSize    int      // ChunkSize 是单块最大字符数（按 rune 计）；0 则 800；不得超过 8192
	ChunkOverlap int      // ChunkOverlap 是相邻块重叠字符数；0 则 min(100, ChunkSize/4)；必须小于 ChunkSize
	Separators   []string // Separators 是分隔符优先级；空则使用内置中英文分隔符；元素不得为空串
}

// FixedConfig fixed 策略配置
type FixedConfig struct {
	ChunkSize    int // ChunkSize 是单块最大字符数（按 rune 计）；0 则 800；不得超过 8192
	ChunkOverlap int // ChunkOverlap 是相邻块重叠字符数；0 则 min(100, ChunkSize/4)；必须小于 ChunkSize
}

// SentenceConfig sentence 策略配置
type SentenceConfig struct {
	ChunkSize    int      // ChunkSize 是单块最大字符数（按 rune 计）；0 则 800；不得超过 8192
	ChunkOverlap int      // ChunkOverlap 是相邻块重叠字符数；0 则 min(100, ChunkSize/4)；必须小于 ChunkSize
	Separators   []string // Separators 是句末分隔符；空则使用内置中英文句末标点；元素不得为空串
}

// MarkdownConfig markdown 策略配置
type MarkdownConfig struct {
	ChunkSize       int // ChunkSize 是单块最大字符数（按 rune 计）；0 则 800；不得超过 8192
	ChunkOverlap    int // ChunkOverlap 是相邻块重叠字符数；0 则 min(100, ChunkSize/4)；必须小于 ChunkSize
	MaxHeadingLevel int // MaxHeadingLevel 是参与分节的最大标题层级；0 则 6；取值 1..6
}
