// 验证码服务测试。
//
// @author zwei

package captcha

import (
	"bytes"
	"context"
	"encoding/base64"
	stderrors "errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	base64Captcha "github.com/mojocn/base64Captcha"
)

// spyStore 是测试用的内存 Store，可注入读写错误并检查摘要
type spyStore struct {
	mu           sync.Mutex
	digests      map[string]string
	saveErr      error
	consumeErr   error
	saveCalls    int
	consumeCalls int
}

// newSpyStore 创建测试 Store
func newSpyStore() *spyStore {
	return &spyStore{digests: make(map[string]string)}
}

// Save 实现 Store.Save
func (s *spyStore) Save(_ context.Context, id, digest string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.digests[id] = digest
	return nil
}

// Consume 实现 Store.Consume
func (s *spyStore) Consume(_ context.Context, id string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consumeCalls++
	if s.consumeErr != nil {
		return "", false, s.consumeErr
	}
	digest, ok := s.digests[id]
	delete(s.digests, id)
	return digest, ok, nil
}

// digest 读取指定 id 的摘要
func (s *spyStore) digest(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	digest, ok := s.digests[id]
	return digest, ok
}

// setDigest 直接写入摘要，用于构造已知答案的场景
func (s *spyStore) setDigest(id, digest string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.digests[id] = digest
}

// setSaveErr 注入写入错误
func (s *spyStore) setSaveErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveErr = err
}

// setConsumeErr 注入读取错误
func (s *spyStore) setConsumeErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consumeErr = err
}

// counts 返回 Save / Consume 调用次数
func (s *spyStore) counts() (save, consume int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCalls, s.consumeCalls
}

// newTestService 创建带测试 Store 的服务
func newTestService(t *testing.T, cfg Config) (Service, *spyStore) {
	t.Helper()
	store := newSpyStore()
	if cfg.TTL == 0 {
		cfg.TTL = time.Minute
	}
	svc, err := New(store, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc, store
}

// assertChallengeID 校验上游 id 的格式
func assertChallengeID(t *testing.T, id string) {
	t.Helper()
	if len(id) != 20 {
		t.Fatalf("id 长度 = %d, 期望 20", len(id))
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			continue
		}
		t.Fatalf("id 含非法字符: %q", id)
	}
}

