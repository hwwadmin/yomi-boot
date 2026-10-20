package oss

import (
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/errors"
)

// TestConfigValidate 验证 Config 构造时 provider 与必填项校验
func TestConfigValidate(t *testing.T) {
	t.Parallel()

	t.Run("unsupported provider", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{Provider: "oss"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rustfs requires endpoint", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{
			Provider: ProviderRustFS,
			RustFS: RustFSConfig{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("s3 requires region", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{
			Provider: ProviderS3,
			S3: S3Config{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("alioss requires region", func(t *testing.T) {
		t.Parallel()
		_, err := New(Config{
			Provider: ProviderAliOSS,
			AliOSS: AliOSSConfig{
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
			},
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestJoinAndParseURI 验证逻辑 URI 拼装与解析
func TestJoinAndParseURI(t *testing.T) {
	t.Parallel()

	uri, err := JoinURI(" avatars ", "/u/1.png")
	if err != nil || uri != "avatars/u/1.png" {
		t.Fatalf("JoinURI: uri=%q err=%v", uri, err)
	}

	b, k, err := ParseURI("avatars/u/1.png")
	if err != nil || b != "avatars" || k != "u/1.png" {
		t.Fatalf("ParseURI: bucket=%q key=%q err=%v", b, k, err)
	}

	b, k, err = ParseURI("/avatars/u/1.png")
	if err != nil || b != "avatars" || k != "u/1.png" {
		t.Fatalf("ParseURI leading slash: bucket=%q key=%q err=%v", b, k, err)
	}

	_, err = JoinURI("", "k")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty bucket: got %v", err)
	}
	_, err = JoinURI("b", "")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty key: got %v", err)
	}
	_, err = JoinURI("a/b", "k")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("bucket with slash: got %v", err)
	}
	_, _, err = ParseURI("")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty uri: got %v", err)
	}
	_, _, err = ParseURI("nobucket")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("uri without slash: got %v", err)
	}
	_, _, err = ParseURI("bucket/")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("uri empty key: got %v", err)
	}
}

// TestObjectURL 验证固定访问 URL 拼装
func TestObjectURL(t *testing.T) {
	t.Parallel()

	store := &rustfsStorage{publicBaseURL: "https://cdn.example.com/"}
	got, err := store.ObjectURL("avatars/u/1.png")
	if err != nil || got != "https://cdn.example.com/avatars/u/1.png" {
		t.Fatalf("ObjectURL: got=%q err=%v", got, err)
	}

	_, err = store.ObjectURL("bad")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("invalid uri: got %v", err)
	}

	store = &rustfsStorage{publicBaseURL: ""}
	_, err = store.ObjectURL("avatars/u/1.png")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("missing base: got %v", err)
	}
}

// TestPresignArgs 验证预签名参数与 uri 校验
func TestPresignArgs(t *testing.T) {
	t.Parallel()

	store := &rustfsStorage{maxPresignExpiry: time.Hour}
	_, err := store.PresignGet(t.Context(), "b/k", 0)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expiry<=0: got %v", err)
	}
	_, err = store.PresignGet(t.Context(), "b/k", 2*time.Hour)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expiry too large: got %v", err)
	}
	_, err = store.PresignGet(t.Context(), "bad", time.Minute)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("invalid uri: got %v", err)
	}
}

// TestRustFSEndpoint 验证 RustFS 端点 scheme 归一化
func TestRustFSEndpoint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		endpoint string
		useSSL   bool
		want     string
	}{
		{"plain without ssl", "127.0.0.1:9000", false, "http://127.0.0.1:9000"},
		{"plain with ssl", "127.0.0.1:9000", true, "https://127.0.0.1:9000"},
		{"scheme preserved", "http://rustfs:9000", true, "http://rustfs:9000"},
		{"trim spaces", " https://rustfs.example.com ", false, "https://rustfs.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := rustFSEndpoint(tc.endpoint, tc.useSSL); got != tc.want {
				t.Fatalf("rustFSEndpoint(%q, %v) = %q, want %q", tc.endpoint, tc.useSSL, got, tc.want)
			}
		})
	}
}

// TestConfigFromPublicBaseURL 验证 BootConfig 映射 PublicBaseURL
func TestConfigFromPublicBaseURL(t *testing.T) {
	t.Parallel()

	cfg := ConfigFrom(BootConfig{
		Provider:      "rustfs",
		PublicBaseURL: " https://cdn.example.com/ ",
		RustFS: BootRustFSConfig{
			Endpoint:        "127.0.0.1:9000",
			AccessKeyID:     "ak",
			SecretAccessKey: "sk",
		},
	})
	if cfg.PublicBaseURL != "https://cdn.example.com/" {
		t.Fatalf("PublicBaseURL: got %q", cfg.PublicBaseURL)
	}
}
