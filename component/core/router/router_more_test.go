package router

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

type registrarFunc func(*gin.RouterGroup)

func (f registrarFunc) Routes(group *gin.RouterGroup) { f(group) }

func TestNewRouter_RegistersAndSkipsNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hit := false
	engine, err := NewRouter("/api", []Registrar{
		nil,
		registrarFunc(func(group *gin.RouterGroup) {
			group.GET("/ping", func(c *gin.Context) {
				hit = true
				c.Status(http.StatusNoContent)
			})
		}),
	}, []gin.HandlerFunc{
		func(c *gin.Context) { c.Next() },
	}, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/ping", nil))
	if !hit || w.Code != http.StatusNoContent {
		t.Fatalf("hit=%v code=%d", hit, w.Code)
	}
}

func TestNewRouter_BadProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if _, err := NewRouter("", nil, nil, []string{"bad cidr"}); err == nil {
		t.Fatal("非法可信代理应失败")
	}
}

func TestMustNewRouter_BadProxyPanics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	defer func() {
		if recover() == nil {
			t.Fatal("非法可信代理 MustNewRouter 应 panic")
		}
	}()
	MustNewRouter("", nil, nil, []string{"bad cidr"})
}

func TestConfigEdges(t *testing.T) {
	if err := (Config{}).Validate(); err == nil {
		t.Fatal("端口小于等于 0 应失败")
	}
	if err := (Config{Port: 1, MaxRequestBodyBytes: -1}).Validate(); err == nil {
		t.Fatal("请求体上限为负应失败")
	}
	if err := (Config{Port: 1, RouterPrefix: " /api"}).Validate(); err == nil {
		t.Fatal("前缀含首尾空格应失败")
	}
	cfg := Config{Port: 8080, RouterPrefix: " /api "}
	if cfg.RouterPrefixClean() != "/api" {
		t.Fatalf("RouterPrefixClean = %q", cfg.RouterPrefixClean())
	}
	if cfg.ListenAddr() != ":8080" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr())
	}
	if cfg.MaxRequestBodyBytesOrDefault() != defaultMaxRequestBodyBytes {
		t.Fatalf("默认请求体上限 = %d", cfg.MaxRequestBodyBytesOrDefault())
	}
	cfg.MaxRequestBodyBytes = 32
	if cfg.MaxRequestBodyBytesOrDefault() != 32 {
		t.Fatalf("请求体上限 = %d", cfg.MaxRequestBodyBytesOrDefault())
	}
}