// assertPNGDataURI 校验图片为合法的 PNG data URI
func assertPNGDataURI(t *testing.T, image string) {
	t.Helper()
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(image, prefix) {
		t.Fatalf("image 前缀 = %q, 期望 %q", image, prefix)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(image, prefix))
	if err != nil {
		t.Fatalf("base64 解码失败: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("image 不是 PNG: %x", raw)
	}
}

// assertAnswerDigest 校验 Store 中保存的是 64 位小写十六进制摘要
func assertAnswerDigest(t *testing.T, digest string) {
	t.Helper()
	if len(digest) != answerDigestLen {
		t.Fatalf("摘要长度 = %d, 期望 %d", len(digest), answerDigestLen)
	}
	for i := 0; i < len(digest); i++ {
		c := digest[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		t.Fatalf("摘要含非十六进制字符: %q", digest)
	}
}

// TestServiceGenerateAndVerify 验证三种驱动都能生成合法挑战，且答案只以摘要落 Store
func TestServiceGenerateAndVerify(t *testing.T) {
	for _, driver := range []Driver{DriverDigit, DriverString, DriverMath} {
		t.Run(string(driver), func(t *testing.T) {
			svc, store := newTestService(t, Config{Driver: driver})
			ctx := context.Background()

			challenge, err := svc.Generate(ctx)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			assertChallengeID(t, challenge.ID)
			assertPNGDataURI(t, challenge.Image)

			stored, ok := store.digest(challenge.ID)
			if !ok || stored == "" {
				t.Fatalf("Store 中未找到摘要: ok=%v digest=%q", ok, stored)
			}
			assertAnswerDigest(t, stored)

			// 错误答案同样消费
			if err := svc.Verify(ctx, challenge.ID, "wrong-answer"); !errors.Is(err, ErrMismatch) {
				t.Fatalf("错误答案 err = %v, 期望 ErrMismatch", err)
			}
			if err := svc.Verify(ctx, challenge.ID, "wrong-answer"); !errors.Is(err, ErrExpired) {
				t.Fatalf("消费后 err = %v, 期望 ErrExpired", err)
			}
		})
	}
}

// fixedDriver 返回固定 id 与答案，用于断言「答案 → 摘要」的写入口径
type fixedDriver struct {
	id     string
	answer string
}

// GenerateIdQuestionAnswer 实现 base64Captcha.Driver
func (d fixedDriver) GenerateIdQuestionAnswer() (string, string, string) {
	return d.id, d.answer, d.answer
}

// DrawCaptcha 返回最小的 Item，图片内容与答案无关
func (fixedDriver) DrawCaptcha(string) (base64Captcha.Item, error) {
	return fixedItem{}, nil
}

// fixedItem 是测试用的最小验证码图片
type fixedItem struct{}

// WriteTo 实现 base64Captcha.Item
func (fixedItem) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write([]byte("png"))
	return int64(n), err
}

// EncodeB64string 实现 base64Captcha.Item
func (fixedItem) EncodeB64string() string {
	return "data:image/png;base64,AAAA"
}

// TestServiceAnswerDigest 验证答案只以摘要写入 Store，且校验时按归一化后的摘要比较
func TestServiceAnswerDigest(t *testing.T) {
	const id = "fixedid1234567890ab"
	store := newSpyStore()
	svc := &service{driver: fixedDriver{id: id, answer: "AbCd"}, store: store, ttl: time.Minute}
	ctx := context.Background()

	challenge, err := svc.Generate(ctx)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if challenge.ID != id {
		t.Fatalf("id = %q, 期望 %q", challenge.ID, id)
	}

	stored, ok := store.digest(id)
	if !ok {
		t.Fatal("Store 中未找到摘要")
	}
	if stored != answerDigest("AbCd") {
		t.Fatalf("Store 存的值 = %q, 期望摘要 %q", stored, answerDigest("AbCd"))
	}
	if stored == "AbCd" {
		t.Fatal("明文答案不应写入 Store")
	}

	// 大小写与首尾空白在摘要前归一化
	if err := svc.Verify(ctx, id, "  abcd  "); err != nil {
		t.Fatalf("归一化后 Verify: %v", err)
	}
	if err := svc.Verify(ctx, id, "abcd"); !errors.Is(err, ErrExpired) {
		t.Fatalf("成功后二次 Verify err = %v, 期望 ErrExpired", err)
	}
}

// TestServiceVerifyMismatchConsumes 验证错误答案同样消费验证码
func TestServiceVerifyMismatchConsumes(t *testing.T) {
	const id = "fixedid1234567890ab"
	store := newSpyStore()
	svc := &service{driver: fixedDriver{id: id, answer: "1234"}, store: store, ttl: time.Minute}
	ctx := context.Background()

	if _, err := svc.Generate(ctx); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := svc.Verify(ctx, id, "4321"); !errors.Is(err, ErrMismatch) {
		t.Fatalf("错误答案 err = %v, 期望 ErrMismatch", err)
	}
	if err := svc.Verify(ctx, id, "1234"); !errors.Is(err, ErrExpired) {
		t.Fatalf("错误答案消费后 err = %v, 期望 ErrExpired", err)
	}
}

// TestServiceVerifyInvalidInput 验证非法入参不触达 Store
func TestServiceVerifyInvalidInput(t *testing.T) {
	svc, store := newTestService(t, Config{Driver: DriverDigit})
	ctx := context.Background()

	tests := []struct {
		name   string
		id     string
		answer string
	}{
		{name: "空 id", id: "", answer: "1234"},
		{name: "空白 id", id: "   ", answer: "1234"},
		{name: "id 含非法字符", id: "abc-123", answer: "1234"},
		{name: "id 过长", id: strings.Repeat("a", maxIDLen+1), answer: "1234"},
		{name: "空 answer", id: "abc123", answer: ""},
		{name: "空白 answer", id: "abc123", answer: "   "},
		{name: "answer 过长", id: "abc123", answer: strings.Repeat("a", maxAnswerLen+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.Verify(ctx, tt.id, tt.answer); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("Verify err = %v, 期望 ErrInvalidArgument", err)
			}
		})
	}

	if save, consume := store.counts(); save != 0 || consume != 0 {
		t.Fatalf("非法入参不应触达 Store: save=%d consume=%d", save, consume)
	}
}

