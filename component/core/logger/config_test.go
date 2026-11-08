package logger

import "testing"

func TestCompressOrDefault_ExplicitFalse(t *testing.T) {
	off := false
	if (RotationConfig{Compress: &off}).CompressOrDefault() {
		t.Fatal("显式 false 不应按默认压缩")
	}
}
