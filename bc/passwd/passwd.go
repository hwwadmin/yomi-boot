// Package passwd 提供密码单向哈希与校验能力（bcrypt）
//
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

// ErrMismatch 表示明文与哈希不匹配
var ErrMismatch = errors.NewNoStack("passwd: password mismatch")

// Hash 使用 DefaultCost 对明文密码加盐哈希
func Hash(plain string) (string, error) {
	return HashWithCost(plain, DefaultCost)
}

// HashWithCost 使用指定成本对明文密码加盐哈希
func HashWithCost(plain string, cost int) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return "", errors.Wrap(err, "passwd: hash password")
	}
	return string(hashed), nil
}

// Verify 校验明文是否与哈希匹配；不匹配返回 ErrMismatch
func Verify(hash, plain string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
	if err == nil {
		return nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return errors.Sentinel(ErrMismatch)
	}
	return errors.Wrap(err, "passwd: verify password")
}
