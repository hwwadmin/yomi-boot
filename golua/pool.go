// Package golua LState 池
//
// @author zwei
package golua

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"

	lua "github.com/yuin/gopher-lua"
)

// pooledState 池内 LState 及其创建世代
type pooledState struct {
	L     *lua.LState
	epoch uint64
}

// statePool 可复用 LState 池
type statePool struct {
	cfg    Config
	epoch  atomic.Uint64
	mu     sync.Mutex
	idle   []*pooledState
	tokens chan struct{}
	closed atomic.Bool
	create func() (*lua.LState, error)
}

// newStatePool 创建状态池
func newStatePool(cfg Config, create func() (*lua.LState, error)) *statePool {
	tokens := make(chan struct{}, cfg.PoolSize)
	for i := 0; i < cfg.PoolSize; i++ {
		tokens <- struct{}{}
	}
	p := &statePool{
		cfg:    cfg,
		idle:   make([]*pooledState, 0, cfg.PoolSize),
		tokens: tokens,
		create: create,
	}
	p.epoch.Store(1)
	return p
}

// bumpEpoch 提升世代并立即丢弃空闲旧 State
func (p *statePool) bumpEpoch() {
	p.epoch.Add(1)
	p.mu.Lock()
	defer p.mu.Unlock()
	cur := p.epoch.Load()
	kept := p.idle[:0]
	for _, st := range p.idle {
		if st.epoch == cur {
			kept = append(kept, st)
			continue
		}
		st.L.Close()
	}
	p.idle = kept
}

// borrow 获取一个与当前 epoch 匹配的 LState；超时返回 ErrPoolAcquireTimeout
func (p *statePool) borrow(ctx context.Context) (*pooledState, error) {
	if p.closed.Load() {
		return nil, errors.Sentinel(ErrPoolClosed)
	}
	timer := time.NewTimer(p.cfg.PoolAcquireTimeoutOrDefault())
	defer timer.Stop()
	select {
	case <-p.tokens:
	case <-ctx.Done():
		return nil, errors.Wrap(ctx.Err(), "golua: 借池被取消")
	case <-timer.C:
		return nil, errors.Sentinel(ErrPoolAcquireTimeout)
	}
	if p.closed.Load() {
		p.releaseToken()
		return nil, errors.Sentinel(ErrPoolClosed)
	}
	st, err := p.takeOrCreate()
	if err != nil {
		p.releaseToken()
		return nil, err
	}
	return st, nil
}

// takeOrCreate 从空闲队列取出或新建 State
func (p *statePool) takeOrCreate() (*pooledState, error) {
	p.mu.Lock()
	cur := p.epoch.Load()
	for len(p.idle) > 0 {
		st := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]
		if st.epoch != cur {
			st.L.Close()
			continue
		}
		p.mu.Unlock()
		return st, nil
	}
	p.mu.Unlock()

	L, err := p.create()
	if err != nil {
		return nil, err
	}
	return &pooledState{L: L, epoch: p.epoch.Load()}, nil
}

// returnState 归还 State；epoch 落后或已关闭则销毁且不回池
func (p *statePool) returnState(st *pooledState) {
	if st == nil || st.L == nil {
		return
	}
	resetState(st.L)
	if p.closed.Load() || st.epoch != p.epoch.Load() {
		st.L.Close()
		p.releaseToken()
		return
	}
	p.mu.Lock()
	if p.closed.Load() || st.epoch != p.epoch.Load() {
		p.mu.Unlock()
		st.L.Close()
		p.releaseToken()
		return
	}
	p.idle = append(p.idle, st)
	p.mu.Unlock()
	p.releaseToken()
}

// releaseToken 归还借池信号量
func (p *statePool) releaseToken() {
	select {
	case p.tokens <- struct{}{}:
	default:
	}
}

// close 关闭池并销毁全部空闲 State
func (p *statePool) close() {
	if !p.closed.CompareAndSwap(false, true) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, st := range p.idle {
		st.L.Close()
	}
	p.idle = nil
}

// resetState 清除执行上下文与临时全局
func resetState(L *lua.LState) {
	if L == nil {
		return
	}
	L.RemoveContext()
	// 共享 _G 只读，用 RawSet 清临时键
	if g, ok := L.Get(lua.GlobalsIndex).(*lua.LTable); ok && g != nil {
		g.RawSetString("args", lua.LNil)
		g.RawSetString("__golua_result", lua.LNil)
	}
}
