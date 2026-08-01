package logger

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSensitive_ShouldMask_ContainsAndExact 验证 contains/exact 两种匹配模式
func TestSensitive_ShouldMask_ContainsAndExact(t *testing.T) {
	contains, err := NewSensitive(SensitiveMatchContains, []string{"token", "password"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains.ShouldMask("access_token") {
		t.Fatal("contains should hit access_token")
	}
	if !contains.ShouldMask("PASSWORD") {
		t.Fatal("case insensitive")
	}
	if contains.ShouldMask("username") {
		t.Fatal("username should not mask")
	}

	exact, err := NewSensitive(SensitiveMatchExact, []string{"token"})
	if err != nil {
		t.Fatal(err)
	}
	if exact.ShouldMask("access_token") {
		t.Fatal("exact should not hit access_token")
	}
	if !exact.ShouldMask("token") {
		t.Fatal("exact should hit token")
	}
}

// TestSensitive_EmptyKeysPassthrough 验证 keys 为空时透传
func TestSensitive_EmptyKeysPassthrough(t *testing.T) {
	s, err := NewSensitive("", nil)
	if err != nil {
		t.Fatal(err)
	}
	kv := s.ApplyKV("password", "secret", "user", "a")
	if kv[1] != "secret" {
		t.Fatalf("empty keys should passthrough, got %v", kv[1])
	}
}

// TestSensitive_ApplyKVAndJSON 验证 ApplyKV 与 ApplyJSON 脱敏
func TestSensitive_ApplyKVAndJSON(t *testing.T) {
	s, err := NewSensitive(SensitiveMatchContains, []string{"password", "token"})
	if err != nil {
		t.Fatal(err)
	}
	kv := s.ApplyKV("password", "p@ss", "user", "a")
	if kv[1] != sensitivePlaceholder {
		t.Fatalf("password want redacted, got %v", kv[1])
	}
	got := s.ApplyJSON(`{"password":"x","nested":{"access_token":"t"},"ok":1}`)
	if !strings.Contains(got, `"password":"[REDACTED]"`) {
		t.Fatalf("json password: %s", got)
	}
	if !strings.Contains(got, `"access_token":"[REDACTED]"`) {
		t.Fatalf("json nested token: %s", got)
	}
	if !strings.Contains(got, `"ok":1`) {
		t.Fatalf("json ok preserved: %s", got)
	}
}

// TestSensitive_ApplyJSON_PreservesLargeIntegers 验证脱敏不丢失超过 float64 精确范围的整数
func TestSensitive_ApplyJSON_PreservesLargeIntegers(t *testing.T) {
	s, err := NewSensitive(SensitiveMatchContains, []string{"password"})
	if err != nil {
		t.Fatal(err)
	}
	got := s.ApplyJSON(`{"id":1891234567890123456,"password":"x"}`)
	if !strings.Contains(got, `"id":1891234567890123456`) {
		t.Fatalf("large integer precision lost: %s", got)
	}
	if !strings.Contains(got, `"password":"[REDACTED]"`) {
		t.Fatalf("password not redacted: %s", got)
	}
}

// TestSensitive_ApplyQuery 验证 query string 参数脱敏
func TestSensitive_ApplyQuery(t *testing.T) {
	s, err := NewSensitive(SensitiveMatchContains, []string{"token"})
	if err != nil {
		t.Fatal(err)
	}
	got := s.ApplyQuery("a=1&access_token=secret&b=2")
	if !strings.Contains(got, "access_token=%5BREDACTED%5D") && !strings.Contains(got, "access_token=[REDACTED]") {
		// Encode will escape brackets
		if !strings.Contains(got, "REDACTED") {
			t.Fatalf("query: %s", got)
		}
	}
	if !strings.Contains(got, "a=1") {
		t.Fatalf("query lost a: %s", got)
	}
}

// TestInfo_SensitiveKeysFromConfig 验证配置 SensitiveKeys 写入 info 日志时脱敏
func TestInfo_SensitiveKeysFromConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Dir:           dir,
		Level:         "info",
		ConsoleLevel:  "error",
		Timezone:      "Asia/Singapore",
		Rotation:      testRotation(),
		SensitiveKeys: []string{"password", "token"},
	}
	closeFn, err := InitLogger("test-svc", "test", cfg)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	Info(context.Background(), "login", "password", "secret", "user", "alice", "access_token", "tok")
	_ = Sync()

	infoPath := filepath.Join(dir, "info", "info.log")
	b, err := os.ReadFile(infoPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	if strings.Contains(content, `"password":"secret"`) {
		t.Fatalf("password not masked: %s", content)
	}
	if !strings.Contains(content, `"password":"[REDACTED]"`) {
		t.Fatalf("want redacted password: %s", content)
	}
	if strings.Contains(content, `"access_token":"tok"`) {
		t.Fatalf("access_token not masked: %s", content)
	}
	if !strings.Contains(content, `"user":"alice"`) {
		t.Fatalf("user should remain: %s", content)
	}
}

// TestConfig_Normalize_SensitiveMatchInvalid 验证非法 sensitive_match 被拒绝
func TestConfig_Normalize_SensitiveMatchInvalid(t *testing.T) {
	cfg := Config{Rotation: testRotation(), SensitiveMatch: "fuzzy"}
	if err := cfg.Normalize(); err == nil {
		t.Fatal("expected invalid sensitive_match error")
	}
}
