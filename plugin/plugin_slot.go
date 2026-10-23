// PluginSlot 定义统一插件插槽：保存配置与有序插件链，并负责整条链的生命周期。

package plugin

import (
	"context"
	"slices"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/config"
	"gitee.com/hq_project/yomi-boot/component/core/errors"
)

const (
	// defaultCloseTimeout 是 Close 未携带 deadline 时的兜底等待上限
	defaultCloseTimeout = 10 * time.Second
	// defaultUnwindTimeout 是 Init 失败回滚已初始化插件的等待上限
	defaultUnwindTimeout = 10 * time.Second
)

// PluginSlot 是统一的插件插槽。
//
// 除 Config 与插件链外只保存每个插件的初始化状态；Init / Close 负责整条链的
// 顺序、回滚与错误聚合，插件不再自行维护启动状态。
type PluginSlot struct {
	Config *config.Config // Config 是已绑定配置（含自定义段）

	chain  []Plugin       // chain 是注册顺序的插件链
	index  map[string]int // index 是插件名到链下标的索引
	states []pluginState  // states 与 chain 一一对应
	inited bool           // inited 标记 Init 是否已执行
	closed bool           // closed 标记 Close 是否已执行或 Init 已失败
}

// pluginState 是单个插件的生命周期状态。
type pluginState struct {
	initialized bool // initialized 表示 Init 成功且未关闭
	skipped     bool // skipped 表示 Init 返回 ErrDisabled
}

// NewPluginSlot 创建插件插槽；插件为空、名称为空或重名时 panic（Fail Fast）。
func NewPluginSlot(cfg *config.Config, plugins []Plugin) *PluginSlot {
	if cfg == nil {
		panic(errors.New("plugin: Config 不能为空"))
	}

	slot := &PluginSlot{
		Config: cfg,
		chain:  make([]Plugin, 0, len(plugins)),
		index:  make(map[string]int, len(plugins)),
	}
	for _, p := range plugins {
		if p == nil {
			panic(errors.New("plugin: 插件不能为空"))
		}
		name := strings.TrimSpace(p.Name())
		if name == "" {
			panic(errors.New("plugin: 插件名称不能为空"))
		}
		if _, exists := slot.index[name]; exists {
			panic(errors.Newf("plugin: 插件名称重复: %s", name))
		}
		slot.index[name] = len(slot.chain)
		slot.chain = append(slot.chain, p)
	}
	slot.states = make([]pluginState, len(slot.chain))
	return slot
}

// Init 按插件链顺序执行 Init；失败时逆序回滚已初始化插件并返回聚合错误。
// 返回 ErrDisabled 的插件被标记为跳过，不参与 hook 与 Close。
func (s *PluginSlot) Init(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.closed {
		return errors.New("plugin: PluginSlot 已关闭，不能 Init")
	}
	if s.inited {
		return errors.New("plugin: PluginSlot 不能重复 Init")
	}
	s.inited = true
	ctx = orBackground(ctx)

	for i, p := range s.chain {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, s.unwind(ctx, i))
		}
		err := safeInit(ctx, p, s)
		if err == nil {
			s.states[i].initialized = true
			continue
		}
		if errors.Is(err, ErrDisabled) {
			s.states[i].skipped = true
			continue
		}
		return errors.Join(
			errors.Wrap(err, "插件初始化失败: %s", p.Name()),
			s.unwind(ctx, i),
		)
	}
	return nil
}

// Close 逆序关闭已初始化插件；可重复调用，返回聚合错误。
// 未携带 deadline 时使用默认 10s；单个插件超时或 panic 后继续关闭剩余插件。
func (s *PluginSlot) Close(ctx context.Context) error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	ctx = orBackground(ctx)
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCloseTimeout)
		defer cancel()
	}

	var errs []error
	for i := len(s.chain) - 1; i >= 0; i-- {
		if !s.states[i].initialized {
			continue
		}
		err := closePlugin(ctx, s.chain[i])
		s.states[i].initialized = false
		if err != nil {
			errs = append(errs, errors.Wrap(err, "插件关闭失败: %s", s.chain[i].Name()))
		}
	}
	return errors.Join(errs...)
}

