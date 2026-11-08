package logger

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCaptureRequestBody_EmptyAndImage(t *testing.T) {
	empty := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	empty.Header.Set("Content-Type", "application/json")
	got := captureRequestBody(empty, AccessConfig{RecordBody: true, MaxBodyBytes: 64}, nil)
	if got.OmittedReason != "" || got.Value != "" {
		t.Fatalf("空正文应直接返回: %+v", got)
	}

	image := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("png"))
	image.Header.Set("Content-Type", "image/png")
	got = captureRequestBody(image, AccessConfig{RecordBody: true, MaxBodyBytes: 64}, nil)
	if got.OmittedReason != omitImageContent {
		t.Fatalf("图片正文应省略: %+v", got)
	}
}

func TestAccessBodyWriter_SecondWriteTruncates(t *testing.T) {
	w := &accessBodyWriter{enabled: true, maxSize: 1}
	w.capture([]byte("ab"))
	w.capture([]byte("c"))
	if !w.truncated || w.body.Len() != 1 {
		t.Fatalf("超出上限后应截断: truncated=%v len=%d", w.truncated, w.body.Len())
	}
}
