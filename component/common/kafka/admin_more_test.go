package kafka

import (
	"context"
	"errors"
	"testing"
)

func TestAdmin_RejectsBlankTopicAndMapsUnknownErrors(t *testing.T) {
	a := &admin{}
	if _, err := a.DescribeTopic(context.Background(), " "); err == nil {
		t.Fatal("空 topic 不应查询")
	}
	if err := a.DeleteTopic(context.Background(), " "); err == nil {
		t.Fatal("空 topic 不应删除")
	}
	if err := mapDescribeTopicError(errors.New("down"), "t"); err == nil {
		t.Fatal("未知查询错误应返回")
	}
	if err := mapCreateTopicError(errors.New("down"), "t"); err == nil {
		t.Fatal("未知创建错误应返回")
	}
	if err := mapDeleteTopicError(errors.New("down"), "t"); err == nil {
		t.Fatal("未知删除错误应返回")
	}
	if toBrokers(nil) != nil {
		t.Fatal("空 broker 列表应返回 nil")
	}
}
