// Package golua 构造配置（按需库，不读 YAML、不进 bootstrap）
//
// @author zwei
package golua

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

const (
	defaultExecTimeout        = 500 * time.Millisecond
	defaultMaxInstructions    = 1_000_000
	defaultPoolSize           = 8
	defaultPoolAcquireTimeout = 200 * time.Millisecond
	defaultScriptCacheSize    = 256
	defaultCallStackSize      = 64
	defaultRegistrySize       = 1024 * 64
	defaultEntryFunc          = "main"
	defaultMaxTableDepth      = 32
	defaultMaxTableSize       = 4096
	defaultMaxStringLen       = 1024 * 1024
)

// Config 创建 golua 组件的构造参数
type Config struct {
	// EntryFunc 入口函数名；空则 main
	EntryFunc string
	// ExecTimeout 墙钟执行超时；0 则默认 500ms
	ExecTimeout time.Duration
	// MaxInstructions 指令预算配置项；当前上游无 hook，运行时以 ExecTimeout 为准；0 则默认 1e6
	MaxInstructions int
	// PoolSize LState 池大小；0 则默认 8
	PoolSize int
	// PoolAcquireTimeout 借池等待上限；0 则默认 200ms
	PoolAcquireTimeout time.Duration
	// ScriptCacheSize 进程内 Proto LRU 容量；0 则默认 256
	ScriptCacheSize int
	// CallStackSize gopher-lua 调用栈；0 则默认 64
	CallStackSize int
	// RegistrySize gopher-lua registry；0 则默认 64Ki
	RegistrySize int
	// AllowLibs 标准库白名单；空则 base/table/string/math
	AllowLibs []string
	// DenyBaseFuncs 从 base 剔除的全局名；空则使用默认危险集
	DenyBaseFuncs []string
	// MaxTableDepth 参数/返回嵌套深度；0 则默认 32
	MaxTableDepth int
	// MaxTableSize 单表元素上限；0 则默认 4096
	MaxTableSize int
	// MaxStringLen 字符串长度上限；0 则默认 1MiB
	MaxStringLen int
	// SkipRefreshOnNew 为 true 时 New 不自动普通 Refresh；默认 false（会 Refresh）
	SkipRefreshOnNew bool
}

// ExecTimeoutOrDefault 返回执行超时
func (c Config) ExecTimeoutOrDefault() time.Duration {
	if c.ExecTimeout > 0 {
		return c.ExecTimeout
	}
	return defaultExecTimeout
}

// PoolAcquireTimeoutOrDefault 返回借池超时
func (c Config) PoolAcquireTimeoutOrDefault() time.Duration {
	if c.PoolAcquireTimeout > 0 {
		return c.PoolAcquireTimeout
	}
	return defaultPoolAcquireTimeout
}

// EntryFuncOrDefault 返回入口函数名
func (c Config) EntryFuncOrDefault() string {
	if v := strings.TrimSpace(c.EntryFunc); v != "" {
		return v
	}
	return defaultEntryFunc
}

// AllowLibsOrDefault 返回库白名单
func (c Config) AllowLibsOrDefault() []string {
	if len(c.AllowLibs) == 0 {
		return []string{"base", "table", "string", "math"}
	}
	return append([]string(nil), c.AllowLibs...)
}

// DenyBaseFuncsOrDefault 返回需剔除的 base 全局
func (c Config) DenyBaseFuncsOrDefault() []string {
	if len(c.DenyBaseFuncs) == 0 {
		return []string{
			"dofile", "loadfile", "load", "loadstring",
			"require", "collectgarbage", "rawset", "rawget",
			"setfenv", "getfenv", "getmetatable", "setmetatable",
		}
	}
	return append([]string(nil), c.DenyBaseFuncs...)
}

// withDefaults 填充默认值
func (c Config) withDefaults() Config {
	out := c
	if out.ExecTimeout <= 0 {
		out.ExecTimeout = defaultExecTimeout
	}
	if out.MaxInstructions <= 0 {
		out.MaxInstructions = defaultMaxInstructions
	}
	if out.PoolSize <= 0 {
		out.PoolSize = defaultPoolSize
	}
	if out.PoolAcquireTimeout <= 0 {
		out.PoolAcquireTimeout = defaultPoolAcquireTimeout
	}
	if out.ScriptCacheSize <= 0 {
		out.ScriptCacheSize = defaultScriptCacheSize
	}
	if out.CallStackSize <= 0 {
		out.CallStackSize = defaultCallStackSize
	}
	if out.RegistrySize <= 0 {
		out.RegistrySize = defaultRegistrySize
	}
	if out.MaxTableDepth <= 0 {
		out.MaxTableDepth = defaultMaxTableDepth
	}
	if out.MaxTableSize <= 0 {
		out.MaxTableSize = defaultMaxTableSize
	}
	if out.MaxStringLen <= 0 {
		out.MaxStringLen = defaultMaxStringLen
	}
	out.EntryFunc = out.EntryFuncOrDefault()
	out.AllowLibs = out.AllowLibsOrDefault()
	out.DenyBaseFuncs = out.DenyBaseFuncsOrDefault()
	return out
}

// Validate 校验构造参数
func (c Config) Validate() error {
	d := c.withDefaults()
	if d.ExecTimeout <= 0 {
		return errors.New("golua: ExecTimeout 必须大于 0")
	}
	if d.MaxInstructions <= 0 {
		return errors.New("golua: MaxInstructions 必须大于 0")
	}
	if d.PoolSize <= 0 {
		return errors.New("golua: PoolSize 必须大于 0")
	}
	if d.PoolAcquireTimeout <= 0 {
		return errors.New("golua: PoolAcquireTimeout 必须大于 0")
	}
	if d.ScriptCacheSize <= 0 {
		return errors.New("golua: ScriptCacheSize 必须大于 0")
	}
	unsafe := map[string]struct{}{
		"os": {}, "io": {}, "package": {}, "debug": {}, "channel": {},
	}
	for _, lib := range d.AllowLibs {
		name := strings.ToLower(strings.TrimSpace(lib))
		if _, bad := unsafe[name]; bad {
			return errors.Newf("golua: AllowLibs 不允许包含不安全库: %s", lib)
		}
	}
	return nil
}
