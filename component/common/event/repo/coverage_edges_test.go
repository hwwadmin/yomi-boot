package repo

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRepoGuards(t *testing.T) {
	if err := forEachIDChunk([]int64{1, 2}, func([]int64) error { return errors.New("chunk") }); err == nil {
		t.Fatal("分片回调失败应返回")
	}
	ids := make([]int64, 0, idChunkSize+1)
	for i := 0; i < idChunkSize+1; i++ {
		ids = append(ids, int64(i%3))
	}
	if len(uniqueIDs(ids)) != 3 {
		t.Fatal("重复主键应去重")
	}
	if err := hardDeleteByIDs(context.Background(), nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := (&OutboxRepo{}).UpdateBatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if rows, err := (&OutboxRepo{}).MarkFetchedBatch(ctx, nil, time.Now()); err != nil || rows != nil {
		t.Fatalf("空 ID 应直接返回: %v %v", rows, err)
	}
	if err := (&OutboxRepo{}).RestorePendingBatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
}
