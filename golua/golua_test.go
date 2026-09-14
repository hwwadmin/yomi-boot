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

	"gitee.com/hq_project/yomi-boot/distlock"
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

func testCfg() Config {
	return Config{
		ExecTimeout:        200 * time.Millisecond,
		MaxInstructions:    1_000_000,
		PoolSize:           2,
		PoolAcquireTimeout: 200 * time.Millisecond,
		ScriptCacheSize:    16,
		SkipRefreshOnNew:   true,
	}
}

func redisTestCfg(t *testing.T, prefix string) Config {
	t.Helper()
	rdb, cleanup := redistest.NewClient(t)
	t.Cleanup(cleanup)
	factory, err := distlock.NewFactory(rdb)
	if err != nil {
		t.Fatal(err)
	}
	cfg := testCfg()
	cfg.CacheBackend = CacheRedis
	cfg.Redis = rdb
	cfg.DistLockFactory = factory
	cfg.RedisKeyPrefix = prefix
	return cfg
}

func newTestComp(t *testing.T, adapter StoreLoadAdapter, cfg Config) *Components {
	t.Helper()
	comp, err := New(context.Background(), cfg, adapter)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = comp.Close() })
	return comp
}

// TestNew_RedisRequiresDeps 验证 redis 后端依赖必填
func TestNew_RedisRequiresDeps(t *testing.T) {
	_, err := New(context.Background(), Config{CacheBackend: CacheRedis}, newMemoryAdapter())
	if err == nil {
		t.Fatal("期望 CacheBackend=redis 缺依赖时报错")
	}
}

