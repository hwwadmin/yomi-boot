// Package passwd 提供密码单向哈希与校验能力（bcrypt）
//
// bcrypt 只使用明文的前 72 字节；Hash 与 Verify 对超长密码统一返回 ErrPasswordTooLong。
// 成本必须位于 bcrypt.MinCost..bcrypt.MaxCost，否则返回 error。
// 属于 boot/bc 密码学基础设施的一种能力；不输出业务 BizCode，强度规则由领域层自行校验。
//
// @author zwei
package passwd

import (
	"gitee.com/hq_project/yomi-boot/errors"

	"golang.org/x/crypto/bcrypt"
)

// DefaultCost 默认 bcrypt 成本，与 golang.org/x/crypto/bcrypt.DefaultCost 一致
const DefaultCost = bcrypt.DefaultCost

// MaxPasswordBytes bcrypt 支持的最大密码字节数
const MaxPasswordBytes = 72

// ErrMismatch 表示明文与哈希不匹配
var ErrMismatch = errors.NewNoStack("passwd: password mismatch")

// ErrPasswordTooLong 表示密码超过 bcrypt 的 72 字节上限
var ErrPasswordTooLong = errors.NewNoStack("passwd: password exceeds 72 bytes")

// Hash 使用 DefaultCost 对明文密码加盐哈希
func Hash(plain string) (string, error) {
	return HashWithCost(plain, DefaultCost)
}

// HashWithCost 使用指定成本对明文密码加盐哈希；cost 须在 bcrypt.MinCost..bcrypt.MaxCost
func HashWithCost(plain string, cost int) (string, error) {
	if len(plain) > MaxPasswordBytes {
		return "", errors.Sentinel(ErrPasswordTooLong)
	}
	if err := validCost(cost); err != nil {
		return "", err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return "", errors.Wrap(err, "passwd: hash password")
	}
	return string(hashed), nil
}

// Verify 校验明文是否与哈希匹配；不匹配返回 ErrMismatch，超长返回 ErrPasswordTooLong
func Verify(hash, plain string) error {
	if len(plain) > MaxPasswordBytes {
		return errors.Sentinel(ErrPasswordTooLong)
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
	if err == nil {
		return nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return errors.Sentinel(ErrMismatch)
	}
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return errors.Sentinel(ErrPasswordTooLong)
	}
	return errors.Wrap(err, "passwd: verify password")
}

// Cost 返回 bcrypt 哈希使用的成本因子
func Cost(hash string) (int, error) {
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		return 0, errors.Wrap(err, "passwd: read cost")
	}
	return cost, nil
}

// NeedsRehash 判断哈希成本是否低于目标成本；哈希非法或目标成本越界时返回 error
func NeedsRehash(hash string, cost int) (bool, error) {
	if err := validCost(cost); err != nil {
		return false, err
	}
	current, err := Cost(hash)
	if err != nil {
		return false, err
	}
	return current < cost, nil
}

// validCost 校验 bcrypt 成本范围
func validCost(cost int) error {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return errors.Newf("passwd: cost 必须在 %d..%d", bcrypt.MinCost, bcrypt.MaxCost)
	}
	return nil
}
