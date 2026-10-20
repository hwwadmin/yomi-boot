package sm4

import (
	"bytes"
	"encoding/hex"
	"testing"

	"gitee.com/hq_project/yomi-boot/errors"
)

var testKey = []byte("0123456789abcdef")

// TestECBStandardVector 验证 GB/T 32907-2016 单块标准向量
func TestECBStandardVector(t *testing.T) {
	// 密钥与明文均为 0123456789abcdeffedcba9876543210
	key, _ := hex.DecodeString("0123456789abcdeffedcba9876543210")
	plain, _ := hex.DecodeString("0123456789abcdeffedcba9876543210")
	c, err := NewECB(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := c.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(sealed[:16]); got != "681edf34d206965e86b3e94f536e4246" {
		t.Fatalf("got %s", got)
	}
	got, err := c.Open(sealed)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("ecb roundtrip got %x err=%v", got, err)
	}
}

// TestCBCRoundTrip 验证 CBC 加解密与篡改检测
func TestCBCRoundTrip(t *testing.T) {
	c, err := NewCBC(testKey)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("sm4-cbc-hello")
	sealed, err := c.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Open(sealed)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("cbc roundtrip got %q err=%v", got, err)
	}
	if _, err := c.Open(sealed[:len(sealed)-1]); !errors.Is(err, ErrOpen) {
		t.Fatalf("期望 ErrOpen，实际 %v", err)
	}
}

// TestGCMRoundTrip 验证 GCM 加解密、篡改检测与密钥长度校验
func TestGCMRoundTrip(t *testing.T) {
	g, err := NewGCM(testKey)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("sm4-gcm-hello")
	sealed, err := g.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := g.Open(sealed)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("gcm roundtrip got %q err=%v", got, err)
	}
	sealed[len(sealed)-1] ^= 0xff
	if _, err := g.Open(sealed); !errors.Is(err, ErrOpen) {
		t.Fatalf("期望 ErrOpen，实际 %v", err)
	}
	if _, err := NewGCM([]byte("short")); err == nil {
		t.Fatal("非法密钥长度应失败")
	}
}
