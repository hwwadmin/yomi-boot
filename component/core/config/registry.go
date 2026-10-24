package config

import (
	"fmt"
	"strings"

	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

// Section 一段可注册配置
type Section interface {
	Key() string                // Key 返回 YAML 根 key，须全局唯一
	Target() any                // Target 返回供 UnmarshalKey 写入的指针
	Validate(cfg *Config) error // Validate 校验本段语义
}

// Registry 配置段注册表
type Registry struct {
	sections []Section
	index    map[string]Section
}

// NewRegistry 创建空注册表
func NewRegistry() *Registry {
	return &Registry{
		index: make(map[string]Section),
	}
}

// Register 注册配置段；key 为空或重复时 panic（Fail Fast）
func (r *Registry) Register(sections ...Section) *Registry {
	if r == nil {
		panic(errors.New("配置注册表不能为空"))
	}
	if r.index == nil {
		r.index = make(map[string]Section)
	}
	for _, s := range sections {
		if s == nil {
			panic(errors.New("配置段不能为空"))
		}
		key := strings.TrimSpace(s.Key())
		if key == "" {
			panic(errors.New("配置段 key 不能为空"))
		}
		if _, exists := r.index[key]; exists {
			panic(errors.Newf("配置段 key 重复注册: %s", key))
		}
		if s.Target() == nil {
			panic(errors.Newf("配置段 target 不能为空: %s", key))
		}
		r.sections = append(r.sections, s)
		r.index[key] = s
	}
	return r
}

// Keys 返回已注册根 key（注册顺序）
func (r *Registry) Keys() []string {
	if r == nil {
		return nil
	}
	keys := make([]string, 0, len(r.sections))
	for _, s := range r.sections {
		keys = append(keys, s.Key())
	}
	return keys
}

// Has 判断是否已注册指定根 key
func (r *Registry) Has(key string) bool {
	if r == nil || r.index == nil {
		return false
	}
	_, ok := r.index[strings.TrimSpace(key)]
	return ok
}

// Config 已绑定的配置
type Config struct {
	sections map[string]Section
	order    []string
}

// Has 判断配置是否包含指定根 key
func (c *Config) Has(key string) bool {
	if c == nil || c.sections == nil {
		return false
	}
	_, ok := c.sections[strings.TrimSpace(key)]
	return ok
}

// Keys 返回已绑定的根 key（注册顺序）
func (c *Config) Keys() []string {
	if c == nil {
		return nil
	}
	out := make([]string, len(c.order))
	copy(out, c.order)
	return out
}

// MustSection 按 key 取出已绑定的强类型配置指针；缺失或类型不匹配时 panic
func MustSection[T any](c *Config, key string) *T {
	if c == nil || c.sections == nil {
		panic(errors.New("配置不能为空"))
	}
	normalized := strings.TrimSpace(key)
	s, ok := c.sections[normalized]
	if !ok {
		panic(errors.Newf("配置段不存在: %s", normalized))
	}
	typed, ok := s.Target().(*T)
	if !ok || typed == nil {
		panic(errors.Newf("配置段类型不匹配: %s", normalized))
	}
	return typed
}

// typedSection 泛型配置段实现
type typedSection[T any] struct {
	key      string
	target   *T
	validate func(*T, *Config) error
}

// NewSection 创建指向已有 target 的配置段
func NewSection[T any](key string, target *T, validate func(*T, *Config) error) Section {
	if target == nil {
		panic(errors.Newf("配置段 target 不能为空: %s", key))
	}
	return &typedSection[T]{
		key:      strings.TrimSpace(key),
		target:   target,
		validate: validate,
	}
}

// Key 返回配置段根 key
func (s *typedSection[T]) Key() string {
	return s.key
}

// Target 返回供 UnmarshalKey 写入的指针
func (s *typedSection[T]) Target() any {
	return s.target
}

// Validate 校验本段语义
func (s *typedSection[T]) Validate(cfg *Config) error {
	if s.validate == nil {
		return nil
	}
	return s.validate(s.target, cfg)
}

// formatUnknownRootKeys 格式化未注册根 key 错误信息
func formatUnknownRootKeys(keys []string) string {
	return fmt.Sprintf("配置存在未注册段: %s", strings.Join(keys, ", "))
}
