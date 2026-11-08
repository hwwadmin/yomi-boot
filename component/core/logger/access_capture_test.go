package logger

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

// buildMultipartBody 构造含文件分片与普通文本字段的 multipart/form-data 请求体
func buildMultipartBody(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "a.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("png-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("note", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

// TestCaptureRequestBody_MultipartOmitted 验证 multipart/form-data 正文按 file_upload 省略
func TestCaptureRequestBody_MultipartOmitted(t *testing.T) {
	buf, contentType := buildMultipartBody(t)
	req := httptest.NewRequest(http.MethodPost, "/upload", buf)
	req.Header.Set("Content-Type", contentType)
	got := captureRequestBody(req, AccessConfig{RecordBody: true, MaxBodyBytes: 64}, nil)
	if got.OmittedReason != omitFileUpload {
		t.Fatalf("multipart 正文应按 file_upload 省略: %+v", got)
	}
	if got.Value != "" {
		t.Fatalf("multipart 正文不应记录: %+v", got)
	}
	if got.ContentType != contentType {
		t.Fatalf("应保留 Content-Type: %+v", got)
	}
	if got.Bytes <= 0 {
		t.Fatalf("应保留原始字节数: %+v", got)
	}
}

// TestAccessMiddleware_MultipartBodyOmitted 端到端验证中间件对 multipart 请求
// 只落 file_upload 省略原因，不落正文；且正文未被消费，handler 仍能正常解析表单
func TestAccessMiddleware_MultipartBodyOmitted(t *testing.T) {
	dir := t.TempDir()
	closeFn, err := InitLogger("test-svc", "test", Config{
		Dir:      dir,
		Level:    "info",
		Rotation: testRotation(),
	}, nil)
	if err != nil {
		t.Fatalf("InitLogger: %v", err)
	}
	defer func() { _ = closeFn() }()

	buf, contentType := buildMultipartBody(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Middleware(AccessConfig{RecordBody: true, MaxBodyBytes: 1024}))
	router.POST("/upload", func(c *gin.Context) {
		if note := c.Request.FormValue("note"); note != "hello" {
			t.Errorf("普通字段应可正常解析: %q", note)
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/upload", buf)
	req.Header.Set("Content-Type", contentType)
	router.ServeHTTP(httptest.NewRecorder(), req)

	_ = Sync()
	accessPath := filepath.Join(dir, "access", "access.log")
	assertFileContains(t, accessPath, `"body_omitted_reason":"file_upload"`)
	assertFileNotContains(t, accessPath, "png-bytes")
	assertFileNotContains(t, accessPath, "hello")
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
