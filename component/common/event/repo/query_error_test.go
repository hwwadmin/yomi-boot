package repo

import (
	"context"
	"testing"
	"time"

	"github.com/hwwadmin/yomi-boot/component/common/event/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func deadDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "root@tcp(127.0.0.1:1)/dead?timeout=1s",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("构造失效数据库失败: %v", err)
	}
	return db
}

type namedDialector struct {
	gorm.Dialector
	name string
}

func (d namedDialector) Name() string { return d.name }

func TestRepo_QueryErrors(t *testing.T) {
	ctx := context.Background()
	db := deadDB(t)
	now := time.Now()
	inbox := NewInboxRepo(db)
	inboxDlq := NewInboxDlqRepo(db)
	outbox := NewOutboxRepo(db)
	outboxDlq := NewOutboxDlqRepo(db)

	if err := inbox.Insert(ctx, &model.SysEventInbox{ID: 1}); err == nil {
		t.Fatal("插入收件箱应失败")
	}
	if err := inbox.DeleteBatch(ctx, []int64{1}); err == nil {
		t.Fatal("删除收件箱应失败")
	}
	if _, err := inbox.Exists(ctx, "svc", "event-1"); err == nil {
		t.Fatal("查询收件箱应失败")
	}
	if _, err := inbox.ListConsumedBefore(ctx, "svc", now, 1); err == nil {
		t.Fatal("列举收件箱应失败")
	}

	if err := inboxDlq.Insert(ctx, &model.SysEventInboxDlq{ID: 1}); err == nil {
		t.Fatal("插入收件箱死信应失败")
	}
	if _, err := inboxDlq.Fetch(ctx, "svc", 1, now); err == nil {
		t.Fatal("查询收件箱死信应失败")
	}
	if err := inboxDlq.UpdateRetry(ctx, 1, 1, now, now); err == nil {
		t.Fatal("更新收件箱死信应失败")
	}
	if err := inboxDlq.Delete(ctx, 1); err == nil {
		t.Fatal("删除收件箱死信应失败")
	}

	if err := outbox.Insert(ctx, &model.SysEventOutbox{ID: 1}); err == nil {
		t.Fatal("插入发件箱应失败")
	}
	if _, err := outbox.FetchPending(ctx, "svc", 1); err == nil {
		t.Fatal("查询待发件箱应失败")
	}
	if _, err := outbox.ResetFetchedTimeout(ctx, "svc", now, 1); err == nil {
		t.Fatal("重置超时发件箱应失败")
	}
	if err := outbox.Update(ctx, &model.SysEventOutbox{ID: 1}); err == nil {
		t.Fatal("更新发件箱应失败")
	}
	if err := outbox.UpdateBatch(ctx, []model.SysEventOutbox{{ID: 1}}); err == nil {
		t.Fatal("批量更新发件箱应失败")
	}
	if _, err := outbox.MarkFetchedBatch(ctx, []int64{1}, now); err == nil {
		t.Fatal("标记已抓取应失败")
	}
	if _, err := outbox.MarkSentFail(ctx, 1); err == nil {
		t.Fatal("标记发送失败应失败")
	}
	if err := outbox.RestorePendingBatch(ctx, []int64{1}); err == nil {
		t.Fatal("恢复待处理应失败")
	}
	ids := make([]int64, idChunkSize+1)
	if _, err := outbox.FindByIDs(ctx, ids); err == nil {
		t.Fatal("批量查询发件箱应失败")
	}
	if err := outbox.DeleteBatch(ctx, []int64{1}); err == nil {
		t.Fatal("批量删除发件箱应失败")
	}

	if err := outboxDlq.Insert(ctx, &model.SysEventOutboxDlq{ID: 1}); err == nil {
		t.Fatal("插入发件箱死信应失败")
	}
	if _, err := outboxDlq.Fetch(ctx, "svc", 1, 3, now); err == nil {
		t.Fatal("查询发件箱死信应失败")
	}
	if err := outboxDlq.Update(ctx, &model.SysEventOutboxDlq{ID: 1}); err == nil {
		t.Fatal("更新发件箱死信应失败")
	}
	if err := outboxDlq.Delete(ctx, 1); err == nil {
		t.Fatal("删除发件箱死信应失败")
	}
	if err := outboxDlq.DeleteWithOutboxSuccess(ctx, &model.SysEventOutbox{ID: 1}, &model.SysEventOutboxDlq{ID: 1}, now, outbox); err == nil {
		t.Fatal("死信回写成功应失败")
	}
	if _, err := outboxDlq.ResetRetryCountInTimeRange(ctx, "svc", now.Add(-time.Hour), now); err == nil {
		t.Fatal("重置死信重试计数应失败")
	}

	odd := deadDB(t)
	odd.Dialector = namedDialector{Dialector: odd.Dialector, name: "oracle"}
	if _, err := NewOutboxRepo(odd).ResetFetchedTimeout(ctx, "svc", now, 1); err == nil {
		t.Fatal("不支持的数据库类型应失败")
	}
}