// TestServiceGenerateBoundaryConfig 验证允许范围内的极端尺寸/干扰配置不会触发上游 panic
func TestServiceGenerateBoundaryConfig(t *testing.T) {
	configs := []Config{
		{Driver: DriverDigit, Width: minWidth, Height: minHeight, Length: maxLength, TTL: time.Minute},
		{
			Driver: DriverString, Width: minWidth, Height: minHeight, Length: maxLength,
			NoiseCount: maxNoiseCount, Lines: LineHollow | LineSlime | LineSine, TTL: time.Minute,
		},
		{
			Driver: DriverMath, Width: minWidth, Height: minHeight,
			NoiseCount: maxNoiseCount, Lines: LineHollow | LineSlime | LineSine, TTL: time.Minute,
		},
		{Driver: DriverDigit, Width: maxWidth, Height: maxHeight, Length: maxLength, TTL: time.Minute},
	}
	for _, cfg := range configs {
		t.Run(string(cfg.Driver), func(t *testing.T) {
			svc, store := newTestService(t, cfg)
			ctx := context.Background()

			challenge, err := svc.Generate(ctx)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			assertPNGDataURI(t, challenge.Image)
			stored, ok := store.digest(challenge.ID)
			if !ok {
				t.Fatal("Store 中未找到摘要")
			}
			assertAnswerDigest(t, stored)

			if err := svc.Verify(ctx, challenge.ID, "wrong"); !errors.Is(err, ErrMismatch) {
				t.Fatalf("错误答案 err = %v, 期望 ErrMismatch", err)
			}
		})
	}
}

// TestServiceVerifyExpired 验证未知 id 返回 ErrExpired
func TestServiceVerifyExpired(t *testing.T) {
	svc, _ := newTestService(t, Config{Driver: DriverDigit})
	if err := svc.Verify(context.Background(), "unknown123", "1234"); !errors.Is(err, ErrExpired) {
		t.Fatalf("Verify err = %v, 期望 ErrExpired", err)
	}
}

// TestServiceStoreErrors 验证 Store 故障不会被映射为验证失败
func TestServiceStoreErrors(t *testing.T) {
	probe := stderrors.New("store down")

	t.Run("consume", func(t *testing.T) {
		svc, store := newTestService(t, Config{Driver: DriverDigit})
		store.setConsumeErr(probe)

		err := svc.Verify(context.Background(), "abc123", "1234")
		if !errors.Is(err, probe) {
			t.Fatalf("Verify err = %v, 期望包含底层错误", err)
		}
		if errors.Is(err, ErrExpired) || errors.Is(err, ErrMismatch) {
			t.Fatalf("Store 故障不应映射为验证失败: %v", err)
		}
	})

	t.Run("save", func(t *testing.T) {
		svc, store := newTestService(t, Config{Driver: DriverDigit})
		store.setSaveErr(probe)

		if _, err := svc.Generate(context.Background()); !errors.Is(err, probe) {
			t.Fatalf("Generate err = %v, 期望包含底层错误", err)
		}
	})
}

// TestServiceContextCanceled 验证取消后不触达 Store
func TestServiceContextCanceled(t *testing.T) {
	svc, store := newTestService(t, Config{Driver: DriverDigit})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := svc.Generate(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate err = %v, 期望 context.Canceled", err)
	}
	if err := svc.Verify(ctx, "abc123", "1234"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Verify err = %v, 期望 context.Canceled", err)
	}
	if save, consume := store.counts(); save != 0 || consume != 0 {
		t.Fatalf("取消后不应触达 Store: save=%d consume=%d", save, consume)
	}
}

// panicDriver 模拟上游渲染 panic
type panicDriver struct{}

func (panicDriver) GenerateIdQuestionAnswer() (string, string, string) {
	return "panicid", "question", "answer"
}

func (panicDriver) DrawCaptcha(string) (base64Captcha.Item, error) {
	panic("boom")
}

// nilItemDriver 模拟上游返回空 Item
type nilItemDriver struct{}

func (nilItemDriver) GenerateIdQuestionAnswer() (string, string, string) {
	return "nilid", "question", "answer"
}

func (nilItemDriver) DrawCaptcha(string) (base64Captcha.Item, error) {
	return nil, nil
}

// errorDriver 模拟上游绘制失败
type errorDriver struct{}

func (errorDriver) GenerateIdQuestionAnswer() (string, string, string) {
	return "errorid", "question", "answer"
}

func (errorDriver) DrawCaptcha(string) (base64Captcha.Item, error) {
	return nil, stderrors.New("draw failed")
}

// TestServiceGenerateFailed 验证上游 panic/空 Item/错误统一转 ErrGenerateFailed
func TestServiceGenerateFailed(t *testing.T) {
	tests := []struct {
		name   string
		driver base64Captcha.Driver
	}{
		{name: "panic", driver: panicDriver{}},
		{name: "nil item", driver: nilItemDriver{}},
		{name: "draw error", driver: errorDriver{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &service{driver: tt.driver, store: newSpyStore(), ttl: time.Minute}
			if _, err := svc.Generate(context.Background()); !errors.Is(err, ErrGenerateFailed) {
				t.Fatalf("Generate err = %v, 期望 ErrGenerateFailed", err)
			}
		})
	}
}

