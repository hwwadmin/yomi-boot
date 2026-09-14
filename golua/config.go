// Package golua 配置
//
// @author zwei
package golua

import (
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/config"
	"gitee.com/hq_project/yomi-boot/errors"
)

// SectionKey YAML 根 key
const SectionKey = "golua"

const (
	defaultExecTimeoutMS        = 500
	defaultMaxInstructions      = 1_000_000
	defaultPoolSize             = 8
	defaultPoolAcquireTimeoutMS = 200
	defaultScriptCacheSize      = 256
	defaultCallStackSize        = 64
	defaultRegistrySize         = 1024 * 64
	defaultRedisKeyPrefix       = "yomi:golua"
	defaultEntryFunc            = "main"
	defaultMaxTableDepth        = 32
	defaultMaxTableSize         = 4096
	defaultMaxStringLen         = 1024 * 1024
)

// Config 表示 golua 配置段
type Config struct {
	Enabled              *bool    `mapstructure:"enabled"`                 // Enabled 是否启用；未配置默认 false
	RefreshOnStart       *bool    `mapstructure:"refresh_on_start"`        // RefreshOnStart 启动时是否普通 Refresh；未配置默认 true
	EntryFunc            string   `mapstructure:"entry_func"`              // EntryFunc 入口函数名；默认 main
	ExecTimeoutMS        int      `mapstructure:"exec_timeout_ms"`         // ExecTimeoutMS 墙钟超时毫秒
	MaxInstructions      int      `mapstructure:"max_instructions"`        // MaxInstructions 指令预算配置项；当前上游无 hook，运行时以 exec_timeout_ms 为准
	PoolSize             int      `mapstructure:"pool_size"`               // PoolSize LState 池大小
	PoolAcquireTimeoutMS int      `mapstructure:"pool_acquire_timeout_ms"` // PoolAcquireTimeoutMS 借 State 等待毫秒
	ScriptCacheSize      int      `mapstructure:"script_cache_size"`       // ScriptCacheSize 进程内 Proto LRU 容量
	CallStackSize        int      `mapstructure:"call_stack_size"`         // CallStackSize gopher-lua 调用栈
	RegistrySize         int      `mapstructure:"registry_size"`           // RegistrySize gopher-lua registry
	RedisKeyPrefix       string   `mapstructure:"redis_key_prefix"`        // RedisKeyPrefix 缓存 key 前缀
	AllowLibs            []string `mapstructure:"allow_libs"`              // AllowLibs 标准库白名单
	DenyBaseFuncs        []string `mapstructure:"deny_base_funcs"`         // DenyBaseFuncs 从 base 剔除的全局名
	MaxTableDepth        int      `mapstructure:"max_table_depth"`         // MaxTableDepth 参数/返回嵌套深度
	MaxTableSize         int      `mapstructure:"max_table_size"`          // MaxTableSize 单表元素上限
	MaxStringLen         int      `mapstructure:"max_string_len"`          // MaxStringLen 字符串长度上限
}

// EnabledOrDefault 返回是否启用，未配置默认 false
func (c Config) EnabledOrDefault() bool {
	return config.BoolOrDefault(c.Enabled)
}

// RefreshOnStartOrDefault 返回启动是否普通刷新，未配置默认 true
func (c Config) RefreshOnStartOrDefault() bool {
	if c.RefreshOnStart == nil {
		return true
	}
	return *c.RefreshOnStart
}

// ExecTimeout 返回执行超时
func (c Config) ExecTimeout() time.Duration {
	ms := c.ExecTimeoutMS
	if ms <= 0 {
		ms = defaultExecTimeoutMS
	}
	return time.Duration(ms) * time.Millisecond
}

// PoolAcquireTimeout 返回借池超时
func (c Config) PoolAcquireTimeout() time.Duration {
	ms := c.PoolAcquireTimeoutMS
	if ms <= 0 {
		ms = defaultPoolAcquireTimeoutMS
	}
	return time.Duration(ms) * time.Millisecond
}

// EntryFuncOrDefault 返回入口函数名
func (c Config) EntryFuncOrDefault() string {
	if v := strings.TrimSpace(c.EntryFunc); v != "" {
		return v
	}
	return defaultEntryFunc
}

// RedisKeyPrefixOrDefault 返回 Redis key 前缀
func (c Config) RedisKeyPrefixOrDefault() string {
	if v := strings.TrimSpace(c.RedisKeyPrefix); v != "" {
		return v
	}
	return defaultRedisKeyPrefix
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

// withDefaults 填充数值默认值（不改写 Enabled / RefreshOnStart 指针语义）
func (c Config) withDefaults() Config {
	out := c
	if out.ExecTimeoutMS <= 0 {
		out.ExecTimeoutMS = defaultExecTimeoutMS
	}
	if out.MaxInstructions <= 0 {
		out.MaxInstructions = defaultMaxInstructions
	}
	if out.PoolSize <= 0 {
		out.PoolSize = defaultPoolSize
	}
	if out.PoolAcquireTimeoutMS <= 0 {
		out.PoolAcquireTimeoutMS = defaultPoolAcquireTimeoutMS
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
	out.RedisKeyPrefix = out.RedisKeyPrefixOrDefault()
	out.AllowLibs = out.AllowLibsOrDefault()
	out.DenyBaseFuncs = out.DenyBaseFuncsOrDefault()
	return out
}

// Validate 校验配置；仅 enabled=true 时校验资源参数
func (c Config) Validate() error {
	if !c.EnabledOrDefault() {
		return nil
	}
	d := c.withDefaults()
	if d.ExecTimeoutMS <= 0 {
		return errors.New("golua.exec_timeout_ms 必须大于 0")
	}
	if d.MaxInstructions <= 0 {
		return errors.New("golua.max_instructions 必须大于 0")
	}
	if d.PoolSize <= 0 {
		return errors.New("golua.pool_size 必须大于 0")
	}
	if d.PoolAcquireTimeoutMS <= 0 {
		return errors.New("golua.pool_acquire_timeout_ms 必须大于 0")
	}
	if d.ScriptCacheSize <= 0 {
		return errors.New("golua.script_cache_size 必须大于 0")
	}
	unsafe := map[string]struct{}{
		"os": {}, "io": {}, "package": {}, "debug": {}, "channel": {},
	}
	for _, lib := range d.AllowLibs {
		name := strings.ToLower(strings.TrimSpace(lib))
		if _, bad := unsafe[name]; bad {
			return errors.Newf("golua.allow_libs 不允许包含不安全库: %s", lib)
		}
	}
	return nil
}

// ConfigSection 返回 golua 配置段
func ConfigSection() config.Section {
	return config.NewSection(SectionKey, &Config{}, func(c *Config, _ *config.Document) error {
		return c.Validate()
	})
}
