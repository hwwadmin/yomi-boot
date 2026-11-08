package sm4

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/core/errors"
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

// TestGCMWithAAD 验证 AAD 绑定、不匹配拒绝及与无 AAD 接口的等价性
func TestGCMWithAAD(t *testing.T) {
	g, err := NewGCM(testKey)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("sm4 gcm aad")
	aad := []byte("tenant=42;key-version=1")

	sealed, err := g.SealWithAAD(plain, aad)
	if err != nil {
		t.Fatal(err)
	}
	got, err := g.OpenWithAAD(sealed, aad)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := g.OpenWithAAD(sealed, []byte("tenant=43;key-version=1")); !errors.Is(err, ErrOpen) {
		t.Fatalf("AAD 不匹配应失败: %v", err)
	}
	if _, err := g.Open(sealed); !errors.Is(err, ErrOpen) {
		t.Fatalf("缺少 AAD 应失败: %v", err)
	}

	sealedNoAAD, err := g.SealWithAAD(plain, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := g.Open(sealedNoAAD); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("nil AAD 应与 Open 等价: got %q err=%v", got, err)
	}

	sealedDefault, err := g.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := g.OpenWithAAD(sealedDefault, nil); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("Seal 应与 nil AAD 等价: got %q err=%v", got, err)
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

type oversizedBlock struct{}

func (oversizedBlock) BlockSize() int          { return 256 }
func (oversizedBlock) Encrypt(dst, src []byte) {}
func (oversizedBlock) Decrypt(dst, src []byte) {}

func TestSealAndOpen_RejectBadBlocks(t *testing.T) {
	if _, err := (&CBC{block: oversizedBlock{}}).Seal([]byte("a")); err == nil {
		t.Fatal("CBC 块长非法时应填充失败")
	}
	if _, err := (&ECB{block: oversizedBlock{}}).Seal([]byte("a")); err == nil {
		t.Fatal("ECB 块长非法时应填充失败")
	}
	ecb, err := NewECB(testKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ecb.Open([]byte{1, 2, 3}); !errors.Is(err, ErrOpen) {
		t.Fatalf("长度不是块整数倍时应失败: %v", err)
	}
}

func TestSM4RejectsBadInputs(t *testing.T) {
	if _, err := NewCBC(nil); err == nil {
		t.Fatal("空密钥应拒绝")
	}
	if _, err := (*CBC)(nil).Seal(nil); err == nil {
		t.Fatal("未初始化的 CBC 不能加密")
	}
	if _, err := (*CBC)(nil).Open(nil); err == nil {
		t.Fatal("未初始化的 CBC 不能解密")
	}
	cbc, err := NewCBC(make([]byte, 16))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cbc.Open([]byte{1}); err == nil {
		t.Fatal("过短密文应失败")
	}
	sealed, err := cbc.Seal([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 0xff
	if _, err := cbc.Open(sealed); err == nil {
		t.Fatal("损坏的填充应失败")
	}

	if _, err := NewECB([]byte("short")); err == nil {
		t.Fatal("短密钥应拒绝")
	}
	if _, err := (*ECB)(nil).Seal(nil); err == nil {
		t.Fatal("未初始化的 ECB 不能加密")
	}
	if _, err := (*ECB)(nil).Open(nil); err == nil {
		t.Fatal("未初始化的 ECB 不能解密")
	}
	ecb, err := NewECB(make([]byte, 16))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ecb.Open(make([]byte, 16)); err == nil {
		t.Fatal("非法填充应失败")
	}

	if _, err := NewGCM(nil); err == nil {
		t.Fatal("空 GCM 密钥应拒绝")
	}
	if _, err := (*GCM)(nil).SealWithAAD(nil, nil); err == nil {
		t.Fatal("未初始化的 GCM 不能加密")
	}
	if _, err := (*GCM)(nil).OpenWithAAD(nil, nil); err == nil {
		t.Fatal("未初始化的 GCM 不能解密")
	}
	gcm, err := NewGCM(make([]byte, 16))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gcm.Open([]byte("short")); err == nil {
		t.Fatal("过短密文应失败")
	}
	box, err := gcm.Seal([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	box[len(box)-1] ^= 0xff
	if _, err := gcm.Open(box); err == nil {
		t.Fatal("被篡改的密文应失败")
	}
}
