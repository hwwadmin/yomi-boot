package hash

import "testing"

// TestMD5Hex 验证 MD5 十六进制摘要及原始长度
func TestMD5Hex(t *testing.T) {
	// echo -n "hello" | md5sum
	got := MD5Hex([]byte("hello"))
	want := "5d41402abc4b2a76b9719d911017c592"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if len(MD5([]byte("hello"))) != 16 {
		t.Fatal("MD5 长度应为 16")
	}
}

// TestSHA1Hex 验证 SHA-1 标准测试向量及原始长度
func TestSHA1Hex(t *testing.T) {
	// echo -n "abc" | sha1sum
	got := SHA1Hex([]byte("abc"))
	want := "a9993e364706816aba3e25717850c26c9cd0d89d"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if len(SHA1([]byte("abc"))) != 20 {
		t.Fatal("SHA1 长度应为 20")
	}
}

// TestSHA256Hex 验证 SHA256 十六进制摘要及原始长度
func TestSHA256Hex(t *testing.T) {
	// echo -n "hello" | sha256sum
	got := SHA256Hex([]byte("hello"))
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if len(SHA256([]byte("hello"))) != 32 {
		t.Fatal("SHA256 长度应为 32")
	}
}

// TestSHA512Hex 验证 SHA512 十六进制摘要及原始长度
func TestSHA512Hex(t *testing.T) {
	// echo -n "hello" | sha512sum
	got := SHA512Hex([]byte("hello"))
	want := "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if len(SHA512([]byte("hello"))) != 64 {
		t.Fatal("SHA512 长度应为 64")
	}
}
