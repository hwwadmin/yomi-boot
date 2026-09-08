package httprecord

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestReadRequestBodyForRecord_NoTruncate 验证未截断时还原完整 body 且关闭 original
func TestReadRequestBodyForRecord_NoTruncate(t *testing.T) {
	raw := `{"userId":1}`
	closed := false
	original := &closeProbe{
		Reader: strings.NewReader(raw),
		onClose: func() {
			closed = true
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api", original)
	req.Body = original

	got, truncated := readRequestBodyForRecord(req, 4096)
	if truncated {
		t.Fatal("want not truncated")
	}
	if got != raw {
		t.Fatalf("record got %q want %q", got, raw)
	}
	if !closed {
		t.Fatal("original body should be closed when fully consumed")
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != raw {
		t.Fatalf("restored body got %q want %q", body, raw)
	}
}

// TestReadRequestBodyForRecord_Truncate 验证截断标记，且 handler 仍可读完整 body，Close 委托 original
func TestReadRequestBodyForRecord_Truncate(t *testing.T) {
	raw := bytes.Repeat([]byte("x"), 100)
	closed := false
	original := &closeProbe{
		Reader: bytes.NewReader(raw),
		onClose: func() {
			closed = true
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api", original)
	req.Body = original

	got, truncated := readRequestBodyForRecord(req, 16)
	if !truncated {
		t.Fatal("want truncated")
	}
	wantPrefix := string(raw[:16]) + truncatedSuffix
	if got != wantPrefix {
		t.Fatalf("record got %q want %q", got, wantPrefix)
	}
	if closed {
		t.Fatal("original should stay open until restored Body.Close")
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, raw) {
		t.Fatalf("restored full body len=%d want=%d", len(body), len(raw))
	}
	if err := req.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if !closed {
		t.Fatal("Close should delegate to original on truncated path")
	}
}

// TestReadRequestBodyForRecord_UTF8Boundary 验证按字节上限截断时回退到合法 UTF-8 边界
func TestReadRequestBodyForRecord_UTF8Boundary(t *testing.T) {
	raw := []byte("报错") // 报=3 字节，错=E9 94 99
	original := io.NopCloser(bytes.NewReader(raw))
	req := httptest.NewRequest(http.MethodPost, "/api", original)
	req.Body = original

	got, truncated := readRequestBodyForRecord(req, 5)
	if !truncated {
		t.Fatal("want truncated")
	}
	want := "报" + truncatedSuffix
	if got != want {
		t.Fatalf("record got %q want %q", got, want)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, raw) {
		t.Fatalf("handler body got %q want %q", body, raw)
	}
}

// TestReadRequestBodyForRecord_Unlimited 验证 maxSize<0 时采集完整请求体且不截断
func TestReadRequestBodyForRecord_Unlimited(t *testing.T) {
	raw := bytes.Repeat([]byte("中"), 3000)
	closed := false
	original := &closeProbe{
		Reader: bytes.NewReader(raw),
		onClose: func() {
			closed = true
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api", original)
	req.Body = original

	got, truncated := readRequestBodyForRecord(req, -1)
	if truncated {
		t.Fatal("unlimited should not truncate")
	}
	if got != string(raw) {
		t.Fatalf("record len=%d want %d", len(got), len(raw))
	}
	if !closed {
		t.Fatal("original should be closed after full consume")
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, raw) {
		t.Fatalf("restored body len=%d want=%d", len(body), len(raw))
	}
}

type closeProbe struct {
	io.Reader
	onClose func()
	closed  bool
}

func (c *closeProbe) Close() error {
	if !c.closed {
		c.closed = true
		if c.onClose != nil {
			c.onClose()
		}
	}
	return nil
}
