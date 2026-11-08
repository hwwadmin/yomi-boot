package appctx

import (
	"context"
	"testing"
	"time"
)

func TestAppAccessors(t *testing.T) {
	app := NewAppCtx("yomi", "test", "1.2.3")
	if app.Name() != "yomi" || app.Env() != "test" || app.Version() != "1.2.3" {
		t.Fatalf("app = %+v", app)
	}
	var nilApp *App
	if nilApp.Name() != "" || nilApp.Env() != "" || nilApp.Version() != "" {
		t.Fatal("nil App 访问器应返回空字符串")
	}
}

func TestConfigNormalizeAndValidateEdges(t *testing.T) {
	var nilCfg *Config
	nilCfg.Normalize()

	cfg := &Config{Name: "yomi", Timezone: " utc "}
	cfg.Normalize()
	if cfg.Timezone != DefaultTimezone {
		t.Fatalf("Timezone = %q", cfg.Timezone)
	}
	if err := (Config{}).Validate(); err == nil {
		t.Fatal("空 name 应失败")
	}
	if err := (Config{Name: "yomi"}).Validate(); err == nil {
		t.Fatal("空 timezone 应失败")
	}
	if err := (Config{Name: "yomi", Timezone: "Local"}).Validate(); err == nil {
		t.Fatal("Local 时区应失败")
	}
	if err := (Config{Name: "yomi", Timezone: "Not/AZone"}).Validate(); err == nil {
		t.Fatal("无效时区应失败")
	}
}

func TestContextHelpers(t *testing.T) {
	parent := context.Background()
	if OrBackground(parent) != parent {
		t.Fatal("非 nil context 应原样返回")
	}
	if bg := OrBackground(context.TODO()); bg == nil {
		t.Fatal("OrBackground 不应返回 nil")
	}

	ctx, cancel := WithCancel(context.TODO())
	cancel()
	<-ctx.Done()

	ctx, cancel = WithTimeout(context.TODO(), time.Hour)
	cancel()
	<-ctx.Done()

	ctx, cancel = BackgroundWithCancel()
	cancel()
	<-ctx.Done()

	ctx, cancel = BackgroundWithTimeout(time.Hour)
	cancel()
	<-ctx.Done()
}
