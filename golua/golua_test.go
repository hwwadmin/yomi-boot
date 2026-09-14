// Package golua 集成测试
//
// @author zwei
package golua

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
	"gitee.com/hq_project/yomi-boot/redis/redistest"
)

type memoryAdapter struct {
	mu      sync.Mutex
	scripts map[string]*Script
}

func newMemoryAdapter(scripts ...*Script) *memoryAdapter {
	m := &memoryAdapter{scripts: make(map[string]*Script)}
	for _, s := range scripts {
		cp := *s
		m.scripts[cp.ID] = &cp
	}
	return m
}

func (m *memoryAdapter) Load(ctx context.Context) ([]*Script, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Script, 0, len(m.scripts))
	for _, s := range m.scripts {
		cp := *s
		out = append(out, &cp)
	}
	return out, nil
}

func (m *memoryAdapter) LoadByID(ctx context.Context, id string) (*Script, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.scripts[id]
	if !ok {
		return nil, errors.Sentinel(ErrNotFound)
	}
	cp := *s
	return &cp, nil
}

func (m *memoryAdapter) put(s *Script) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *s
	m.scripts[cp.ID] = &cp
}

func (m *memoryAdapter) delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.scripts, id)
}

func enabledCfg() Config {
	enabled := true
	refresh := false
	return Config{
		Enabled:              &enabled,
		RefreshOnStart:       &refresh,
		ExecTimeoutMS:        200,
		MaxInstructions:      1_000_000,
		PoolSize:             2,
		PoolAcquireTimeoutMS: 200,
		ScriptCacheSize:      16,
	}
}

// TestNew_Disabled 验证未启用时返回 ErrDisabled
func TestNew_Disabled(t *testing.T) {
	rdb, cleanup := redistest.NewClient(t)
	defer cleanup()
	_, err := New(context.Background(), Config{}, newMemoryAdapter(), rdb)
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("期望 ErrDisabled，实际 %v", err)
	}
}

// TestExecute_Basic 验证灌库后执行 main 返回结果
func TestExecute_Basic(t *testing.T) {
	rdb, cleanup := redistest.NewClient(t)
	defer cleanup()

	adapter := newMemoryAdapter(&Script{
		ID:      "add",
		Source:  `function main(args) return args.a + args.b end`,
		Version: "1",
	})
	comp, err := New(context.Background(), enabledCfg(), adapter, rdb)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close()

	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := comp.Engine.Execute(context.Background(), "add", map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	if got.(float64) != 3 {
		t.Fatalf("期望 3，实际 %v", got)
	}
}

// TestRefresh_SkipWhenReady 验证普通 Refresh 在缓存非空时跳过
func TestRefresh_SkipWhenReady(t *testing.T) {
	rdb, cleanup := redistest.NewClient(t)
	defer cleanup()

	adapter := newMemoryAdapter(&Script{
		ID:      "s1",
		Source:  `function main(args) return 1 end`,
		Version: "1",
	})
	comp, err := New(context.Background(), enabledCfg(), adapter, rdb)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close()

	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}
	adapter.put(&Script{
		ID:      "s1",
		Source:  `function main(args) return 99 end`,
		Version: "2",
	})
	if err := comp.StoreLoad.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := comp.Engine.Execute(context.Background(), "s1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(float64) != 1 {
		t.Fatalf("普通 Refresh 不应覆盖，期望 1，实际 %v", got)
	}
}

// TestRefreshScriptForce_UpdateAndInvalidate 验证强制单脚本刷新后执行新逻辑
func TestRefreshScriptForce_UpdateAndInvalidate(t *testing.T) {
	rdb, cleanup := redistest.NewClient(t)
	defer cleanup()

	adapter := newMemoryAdapter(&Script{
		ID:      "s1",
		Source:  `function main(args) return 1 end`,
		Version: "1",
	})
	comp, err := New(context.Background(), enabledCfg(), adapter, rdb)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close()

	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}
	adapter.put(&Script{
		ID:      "s1",
		Source:  `function main(args) return 2 end`,
		Version: "2",
	})
	if err := comp.StoreLoad.RefreshScriptForce(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	got, err := comp.Engine.Execute(context.Background(), "s1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(float64) != 2 {
		t.Fatalf("期望 2，实际 %v", got)
	}
}

// TestRefreshScriptForce_NotFoundDeletesCache 验证数据源删除后强制刷新清理缓存
func TestRefreshScriptForce_NotFoundDeletesCache(t *testing.T) {
	rdb, cleanup := redistest.NewClient(t)
	defer cleanup()

	adapter := newMemoryAdapter(&Script{
		ID:      "gone",
		Source:  `function main(args) return 1 end`,
		Version: "1",
	})
	comp, err := New(context.Background(), enabledCfg(), adapter, rdb)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close()

	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}
	adapter.delete("gone")
	if err := comp.StoreLoad.RefreshScriptForce(context.Background(), "gone"); err != nil {
		t.Fatal(err)
	}
	_, err = comp.Engine.Execute(context.Background(), "gone", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound，实际 %v", err)
	}
}

