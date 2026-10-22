package lifecycle

import (
	"context"
	"net"
	"net/http"
	"slices"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/boot/errors"
)

// TestRunContextPhases 验证 Start → Ready → Drain → Release 的阶段顺序
func TestRunContextPhases(t *testing.T) {
	var events []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := &App{
		Name:       "test",
		HTTPServer: &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()},
		OnReady: []Hook{
			func(context.Context) error {
				events = append(events, "ready")
				cancel()
				return nil
			},
		},
		OnDraining: []Hook{
			func(context.Context) error {
				events = append(events, "drain-1")
				return nil
			},
			func(context.Context) error {
				events = append(events, "drain-2")
				return nil
			},
		},
		StopTimeout: time.Second,
		Release:     func() { events = append(events, "release") },
	}

	if err := RunContext(ctx, func() *App { return app }); err != nil {
		t.Fatalf("RunContext 返回错误: %v", err)
	}

	want := []string{"ready", "drain-2", "drain-1", "release"}
	if !slices.Equal(events, want) {
		t.Fatalf("阶段顺序 = %v, want %v", events, want)
	}
}

// TestRunContextReadyFailure 验证 Ready 失败时仍执行 Drain 与 Release
func TestRunContextReadyFailure(t *testing.T) {
	var events []string
	app := &App{
		Name:       "test",
		HTTPServer: &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()},
		OnReady: []Hook{
			func(context.Context) error {
				events = append(events, "ready")
				return errors.New("ready failed")
			},
		},
		OnDraining: []Hook{
			func(context.Context) error {
				events = append(events, "drain")
				return nil
			},
		},
		StopTimeout: time.Second,
		Release:     func() { events = append(events, "release") },
	}

	if err := RunContext(context.Background(), func() *App { return app }); err == nil {
		t.Fatal("Ready 失败时 RunContext 应返回错误")
	}

	want := []string{"ready", "drain", "release"}
	if !slices.Equal(events, want) {
		t.Fatalf("阶段顺序 = %v, want %v", events, want)
	}
}

// TestRunContextBindFailure 验证监听失败时执行 Release
func TestRunContextBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer func() { _ = listener.Close() }()

	released := false
	app := &App{
		Name:       "test",
		HTTPServer: &http.Server{Addr: listener.Addr().String(), Handler: http.NewServeMux()},
		Release:    func() { released = true },
	}

	if err := RunContext(context.Background(), func() *App { return app }); err == nil {
		t.Fatal("端口被占用时 RunContext 应返回错误")
	}
	if !released {
		t.Fatal("监听失败后 Release 未执行")
	}
}

// TestRunContextNilApp 验证 factory 返回 nil 时返回错误
func TestRunContextNilApp(t *testing.T) {
	if err := RunContext(context.Background(), func() *App { return nil }); err == nil {
		t.Fatal("App 为 nil 时应返回错误")
	}
}

// TestRunContextCanceledBeforeFactory 验证 ctx 已取消时不构建 App
func TestRunContextCanceledBeforeFactory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	if err := RunContext(ctx, func() *App {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("RunContext 返回错误: %v", err)
	}
	if called {
		t.Fatal("ctx 已取消时不应调用 factory")
	}
}
