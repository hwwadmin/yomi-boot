// Package token 提供令牌撤销辅助函数
//
// @author zwei
package token

import (
	"context"
)

// RevokeByRelIDs 为给定切片中所有唯一的关联主体 ID 撤销令牌
func RevokeByRelIDs(ctx context.Context, tokenService TokenService, relIDs ...[]int64) error {
	if tokenService == nil {
		return nil
	}
	unique := make(map[int64]struct{})
	for _, items := range relIDs {
		for _, relID := range items {
			if relID <= 0 {
				continue
			}
			if _, exists := unique[relID]; exists {
				continue
			}
			unique[relID] = struct{}{}
			if err := tokenService.RevokeByUserID(ctx, relID); err != nil {
				return err
			}
		}
	}
	return nil
}
