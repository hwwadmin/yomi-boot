package db

import (
	"context"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/core/shared/enums"
)

func TestMustOpen_WarmIdleFails(t *testing.T) {
	cfg := Config{
		Type:                   enums.MySQL,
		Host:                   "127.0.0.1",
		Port:                   1,
		User:                   "u",
		Database:               "db",
		Params:                 "timeout=50ms",
		MaxOpenConns:           1,
		MaxIdleConns:           1,
		MinIdleConns:           1,
		ConnMaxLifetimeMinutes: 1,
		ConnMaxIdleTimeMinutes: 1,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	defer func() {
		if recover() == nil {
			t.Fatal("预热失败时 MustOpen 应 panic")
		}
	}()
	MustOpen(ctx, cfg)
}
