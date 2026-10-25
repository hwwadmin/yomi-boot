// HTTP 访问记录脱敏链路的单元测试
//
// @author zwei
package httprecord

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestIsSensitiveHeader 验证敏感请求头判定不区分大小写与空白
func TestIsSensitiveHeader(t *testing.T) {
	for _, key := range []string{"Authorization", "authorization", " AUTHORIZATION ", "Cookie", "SET-COOKIE"} {
		if !isSensitiveHeader(key) {
			t.Fatalf("请求头 %q 应判定为敏感", key)
		}
	}
	for _, key := range []string{"Content-Type", "X-Request-Id", ""} {
		if isSensitiveHeader(key) {
			t.Fatalf("请求头 %q 不应判定为敏感", key)
		}
	}
}

// TestIsSensitiveBodyKey 验证敏感 JSON 字段判定不区分大小写与空白
func TestIsSensitiveBodyKey(t *testing.T) {
	for _, key := range []string{"password", "Password", " PASSWORD ", "token", "access_token", "Refresh_Token", "oldPassword"} {
		if !isSensitiveBodyKey(key) {
			t.Fatalf("字段 %q 应判定为敏感", key)
		}
	}
	for _, key := range []string{"username", "name", ""} {
		if isSensitiveBodyKey(key) {
			t.Fatalf("字段 %q 不应判定为敏感", key)
		}
	}
}

// TestMaskJSONString 验证 JSON 字符串脱敏的命中、嵌套、数组与非 JSON 透传
func TestMaskJSONString(t *testing.T) {
	t.Run("非 JSON 原样返回", func(t *testing.T) {
		raw := "not a json"
		if got := maskJSONString(raw); got != raw {
			t.Fatalf("非 JSON 应原样返回，实际 %q", got)
		}
	})

	t.Run("空串原样返回", func(t *testing.T) {
		if got := maskJSONString("   "); got != "   " {
			t.Fatalf("空串应原样返回，实际 %q", got)
		}
	})

	t.Run("敏感字段脱敏且保留大数精度", func(t *testing.T) {
		raw := `{"name":"u","password":"p@ss","nested":{"token":"tk","keep":1},"list":[{"access_token":"a"}],"big":12345678901234567890}`
		got := maskJSONString(raw)
		for _, want := range []string{`"password":"***"`, `"token":"***"`, `"access_token":"***"`, `"keep":1`, `12345678901234567890`} {
			if !strings.Contains(got, want) {
				t.Fatalf("脱敏结果缺少 %s: %s", want, got)
			}
		}
		if strings.Contains(got, "p@ss") || strings.Contains(got, `"tk"`) {
			t.Fatalf("敏感值未脱敏: %s", got)
		}
	})
}

// TestMaskQueryString 验证 query string 敏感参数脱敏与解析失败透传
func TestMaskQueryString(t *testing.T) {
	got := maskQueryString("access_token=secret&q=1&refresh_token=r")
	if !strings.Contains(got, "q=1") {
		t.Fatalf("普通参数应保留: %s", got)
	}
	if strings.Contains(got, "secret") || strings.Contains(got, "=r&") {
		t.Fatalf("敏感参数未脱敏: %s", got)
	}
	if got := maskQueryString("%zz=broken"); got != "%zz=broken" {
		t.Fatalf("解析失败应原样返回，实际 %q", got)
	}
}

// TestCollectRequestHeaders 验证请求头收集与按需脱敏
func TestCollectRequestHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("Authorization", "Bearer abc")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("X-Multi", "a")
	req.Header.Add("X-Multi", "b")
	req.Header["X-Empty"] = []string{}

	masked := collectRequestHeaders(req, true)
	if masked["Authorization"] != "***" {
		t.Fatalf("敏感头应脱敏: %v", masked["Authorization"])
	}
	if masked["Content-Type"] != "application/json" {
		t.Fatalf("普通头应保留: %v", masked["Content-Type"])
	}
	if masked["X-Multi"] != "a,b" {
		t.Fatalf("多值头应合并: %v", masked["X-Multi"])
	}
	if _, ok := masked["X-Empty"]; ok {
		t.Fatal("空值头应跳过")
	}

	raw := collectRequestHeaders(req, false)
	if raw["Authorization"] != "Bearer abc" {
		t.Fatalf("不脱敏时应保留原值: %v", raw["Authorization"])
	}
	if collectRequestHeaders(nil, true) != nil {
		t.Fatal("nil 请求应返回 nil")
	}
}

// TestNormalizePath 验证路径规范化
func TestNormalizePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "", want: "/"},
		{in: "/", want: "/"},
		{in: " /a/b ", want: "/a/b"},
		{in: "/a//b", want: "/a/b"},
		{in: "/a/b/", want: "/a/b"},
	}
	for _, tc := range cases {
		if got := normalizePath(tc.in); got != tc.want {
			t.Fatalf("normalizePath(%q) 期望 %q，实际 %q", tc.in, tc.want, got)
		}
	}
}

// TestParseBizResponse 验证业务码解析（编码器已在 TestMain 注册）
func TestParseBizResponse(t *testing.T) {
	code, msg := parseBizResponse(`{"code":"OK","msg":"success"}`)
	if code != "OK" || msg != "success" {
		t.Fatalf("业务码解析不符: %q %q", code, msg)
	}
	if code, msg = parseBizResponse(`{"foo":1}`); code != "" || msg != "" {
		t.Fatalf("非业务响应应返回空: %q %q", code, msg)
	}
	if code, msg = parseBizResponse("not-json"); code != "" || msg != "" {
		t.Fatalf("非法 JSON 应返回空: %q %q", code, msg)
	}
}
