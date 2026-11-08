package aes

import (
	"bytes"
	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"testing"
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

// TestGCMWithAAD 验证 AAD 绑定、AAD 不匹配拒绝及与无 AAD 接口的等价性
func TestGCMWithAAD(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 32)
	g, err := NewGCM(key)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("hello aes gcm aad")
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

// oversizedBlock 的块长超出 PKCS7 上限，用来走到填充失败分支。
type oversizedBlock struct{}

func (oversizedBlock) BlockSize() int          { return 256 }
func (oversizedBlock) Encrypt(dst, src []byte) {}
func (oversizedBlock) Decrypt(dst, src []byte) {}

func TestSeal_RejectsOversizedBlock(t *testing.T) {
	if _, err := (&CBC{block: oversizedBlock{}}).Seal([]byte("a")); err == nil {
		t.Fatal("CBC 块长非法时应填充失败")
	}
	if _, err := (&ECB{block: oversizedBlock{}}).Seal([]byte("a")); err == nil {
		t.Fatal("ECB 块长非法时应填充失败")
	}
}

func TestAESRejectsBadInputs(t *testing.T) {
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
	if _, err := cbc.Open([]byte{1, 2, 3}); err == nil {
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
	if _, err := pkcs7Pad(nil, 0); err == nil {
		t.Fatal("非法块大小应失败")
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
	if _, err := ecb.Open(nil); err == nil {
		t.Fatal("空密文应失败")
	}
	badBlock := make([]byte, 16)
	if _, err := ecb.Open(badBlock); err == nil {
		t.Fatal("非法填充应失败")
	}

	if _, err := NewGCM(make([]byte, 15)); err == nil {
		t.Fatal("非法 GCM 密钥应拒绝")
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
	if _, err := gcm.OpenWithAAD([]byte("short"), nil); err == nil {
		t.Fatal("过短 GCM 密文应失败")
	}
	box, err := gcm.SealWithAAD([]byte("hello"), []byte("aad"))
	if err != nil {
		t.Fatal(err)
	}
	box[len(box)-1] ^= 0xff
	if _, err := gcm.OpenWithAAD(box, []byte("aad")); err == nil {
		t.Fatal("被篡改的 GCM 密文应失败")
	}
}
