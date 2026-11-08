package outbox

import (
	"context"
	"testing"

	"github.com/hwwadmin/yomi-boot/component/common/event/cfg"
	"github.com/hwwadmin/yomi-boot/component/common/event/repo"
)

func TestFetchTimeoutRejectsNonPositiveLimit(t *testing.T) {
	task := NewFetchTimeout(cfg.Runtime{}, &repo.OutboxRepo{})
	if err := task.doTask(context.Background()); err == nil {
		t.Fatal("非正抓取上限应失败")
	}
}
