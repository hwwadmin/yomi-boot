package aes

import (
	"bytes"
	"testing"
	"yomi-boot/errors"
)

// TestGCMRoundTrip 验证 GCM 加解密往返及密文篡改检测
func TestGCMRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 32)
	g, err := NewGCM(key)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("hello aes gcm")
	sealed, err := g.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := g.Open(sealed)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("got %q err=%v", got, err)
	}
	sealed[len(sealed)-1] ^= 0xff
	if _, err := g.Open(sealed); !errors.Is(err, ErrOpen) {
		t.Fatalf("篡改应失败: %v", err)
	}
}

// TestCBCRoundTrip 验证 CBC 加解密往返
func TestCBCRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x22}, 16)
	c, err := NewCBC(key)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("cbc plain text!!")
	sealed, err := c.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Open(sealed)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("got %q err=%v", got, err)
	}
}

// TestECBRoundTripAndBlockPattern 验证 ECB 加解密往返及相同明文块产生相同密文块
func TestECBRoundTripAndBlockPattern(t *testing.T) {
	key := bytes.Repeat([]byte{0x33}, 16)
	e, err := NewECB(key)
	if err != nil {
		t.Fatal(err)
	}
	block := bytes.Repeat([]byte("1234567890abcdef"), 1) // 16 bytes
	plain := append(block, block...)
	ct, err := e.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	// PKCS7 会再补一块，前两块密文应相等（ECB）
	if !bytes.Equal(ct[:16], ct[16:32]) {
		t.Fatal("ECB 相同明文块应产生相同密文块")
	}
	got, err := e.Open(ct)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("got %q err=%v", got, err)
	}
}

// TestInvalidKey 验证非法密钥长度应失败
func TestInvalidKey(t *testing.T) {
	if _, err := NewGCM([]byte("short")); err == nil {
		t.Fatal("非法 key 应失败")
	}
}
