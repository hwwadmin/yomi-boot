package utf8util

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

func TestCut_UnlimitedKeepsFullValid(t *testing.T) {
	t.Parallel()
	raw := []byte("你好世界")
	got, truncated := Cut(raw, Unlimited)
	if truncated {
		t.Fatal("unlimited should not truncate")
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestCut_ASCIIWithinLimit(t *testing.T) {
	t.Parallel()
	raw := []byte("hello")
	got, truncated := Cut(raw, 16)
	if truncated {
		t.Fatal("want not truncated")
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestCut_ChineseMidRune(t *testing.T) {
	t.Parallel()
	// 错 = E9 94 99；切在前 2 字节必须回退，不能留下 \xE9\x94
	raw := []byte("报错")
	got, truncated := Cut(raw, 5) // 报(3) + 错的前 2 字节
	if !truncated {
		t.Fatal("want truncated")
	}
	if !utf8.Valid(got) {
		t.Fatalf("result is not valid UTF-8: %q %x", got, got)
	}
	if string(got) != "报" {
		t.Fatalf("got %q want 报", got)
	}
}

func TestCut_FourByteEmoji(t *testing.T) {
	t.Parallel()
	raw := []byte("ok😀")          // 2 + 4
	got, truncated := Cut(raw, 4) // ok + 😀 的前 2 字节
	if !truncated {
		t.Fatal("want truncated")
	}
	if !utf8.Valid(got) {
		t.Fatalf("result is not valid UTF-8: %x", got)
	}
	if string(got) != "ok" {
		t.Fatalf("got %q want ok", got)
	}
}

func TestCut_ExactRuneBoundary(t *testing.T) {
	t.Parallel()
	raw := []byte("你好")
	got, truncated := Cut(raw, 3)
	if !truncated {
		t.Fatal("want truncated")
	}
	if string(got) != "你" {
		t.Fatalf("got %q want 你", got)
	}
}

func TestCut_ZeroMaxNonEmpty(t *testing.T) {
	t.Parallel()
	got, truncated := Cut([]byte("abc"), 0)
	if !truncated {
		t.Fatal("want truncated")
	}
	if len(got) != 0 {
		t.Fatalf("got %q want empty", got)
	}
}

func TestCut_SanitizesInvalidWithoutSizeCap(t *testing.T) {
	t.Parallel()
	raw := []byte("ok\xE9\x94")
	got, truncated := Cut(raw, Unlimited)
	if truncated {
		t.Fatal("unlimited should not report truncated")
	}
	if !utf8.Valid(got) {
		t.Fatalf("result is not valid UTF-8: %x", got)
	}
	if string(got) != "ok" {
		t.Fatalf("got %q want ok", got)
	}
}