// TestExecute_Timeout 验证死循环触发执行超时
func TestExecute_Timeout(t *testing.T) {
	rdb, cleanup := redistest.NewClient(t)
	defer cleanup()

	adapter := newMemoryAdapter(&Script{
		ID:      "loop",
		Source:  `function main(args) while true do end end`,
		Version: "1",
	})
	cfg := enabledCfg()
	cfg.ExecTimeoutMS = 50
	comp, err := New(context.Background(), cfg, adapter, rdb)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close()
	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = comp.Engine.Execute(context.Background(), "loop", nil)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("期望 ErrTimeout，实际 %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("超时应较快返回，实际 %v", elapsed)
	}
}

// TestSandbox_NoOS 验证 os 库不可用
func TestSandbox_NoOS(t *testing.T) {
	rdb, cleanup := redistest.NewClient(t)
	defer cleanup()

	adapter := newMemoryAdapter(&Script{
		ID:      "os",
		Source:  `function main(args) return os.time() end`,
		Version: "1",
	})
	comp, err := New(context.Background(), enabledCfg(), adapter, rdb)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close()
	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = comp.Engine.Execute(context.Background(), "os", nil)
	if err == nil {
		t.Fatal("期望沙箱拒绝 os，实际成功")
	}
	if !errors.Is(err, ErrRuntime) {
		t.Fatalf("期望 ErrRuntime，实际 %v", err)
	}
}

// TestExecute_NoGlobalPollution 验证池复用时不会串用上一脚本的 main
func TestExecute_NoGlobalPollution(t *testing.T) {
	rdb, cleanup := redistest.NewClient(t)
	defer cleanup()

	adapter := newMemoryAdapter(
		&Script{
			ID:      "a",
			Source:  `function main(args) return 1 end`,
			Version: "1",
		},
		&Script{
			ID:      "b",
			Source:  `local x = 1`, // 故意不定义 main
			Version: "1",
		},
	)
	cfg := enabledCfg()
	cfg.PoolSize = 1
	comp, err := New(context.Background(), cfg, adapter, rdb)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close()
	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}

	got, err := comp.Engine.Execute(context.Background(), "a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(float64) != 1 {
		t.Fatalf("脚本 a 期望 1，实际 %v", got)
	}

	_, err = comp.Engine.Execute(context.Background(), "b", nil)
	if err == nil {
		t.Fatal("脚本 b 无 main 时应失败，不能复用 a 的 main")
	}
	if strings.Contains(err.Error(), "入口函数不存在") == false {
		t.Fatalf("期望入口不存在错误，实际 %v", err)
	}
}

// TestExecute_NoGWritePollution 验证经 _G 赋值不会污染后续请求
func TestExecute_NoGWritePollution(t *testing.T) {
	rdb, cleanup := redistest.NewClient(t)
	defer cleanup()

	adapter := newMemoryAdapter(
		&Script{
			ID:      "writer",
			Source:  `_G.leak = 42; function main(args) return 1 end`,
			Version: "1",
		},
		&Script{
			ID:      "reader",
			Source:  `function main(args) return _G.leak end`,
			Version: "1",
		},
	)
	cfg := enabledCfg()
	cfg.PoolSize = 1
	comp, err := New(context.Background(), cfg, adapter, rdb)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close()
	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, err := comp.Engine.Execute(context.Background(), "writer", nil); err != nil {
		t.Fatal(err)
	}
	got, err := comp.Engine.Execute(context.Background(), "reader", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("期望 _G.leak 未跨请求泄漏，实际 %v", got)
	}
}

// TestRefreshOnStart 验证启动普通刷新灌库
func TestRefreshOnStart(t *testing.T) {
	rdb, cleanup := redistest.NewClient(t)
	defer cleanup()

	enabled := true
	refresh := true
	cfg := enabledCfg()
	cfg.Enabled = &enabled
	cfg.RefreshOnStart = &refresh

	adapter := newMemoryAdapter(&Script{
		ID:      "boot",
		Source:  `function main(args) return "ok" end`,
		Version: "1",
	})
	comp, err := New(context.Background(), cfg, adapter, rdb)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close()

	got, err := comp.Engine.Execute(context.Background(), "boot", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(string) != "ok" {
		t.Fatalf("期望 ok，实际 %v", got)
	}
}
