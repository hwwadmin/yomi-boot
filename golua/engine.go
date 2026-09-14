// Package golua Engine 执行入口（含 Redis 代际自愈）
//
// @author zwei
package golua

import (
	"context"
	"strings"
	"sync/atomic"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/logger"

	lua "github.com/yuin/gopher-lua"
)

var (
	_ Engine            = (*engine)(nil)
	_ ScriptInvalidator = (*engine)(nil)
)

// Engine 脚本执行引擎
type Engine interface {
	// Execute 按脚本 ID 从快照缓存读取（含 gen 对齐）后沙箱执行
	Execute(ctx context.Context, scriptID string, args map[string]any) (any, error)
	// Close 关闭池
	Close() error
}

// engine 是 Engine / ScriptInvalidator 的默认实现
type engine struct {
	cfg          Config
	cache        scriptCache
	pool         *statePool
	proto        *protoCache
	closed       atomic.Bool
	genSync      bool // Redis 后端启用 current_gen 水位线
	genWatermark atomic.Uint64
}

// newEngine 创建引擎（不触发 Refresh）
func newEngine(cfg Config, cache scriptCache) (*engine, error) {
	if cache == nil {
		return nil, errors.New("golua: scriptCache 不能为空")
	}
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	_, genSync := cache.(*redisScriptCache)
	e := &engine{
		cfg:     cfg,
		cache:   cache,
		proto:   newProtoCache(cfg.ProtoCacheSize),
		genSync: genSync,
	}
	e.pool = newStatePool(cfg, func() (*lua.LState, error) {
		return newSandboxedState(cfg)
	})
	return e, nil
}

// Execute 从快照缓存读取脚本（Redis 下含 current_gen 对齐），再沙箱执行
func (e *engine) Execute(ctx context.Context, scriptID string, args map[string]any) (any, error) {
	if e == nil || e.closed.Load() {
		return nil, errors.Sentinel(ErrPoolClosed)
	}
	scriptID = strings.TrimSpace(scriptID)
	if scriptID == "" {
		return nil, errors.New("golua: 脚本 id 不能为空")
	}

	script, gen, genOK, err := e.cache.GetWithGen(ctx, scriptID)
	if err != nil {
		return nil, err
	}
	e.alignGeneration(ctx, gen, genOK)

	proto, err := e.proto.getOrCompile(script.ID, script.ContentHash(), script.Source)
	if err != nil {
		return nil, errors.Wrap(errors.Sentinel(ErrCompile), "%s", err.Error())
	}

	st, err := e.pool.borrow(ctx)
	if err != nil {
		return nil, err
	}
	defer e.pool.returnState(st)

	execCtx := ctx
	cancel := func() {}
	if e.cfg.ExecTimeoutOrDefault() > 0 {
		execCtx, cancel = context.WithTimeout(ctx, e.cfg.ExecTimeoutOrDefault())
	}
	defer cancel()
	st.L.SetContext(execCtx)

	// 每请求隔离 env：脚本全局写不进池内 _G，避免串剧本
	env := newExecEnv(st.L)
	fn := st.L.NewFunctionFromProto(proto)
	st.L.SetFEnv(fn, env)
	st.L.Push(fn)
	if err := st.L.PCall(0, 0, nil); err != nil {
		return nil, mapExecErr(err)
	}
	st.L.SetTop(0)

	entry := e.cfg.EntryFuncOrDefault()
	lv := env.RawGetString(entry)
	fnEntry, ok := lv.(*lua.LFunction)
	if !ok || fnEntry == nil {
		return nil, errors.Newf("golua: 入口函数不存在: %s", entry)
	}

	argsTbl, err := toLuaValue(st.L, args, e.cfg, 0)
	if err != nil {
		return nil, err
	}
	st.L.Push(fnEntry)
	st.L.Push(argsTbl)
	if err := st.L.PCall(1, 1, nil); err != nil {
		return nil, mapExecErr(err)
	}
	ret := st.L.Get(-1)
	st.L.Pop(1)
	return fromLuaValue(ret, e.cfg, 0)
}

// alignGeneration Redis 代际落后时全量失效本地 Proto/池
func (e *engine) alignGeneration(ctx context.Context, remoteGen uint64, genOK bool) {
	if e == nil || !e.genSync {
		return
	}
	if !genOK {
		logger.Warn(ctx, "golua: 读取 current_gen 失败，依赖源码哈希对齐编译缓存")
		return
	}
	for {
		local := e.genWatermark.Load()
		if remoteGen <= local {
			return
		}
		e.InvalidateAllScripts()
		if e.genWatermark.CompareAndSwap(local, remoteGen) {
			return
		}
	}
}

// ObserveGeneration 抬高本机代际水位（不回退）
func (e *engine) ObserveGeneration(gen uint64) {
	if e == nil || !e.genSync {
		return
	}
	for {
		local := e.genWatermark.Load()
		if gen <= local {
			return
		}
		if e.genWatermark.CompareAndSwap(local, gen) {
			return
		}
	}
}

// InvalidateScripts 丢弃指定脚本编译缓存并 bump 池世代
func (e *engine) InvalidateScripts(ids ...string) {
	if e == nil {
		return
	}
	e.proto.removeIDs(ids...)
	e.pool.bumpEpoch()
}

// InvalidateAllScripts 丢弃全部编译缓存并 bump 池世代
func (e *engine) InvalidateAllScripts() {
	if e == nil {
		return
	}
	e.proto.clear()
	e.pool.bumpEpoch()
}

// Close 关闭引擎；可重复调用
func (e *engine) Close() error {
	if e == nil {
		return nil
	}
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}
	e.pool.close()
	e.proto.clear()
	return nil
}

// mapExecErr 将 gopher-lua / context 错误映射为包内哨兵
func mapExecErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(msg, context.DeadlineExceeded.Error()) ||
		strings.Contains(msg, context.Canceled.Error()) {
		return errors.Wrap(errors.Sentinel(ErrTimeout), "%s", msg)
	}
	if strings.Contains(msg, "instruction") {
		return errors.Wrap(errors.Sentinel(ErrInstructionLimit), "%s", msg)
	}
	return errors.Wrap(errors.Sentinel(ErrRuntime), "%s", msg)
}