// TestServiceNilDependencies 验证依赖缺失时返回错误而不是 panic
func TestServiceNilDependencies(t *testing.T) {
	if svc, err := New(nil, Config{}); err == nil || svc != nil {
		t.Fatalf("New(nil) = (%v, %v), 期望错误", svc, err)
	}

	var nilService *service
	if _, err := nilService.Generate(context.Background()); err == nil {
		t.Fatal("nil service Generate 应返回错误")
	}
	if err := nilService.Verify(context.Background(), "abc123", "1234"); err == nil {
		t.Fatal("nil service Verify 应返回错误")
	}

	svc := &service{store: newSpyStore(), ttl: time.Minute}
	if _, err := svc.Generate(context.Background()); err == nil {
		t.Fatal("nil driver Generate 应返回错误")
	}
}

// TestNewConfigError 验证非法配置不会构造出服务
func TestNewConfigError(t *testing.T) {
	store := newSpyStore()
	tests := []Config{
		{Driver: "unknown"},
		{Width: minWidth - 1},
		{Height: minHeight - 1},
		{TTL: minTTL - time.Second},
		{Driver: DriverString, Source: "ab"},
	}
	for _, cfg := range tests {
		if svc, err := New(store, cfg); err == nil || svc != nil {
			t.Fatalf("New(%+v) = (%v, %v), 期望错误", cfg, svc, err)
		}
	}
}

// TestMustNew 验证 Fail Fast 构造
func TestMustNew(t *testing.T) {
	if svc := MustNew(newSpyStore(), Config{}); svc == nil {
		t.Fatal("MustNew 合法配置不应返回 nil")
	}
}

// TestServiceConcurrent 验证并发生成与校验
func TestServiceConcurrent(t *testing.T) {
	svc, store := newTestService(t, Config{Driver: DriverDigit})
	ctx := context.Background()

	const total = 20
	ids := make([]string, total)
	seen := make(map[string]struct{}, total)

	for i := 0; i < total; i++ {
		challenge, err := svc.Generate(ctx)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if _, ok := seen[challenge.ID]; ok {
			t.Fatalf("重复 id: %s", challenge.ID)
		}
		seen[challenge.ID] = struct{}{}
		ids[i] = challenge.ID

		stored, ok := store.digest(challenge.ID)
		if !ok {
			t.Fatalf("Store 中未找到摘要: %s", challenge.ID)
		}
		assertAnswerDigest(t, stored)
	}

	// 用已知答案覆盖摘要后并发校验
	answers := make([]string, total)
	for i := range answers {
		answers[i] = fmt.Sprintf("ans%02d", i)
		store.setDigest(ids[i], answerDigest(answers[i]))
	}

	var wg sync.WaitGroup
	errCh := make(chan error, total)
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- svc.Verify(ctx, ids[i], answers[i])
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("并发 Verify: %v", err)
		}
	}
}

// TestLineOptions 验证干扰线位掩码映射
func TestLineOptions(t *testing.T) {
	if got := lineOptions(0); got != 0 {
		t.Fatalf("lineOptions(0) = %d", got)
	}
	if got := lineOptions(LineHollow); got != base64Captcha.OptionShowHollowLine {
		t.Fatalf("LineHollow = %d", got)
	}
	got := lineOptions(LineHollow | LineSlime | LineSine)
	want := base64Captcha.OptionShowHollowLine | base64Captcha.OptionShowSlimeLine | base64Captcha.OptionShowSineLine
	if got != want {
		t.Fatalf("组合 = %d, 期望 %d", got, want)
	}
}

// TestAnswerDigest 验证答案摘要的归一化与长度
func TestAnswerDigest(t *testing.T) {
	if answerDigest("ABCD") != answerDigest(" abcd ") {
		t.Fatal("大写与首尾空白应归一化为同一摘要")
	}
	if answerDigest("abcd") == answerDigest("abce") {
		t.Fatal("不同答案的摘要不应相同")
	}
	got := answerDigest("1234")
	if len(got) != answerDigestLen {
		t.Fatalf("摘要长度 = %d, 期望 %d", len(got), answerDigestLen)
	}
	if got == "1234" {
		t.Fatal("摘要不应等于明文")
	}
}

// TestEqualDigest 验证摘要比较
func TestEqualDigest(t *testing.T) {
	digest := answerDigest("abcd")
	if !equalDigest(digest, answerDigest("ABCD")) {
		t.Fatal("同一答案的摘要应判等")
	}
	if equalDigest(digest, answerDigest("abce")) {
		t.Fatal("不同答案的摘要不应判等")
	}
	if equalDigest(digest, digest[:len(digest)-1]) {
		t.Fatal("长度不同的摘要不应判等")
	}
}
