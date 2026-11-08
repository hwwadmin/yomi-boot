package logger

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, errors.New("read") }
func (errReadCloser) Close() error             { return nil }

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

func TestAccessCapture_MiddlewareAndBody(t *testing.T) {
	if captureRequestBody(nil, AccessConfig{RecordBody: true}, nil).OmittedReason != "" {
		t.Fatal("nil 请求应返回空采集结果")
	}
	imageReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("png"))
	imageReq.Header.Set("Content-Type", "image/png")
	if captureRequestBody(imageReq, AccessConfig{RecordBody: true}, nil).OmittedReason != omitImageContent {
		t.Fatal("图片正文应省略")
	}
	pdfReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("pdf"))
	pdfReq.Header.Set("Content-Type", "application/pdf")
	if captureRequestBody(pdfReq, AccessConfig{RecordBody: true}, nil).OmittedReason != omitBinaryContent {
		t.Fatal("二进制正文应省略")
	}
	emptyBody := httptest.NewRequest(http.MethodPost, "/", nil)
	emptyBody.Header.Set("Content-Type", "application/json")
	emptyBody.Body = nil
	if captureRequestBody(emptyBody, AccessConfig{RecordBody: true}, nil).OmittedReason != "" {
		t.Fatal("空 body 应直接返回")
	}
	failReq := httptest.NewRequest(http.MethodPost, "/", errReadCloser{})
	failReq.Header.Set("Content-Type", "text/plain")
	if captureRequestBody(failReq, AccessConfig{RecordBody: true, MaxBodyBytes: 0}, nil).OmittedReason != omitCaptureFailed {
		t.Fatal("读取失败应标记采集失败")
	}
	zeroLen := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("ab"))
	zeroLen.ContentLength = 0
	zeroLen.Header.Set("Content-Type", "text/plain")
	got := captureRequestBody(zeroLen, AccessConfig{RecordBody: true, MaxBodyBytes: 8}, nil)
	if got.Value != "ab" || got.Bytes != 2 {
		t.Fatalf("零 Content-Length 应回填字节数: %+v", got)
	}
	headers := http.Header{"X-Empty": {}, "X-Long": {string(bytes.Repeat([]byte("a"), 20))}}
	collected := collectHeaders(headers, AccessConfig{MaxFieldBytes: 4}, nil)
	if _, ok := collected["x-empty"]; ok || collected["x-long"] == "" {
		t.Fatalf("header 采集结果 = %v", collected)
	}

	recorder := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(recorder)
	engine.GET("/body", func(c *gin.Context) {
		writer := newAccessBodyWriter(c.Writer, 4, true)
		if _, err := writer.WriteString("hello"); err != nil {
			t.Fatal(err)
		}
		writer.capture([]byte("more"))
		image := writer.captureResult(AccessConfig{RecordBody: true}, nil, "image/png")
		if image.OmittedReason != omitImageContent {
			t.Fatal("响应图片应省略")
		}
		disabled := writer.captureResult(AccessConfig{}, nil, "text/plain")
		if disabled.OmittedReason != omitAccessBodyDisabled {
			t.Fatal("关闭正文时应给出原因")
		}
	})
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/body", nil))

	if requestContext(nil) == nil || normalizePath("  ") != "/" || normalizePath("health") != "/health" {
		t.Fatal("路径和空上下文归一化不正确")
	}
	if !shouldKeepAccess(500, 0) || !shouldSample(0) {
		t.Fatal("错误状态和零采样率应保留日志")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	disabledAccess := false
	router.Use(Middleware(AccessConfig{Enabled: &disabledAccess}))
	router.GET("/off", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/off", nil))

	sampled := gin.New()
	sampled.Use(Middleware(AccessConfig{
		RecordBody:   false,
		SkipPaths:    []string{"/skip"},
		MaxBodyBytes: 32,
	}))
	sampled.GET("/skip", func(c *gin.Context) { c.Status(http.StatusOK) })
	sampled.GET("/live", func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/plain")
		c.String(http.StatusOK, "ok")
	})
	skipReq := httptest.NewRequest(http.MethodGet, "/skip", nil)
	sampled.ServeHTTP(httptest.NewRecorder(), skipReq)
	liveReq := httptest.NewRequest(http.MethodGet, "/live?q=1", nil)
	liveReq.Header.Set("User-Agent", "test-agent")
	sampled.ServeHTTP(httptest.NewRecorder(), liveReq)
}
