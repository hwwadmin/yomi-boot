package httputil

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"
)

// TestReplayRequestBody_FullyConsumed 验证未截断时关闭 original 并用内存 Body 还原
func TestReplayRequestBody_FullyConsumed(t *testing.T) {
	raw := []byte(`{"ok":true}`)
	original := io.NopCloser(bytes.NewReader(raw))
	req := httptest.NewRequest(http.MethodPost, "/x", original)
	req.Body = original

	recorded, err := io.ReadAll(io.LimitReader(original, int64(len(raw))+1))
	if err != nil {
		t.Fatal(err)
	}
	ReplayRequestBody(req, original, recorded, len(raw))

	got, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("body mismatch: got %q want %q", got, raw)
	}
	// 再次 Close 应安全（NopCloser / NoBody）
	if err := req.Body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestReplayRequestBody_TruncatedDelegatesClose 验证截断路径 Close 委托给 original
func TestReplayRequestBody_TruncatedDelegatesClose(t *testing.T) {
	closed := false
	original := &closeTracker{
		Reader: strings.NewReader("abcdefghij"),
		onClose: func() {
			closed = true
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/x", original)
	req.Body = original

	maxSize := 4
	recorded, err := io.ReadAll(io.LimitReader(original, int64(maxSize)+1))
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) <= maxSize {
		t.Fatalf("expected truncation peek, len=%d", len(recorded))
	}
	ReplayRequestBody(req, original, recorded, maxSize)

	got, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abcdefghij" {
		t.Fatalf("full body want abcdefghij got %q", got)
	}
	if err := req.Body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !closed {
		t.Fatal("Close should delegate to original")
	}
}

type closeTracker struct {
	io.Reader
	onClose func()
	closed  bool
}

func (c *closeTracker) Close() error {
	if !c.closed {
		c.closed = true
		if c.onClose != nil {
			c.onClose()
		}
	}
	return nil
}

// TestReplayRequestBody_KeepAliveReuse 验证还原后 HTTP/1.1 连接可复用
func TestReplayRequestBody_KeepAliveReuse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		original := r.Body
		const maxSize = 64
		recorded, err := io.ReadAll(io.LimitReader(original, int64(maxSize)+1))
		if err != nil {
			_ = original.Close()
			r.Body = http.NoBody
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ReplayRequestBody(r, original, recorded, maxSize)
		b, _ := io.ReadAll(r.Body)
		_, _ = w.Write(b)
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	tr := &http.Transport{DisableKeepAlives: false, MaxIdleConnsPerHost: 4}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	// 覆盖未截断与截断两条路径
	bodies := [][]byte{
		bytes.Repeat([]byte("a"), 40),
		bytes.Repeat([]byte("b"), 200),
	}
	for _, body := range bodies {
		reused := make([]bool, 0, 4)
		for i := 0; i < 4; i++ {
			v := false
			req, err := http.NewRequest(http.MethodPost, "http://"+ln.Addr().String()+"/echo", bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req = req.WithContext(httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
				GotConn: func(info httptrace.GotConnInfo) { v = info.Reused },
			}))
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if !bytes.Equal(got, body) {
				t.Fatalf("echo mismatch len got=%d want=%d", len(got), len(body))
			}
			reused = append(reused, v)
		}
		for i := 1; i < len(reused); i++ {
			if !reused[i] {
				t.Fatalf("bodyLen=%d connection not reused at req#%d flags=%v", len(body), i, reused)
			}
		}
	}
}
