// Package ginutil IP 工具函数的单元测试
//
// @author zwei
package ginutil

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestClientIP_ignoresSpoofedHeaderWithoutTrustedProxy 验证未配置可信代理时 ClientIP 忽略 X-Forwarded-For 头，返回 RemoteAddr
func TestClientIP_ignoresSpoofedHeaderWithoutTrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建未配置可信代理的 engine
	engine := gin.New()
	if err := engine.SetTrustedProxies(nil); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}

	var gotIP string
	engine.GET("/", func(c *gin.Context) {
		gotIP = ClientIP(c)
	})

	// 构造带有伪造 X-Forwarded-For 的请求
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	engine.ServeHTTP(w, req)

	// 未配置可信代理时应返回 RemoteAddr
	if gotIP != "203.0.113.10" {
		t.Fatalf("ClientIP() = %q, want %q", gotIP, "203.0.113.10")
	}
}

// TestClientIP_usesForwardedForFromTrustedProxy 验证配置可信代理后 ClientIP 正确返回 X-Forwarded-For 中的客户端 IP
func TestClientIP_usesForwardedForFromTrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建配置了可信代理的 engine
	engine := gin.New()
	if err := engine.SetTrustedProxies([]string{"127.0.0.1"}); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}

	var gotIP string
	engine.GET("/", func(c *gin.Context) {
		gotIP = ClientIP(c)
	})

	// 构造来自可信代理的请求
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "198.51.100.20")
	engine.ServeHTTP(w, req)

	// 配置了可信代理时应返回 X-Forwarded-For 中的真实 IP
	if gotIP != "198.51.100.20" {
		t.Fatalf("ClientIP() = %q, want %q", gotIP, "198.51.100.20")
	}
}
