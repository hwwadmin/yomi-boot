// Package golua Engine 执行入口
//
// @author zwei
package golua

import (
	"context"
	"strings"
	"sync/atomic"

	"gitee.com/hq_project/yomi-boot/errors"

	lua "github.com/yuin/gopher-lua"
)

var (
	_ Engine            = (*engine)(nil)
	_ ScriptInvalidator = (*engine)(nil)
)

// Engine 脚本执行引擎
type Engine interface {
	// Execute 按脚本 ID 从 StoreQuery 读取并执行
	Execute(ctx context.Context, scriptID string, args map[string]any) (any, error)
	// Close 关闭池
	Close() error
}

// engine 是 Engine / ScriptInvalidator 的默认实现
type engine struct {
	cfg    Config
	query  StoreQuery
	pool   *statePool
	cache  *protoCache
	closed atomic.Bool
}

// newEngine 创建引擎（不触发 Refresh）
func newEngine(cfg Config, query StoreQuery) (*engine, error) {
	if query == nil {
		return nil, errors.New("golua: StoreQuery 不能为空")
	}
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	e := &engine{
		cfg:   cfg,
		query: query,
		cache: newProtoCache(cfg.ScriptCacheSize),
	}
	e.pool = newStatePool(cfg, func() (*lua.LState, error) {
		return newSandboxedState(cfg)
	})
	return e, nil
}

// Execute 执行脚本
func (e *engine) Execute(ctx context.Context, scriptID string, args map[string]any) (any, error) {
	if e == nil || e.closed.Load() {
		return nil, errors.Sentinel(ErrPoolClosed)
	}
	scriptID = strings.TrimSpace(scriptID)
	if scriptID == "" {
		return nil, errors.New("golua: 脚本 id 不能为空")
	}
	script, err := e.query.Get(ctx, scriptID)
	if err != nil {
		return nil, err
	}
	proto, err := e.cache.getOrCompile(script.ID, script.EffectiveVersion(), script.Source)
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
	if e.cfg.ExecTimeout() > 0 {
		execCtx, cancel = context.WithTimeout(ctx, e.cfg.ExecTimeout())
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

// InvalidateScripts 丢弃指定脚本编译缓存并 bump 池世代
func (e *engine) InvalidateScripts(ids ...string) {
	if e == nil {
		return
	}
	e.cache.removeIDs(ids...)
	e.pool.bumpEpoch()
}

// InvalidateAllScripts 丢弃全部编译缓存并 bump 池世代
func (e *engine) InvalidateAllScripts() {
	if e == nil {
		return
	}
	e.cache.clear()
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
	e.cache.clear()
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
