package aes

import "testing"

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