// unwind 逆序关闭 chain[:failedIndex] 中已初始化的插件，并标记插槽关闭。
func (s *PluginSlot) unwind(ctx context.Context, failedIndex int) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultUnwindTimeout)
	defer cancel()

	var errs []error
	for i := failedIndex - 1; i >= 0; i-- {
		if !s.states[i].initialized {
			continue
		}
		err := closePlugin(cleanupCtx, s.chain[i])
		s.states[i].initialized = false
		if err != nil {
			errs = append(errs, errors.Wrap(err, "插件回滚失败: %s", s.chain[i].Name()))
		}
	}
	s.closed = true
	return errors.Join(errs...)
}

// safeInit 执行插件 Init 并把 panic 转为 error。
func safeInit(ctx context.Context, p Plugin, slot *PluginSlot) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Newf("插件初始化 panic: %v", r)
		}
	}()
	return p.Init(ctx, slot)
}

// closePlugin 在 ctx 内执行插件 Close；超时返回 ctx.Err()，不阻塞整条关闭链。
// 预算已耗尽时直接调用，让剩余插件仍有机会响应取消并做快速清理。
func closePlugin(ctx context.Context, p Plugin) error {
	if ctx.Err() != nil {
		return safeClose(ctx, p)
	}

	done := make(chan error, 1)
	go func() {
		done <- safeClose(ctx, p)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// safeClose 执行插件 Close 并把 panic 转为 error。
func safeClose(ctx context.Context, p Plugin) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Newf("插件关闭 panic: %v", r)
		}
	}()
	return p.Close(ctx)
}

// Plugin 按名称查找插件；插件未注册时返回 nil, false。
func (s *PluginSlot) Plugin(name string) (Plugin, bool) {
	if s == nil {
		return nil, false
	}
	idx, ok := s.index[strings.TrimSpace(name)]
	if !ok {
		return nil, false
	}
	return s.chain[idx], true
}

// MustPlugin 按名称查找插件；插件未注册时 panic。
func (s *PluginSlot) MustPlugin(name string) Plugin {
	p, ok := s.Plugin(name)
	if !ok {
		panic(errors.Newf("plugin: 未注册插件: %s", name))
	}
	return p
}

// Started 返回指定名称的插件是否已初始化；未注册或 Init 返回 ErrDisabled 时为 false。
func (s *PluginSlot) Started(name string) bool {
	if s == nil {
		return false
	}
	idx, ok := s.index[strings.TrimSpace(name)]
	return ok && s.states[idx].initialized
}

// Plugins 按链顺序返回插件副本。
func (s *PluginSlot) Plugins() []Plugin {
	if s == nil {
		return nil
	}
	return slices.Clone(s.chain)
}

// ReadyHooks 按 Init 顺序返回已初始化插件的就绪钩子。
func (s *PluginSlot) ReadyHooks() []func(context.Context) error {
	if s == nil {
		return nil
	}
	var hooks []func(context.Context) error
	for i, p := range s.chain {
		if !s.states[i].initialized {
			continue
		}
		if h, ok := p.(ReadyHook); ok {
			hooks = append(hooks, h.OnReady)
		}
	}
	return hooks
}

// DrainHooks 按 Init 逆序返回已初始化插件的排空钩子。
func (s *PluginSlot) DrainHooks() []func(context.Context) error {
	if s == nil {
		return nil
	}
	var hooks []func(context.Context) error
	for i := len(s.chain) - 1; i >= 0; i-- {
		if !s.states[i].initialized {
			continue
		}
		if h, ok := s.chain[i].(DrainHook); ok {
			hooks = append(hooks, h.OnDraining)
		}
	}
	return hooks
}

// orBackground 将 nil context 归一化为 Background。
func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