// TestExecute_Basic 验证内存缓存灌库后执行
func TestExecute_Basic(t *testing.T) {
	adapter := newMemoryAdapter(&Script{
		ID:      "add",
		Source:  `function main(args) return args.a + args.b end`,
		Version: "1",
	})
	comp := newTestComp(t, adapter, testCfg())
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
	adapter := newMemoryAdapter(&Script{
		ID:      "s1",
		Source:  `function main(args) return 1 end`,
		Version: "1",
	})
	comp := newTestComp(t, adapter, testCfg())
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

// TestRefreshScriptForce_UpdateAndInvalidate 验证强制单脚本刷新
func TestRefreshScriptForce_UpdateAndInvalidate(t *testing.T) {
	adapter := newMemoryAdapter(&Script{
		ID:      "s1",
		Source:  `function main(args) return 1 end`,
		Version: "1",
	})
	comp := newTestComp(t, adapter, testCfg())
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

// TestRefreshScriptForce_NotFoundDeletesCache 验证删除脚本
func TestRefreshScriptForce_NotFoundDeletesCache(t *testing.T) {
	adapter := newMemoryAdapter(&Script{
		ID:      "gone",
		Source:  `function main(args) return 1 end`,
		Version: "1",
	})
	comp := newTestComp(t, adapter, testCfg())
	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}
	adapter.delete("gone")
	if err := comp.StoreLoad.RefreshScriptForce(context.Background(), "gone"); err != nil {
		t.Fatal(err)
	}
	_, err := comp.Engine.Execute(context.Background(), "gone", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound，实际 %v", err)
	}
}

// TestExecute_Timeout 验证死循环超时
func TestExecute_Timeout(t *testing.T) {
	adapter := newMemoryAdapter(&Script{
		ID:      "loop",
		Source:  `function main(args) while true do end end`,
		Version: "1",
	})
	cfg := testCfg()
	cfg.ExecTimeout = 50 * time.Millisecond
	comp := newTestComp(t, adapter, cfg)
	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := comp.Engine.Execute(context.Background(), "loop", nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("期望 ErrTimeout，实际 %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("超时应较快返回")
	}
}

// TestSandbox_NoOS 验证 os 不可用
func TestSandbox_NoOS(t *testing.T) {
	adapter := newMemoryAdapter(&Script{
		ID:      "os",
		Source:  `function main(args) return os.time() end`,
		Version: "1",
	})
	comp := newTestComp(t, adapter, testCfg())
	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := comp.Engine.Execute(context.Background(), "os", nil)
	if err == nil {
		t.Fatal("期望沙箱拒绝 os")
	}
	if !errors.Is(err, ErrRuntime) {
		t.Fatalf("期望 ErrRuntime，实际 %v", err)
	}
}

// TestExecute_NoGlobalPollution 验证不串用旧 main
func TestExecute_NoGlobalPollution(t *testing.T) {
	adapter := newMemoryAdapter(
		&Script{ID: "a", Source: `function main(args) return 1 end`, Version: "1"},
		&Script{ID: "b", Source: `local x = 1`, Version: "1"},
	)
	cfg := testCfg()
	cfg.PoolSize = 1
	comp := newTestComp(t, adapter, cfg)
	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := comp.Engine.Execute(context.Background(), "a", nil); err != nil {
		t.Fatal(err)
	}
	_, err := comp.Engine.Execute(context.Background(), "b", nil)
	if err == nil || !strings.Contains(err.Error(), "入口函数不存在") {
		t.Fatalf("期望入口不存在，实际 %v", err)
	}
}

// TestExecute_NoGWritePollution 验证 _G 不跨请求泄漏
func TestExecute_NoGWritePollution(t *testing.T) {
	adapter := newMemoryAdapter(
		&Script{ID: "writer", Source: `_G.leak = 42; function main(args) return 1 end`, Version: "1"},
		&Script{ID: "reader", Source: `function main(args) return _G.leak end`, Version: "1"},
	)
	cfg := testCfg()
	cfg.PoolSize = 1
	comp := newTestComp(t, adapter, cfg)
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
		t.Fatalf("期望无泄漏，实际 %v", got)
	}
}

// TestRefreshOnNew 验证 New 默认普通 Refresh
func TestRefreshOnNew(t *testing.T) {
	adapter := newMemoryAdapter(&Script{
		ID:      "boot",
		Source:  `function main(args) return "ok" end`,
		Version: "1",
	})
	cfg := testCfg()
	cfg.SkipRefreshOnNew = false
	comp := newTestComp(t, adapter, cfg)
	got, err := comp.Engine.Execute(context.Background(), "boot", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(string) != "ok" {
		t.Fatalf("期望 ok，实际 %v", got)
	}
}

// TestRedisScriptCache_Execute 验证 Redis 缓存路径（写路径带 distlock）
func TestRedisScriptCache_Execute(t *testing.T) {
	cfg := redisTestCfg(t, "yomi:golua:test")
	adapter := newMemoryAdapter(&Script{
		ID:      "r1",
		Source:  `function main(args) return 7 end`,
		Version: "1",
	})
	comp := newTestComp(t, adapter, cfg)
	if err := comp.StoreLoad.RefreshForce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := comp.Engine.Execute(context.Background(), "r1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(float64) != 7 {
		t.Fatalf("期望 7，实际 %v", got)
	}
}

// TestRedisScriptCache_BusinessErrNotWrappedAsLock 验证临界区业务错误不伪装成写锁失败
func TestRedisScriptCache_BusinessErrNotWrappedAsLock(t *testing.T) {
	cfg := redisTestCfg(t, "yomi:golua:test:bizerr")
	cache, err := newRedisScriptCache(cfg.Redis, cfg.RedisKeyPrefix, cfg.DistLockFactory)
	if err != nil {
		t.Fatal(err)
	}
	err = cache.ReplaceAll(context.Background(), []*Script{
		{ID: "dup", Source: `function main(args) return 1 end`, Version: "1"},
		{ID: "dup", Source: `function main(args) return 2 end`, Version: "2"},
	})
	if err == nil {
		t.Fatal("期望 id 重复报错")
	}
	if strings.Contains(err.Error(), "写锁失败") {
		t.Fatalf("业务错误不应包装成写锁失败: %v", err)
	}
	if !strings.Contains(err.Error(), "脚本 id 重复") {
		t.Fatalf("期望包含 id 重复信息，实际 %v", err)
	}
}

// TestRedisScriptCache_ReplaceAllFromHoldsLockAcrossLoad 验证 Load 在写锁内执行
func TestRedisScriptCache_ReplaceAllFromHoldsLockAcrossLoad(t *testing.T) {
	cfg := redisTestCfg(t, "yomi:golua:test:from")
	cache, err := newRedisScriptCache(cfg.Redis, cfg.RedisKeyPrefix, cfg.DistLockFactory)
	if err != nil {
		t.Fatal(err)
	}

	loadStarted := make(chan struct{})
	loadContinue := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		errCh <- cache.ReplaceAllFrom(context.Background(), func(ctx context.Context) ([]*Script, error) {
			close(loadStarted)
			<-loadContinue
			return []*Script{{
				ID: "f1", Source: `function main(args) return 1 end`, Version: "1",
			}}, nil
		})
	}()

	<-loadStarted
	// Load 期间另一把同 key 锁应拿不到（短超时）
	other := cfg.DistLockFactory.GetLock(cfg.RedisKeyPrefix+":lock", distlock.WithTTL(5*time.Second))
	tryCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := other.Lock(tryCtx); err == nil {
		_ = other.Unlock(context.Background())
		close(loadContinue)
		t.Fatal("Load 进行中期望写锁仍被占用")
	}
	close(loadContinue)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}
