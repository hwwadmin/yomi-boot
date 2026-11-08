package httputil

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestRemoteAddrHost(t *testing.T) {
	if RemoteAddrHost(nil) != "" {
		t.Fatal("nil 请求应返回空主机")
	}
	req := &http.Request{RemoteAddr: "203.0.113.8:443"}
	if got := RemoteAddrHost(req); got != "203.0.113.8" {
		t.Fatalf("RemoteAddrHost = %q", got)
	}
	req.RemoteAddr = " 203.0.113.9 "
	if got := RemoteAddrHost(req); got != "203.0.113.9" {
		t.Fatalf("无端口 RemoteAddrHost = %q", got)
	}
}

func TestReplayRequestBodyEdges(t *testing.T) {
	ReplayRequestBody(nil, io.NopCloser(bytes.NewReader(nil)), []byte("x"), 1)

	req := &http.Request{}
	ReplayRequestBody(req, nil, nil, 1)
	if req.Body != http.NoBody {
		t.Fatal("没有原始 Body 时应替换为 NoBody")
	}

	original := io.NopCloser(bytes.NewReader([]byte("rest")))
	req = &http.Request{}
	ReplayRequestBody(req, original, nil, -1)
	if req.Body != http.NoBody {
		t.Fatal("空记录应替换为 NoBody")
	}

	original = io.NopCloser(bytes.NewReader([]byte("rest")))
	req = &http.Request{}
	ReplayRequestBody(req, original, []byte("ab"), 1)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "abrest" {
		t.Fatalf("超限拼接 = %q", body)
	}
	if err := req.Body.Close(); err != nil {
		t.Fatal(err)
	}
}
