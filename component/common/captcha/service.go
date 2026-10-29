// 验证码服务实现。
//
// @author zwei

package captcha

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"time"

	"gitee.com/hq_project/yomi-boot/component/core/errors"

	base64Captcha "github.com/mojocn/base64Captcha"
)

// maxAnswerLen 是答案明文的最大长度；生成与用户输入都用它做上限
const maxAnswerLen = 64

// Challenge 表示一次验证码挑战
type Challenge struct {
	ID        string // ID 是校验时原样回传的标识；区分大小写
	Image     string // Image 是 data:image/png;base64,...，可直接作为 img src
	ExpiresIn int    // ExpiresIn 是验证码剩余有效期（秒，按 TTL 向下取整），供前端做倒计时与提前刷新
}

// Service 表示验证码服务
type Service interface {
	// Generate 生成验证码；答案只写 Store，不返回给调用方
	Generate(ctx context.Context) (Challenge, error)
	// Verify 校验并消费验证码：成功返回 nil；不存在/已过期/已消费返回 ErrExpired；
	// 答案不符返回 ErrMismatch（此时同样已消费）；参数非法返回 ErrInvalidArgument；
	// Store 故障原样返回，不视为验证失败
	Verify(ctx context.Context, id, answer string) error
}

// service 是验证码服务实现
type service struct {
	driver base64Captcha.Driver
	store  Store
	ttl    time.Duration
}

var _ Service = (*service)(nil)

// New 创建验证码服务；store 为 nil 或配置非法时返回错误
func New(store Store, cfg Config) (Service, error) {
	if store == nil {
		return nil, invalidArgument("captcha: store 不能为 nil")
	}

	normalized, err := cfg.normalize()
	if err != nil {
		return nil, err
	}
	driver, err := newDriver(normalized)
	if err != nil {
		return nil, err
	}

	return &service{driver: driver, store: store, ttl: normalized.TTL}, nil
}

// Generate 生成验证码
func (s *service) Generate(ctx context.Context) (Challenge, error) {
	if s == nil || s.driver == nil || s.store == nil {
		return Challenge{}, errors.New("captcha: service 未初始化")
	}
	if err := ctx.Err(); err != nil {
		return Challenge{}, err
	}

	id, answer, image, err := s.render()
	if err != nil {
		return Challenge{}, err
	}
	if err := ctx.Err(); err != nil {
		return Challenge{}, err
	}
	// 只把答案摘要交给 Store，明文答案不落存储
	if err := s.store.Save(ctx, id, answerDigest(answer), s.ttl); err != nil {
		return Challenge{}, err
	}

	return Challenge{ID: id, Image: image, ExpiresIn: int(s.ttl / time.Second)}, nil
}

// render 调用上游驱动生成 id、答案与图片；上游 panic 统一转 ErrGenerateFailed
func (s *service) render() (id, answer, image string, err error) {
	defer func() {
		if r := recover(); r != nil {
			id, answer, image = "", "", ""
			err = errors.Wrap(ErrGenerateFailed, "captcha: 渲染失败: %v", r)
		}
	}()

	id, question, answer := s.driver.GenerateIdQuestionAnswer()
	// id 必须与 Verify 用同一套校验，确保发出的验证码一定能被校验；答案在此归一化，后续不再清洗
	answer = normalizeAnswer(answer)
	if !validID(id) || answer == "" || len(answer) > maxAnswerLen {
		return "", "", "", errors.Sentinel(ErrGenerateFailed)
	}

	item, err := s.driver.DrawCaptcha(question)
	if err != nil {
		return "", "", "", errors.Wrap(ErrGenerateFailed, "captcha: 绘制失败: %v", err)
	}
	if item == nil {
		return "", "", "", errors.Sentinel(ErrGenerateFailed)
	}

	image = item.EncodeB64string()
	if image == "" {
		return "", "", "", errors.Sentinel(ErrGenerateFailed)
	}
	return id, answer, image, nil
}

// Verify 校验并消费验证码
func (s *service) Verify(ctx context.Context, id, answer string) error {
	id = strings.TrimSpace(id)
	answer = normalizeAnswer(answer)
	if !validID(id) || answer == "" || len(answer) > maxAnswerLen {
		return errors.Sentinel(ErrInvalidArgument)
	}
	if s == nil || s.store == nil {
		return errors.New("captcha: service 未初始化")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	stored, found, err := s.store.Consume(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return errors.Sentinel(ErrExpired)
	}
	if !equalDigest(stored, answerDigest(answer)) {
		return errors.Sentinel(ErrMismatch)
	}
	return nil
}

// validID 校验 id 仅含 ASCII 字母数字且长度不超过上限
func validID(id string) bool {
	if id == "" || len(id) > maxIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			continue
		}
		return false
	}
	return true
}

// normalizeAnswer 归一化答案：去首尾空白并统一大写；生成与校验都只在入口调用一次
func normalizeAnswer(answer string) string {
	return strings.ToUpper(strings.TrimSpace(answer))
}

// answerDigest 计算已归一化答案的 SHA-256 摘要（小写十六进制）
func answerDigest(answer string) string {
	sum := sha256.Sum256([]byte(answer))
	return hex.EncodeToString(sum[:])
}

// equalDigest 以常量时间比较两个摘要
func equalDigest(stored, computed string) bool {
	if len(stored) != len(computed) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(computed)) == 1
}
