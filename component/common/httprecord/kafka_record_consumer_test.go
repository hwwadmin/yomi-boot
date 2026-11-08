package httprecord

import (
	yomikafka "github.com/hwwadmin/yomi-boot/component/common/kafka"
	"testing"
)

// TestMapRecordHandlerResultToKafka 验证 RecordHandleResult 到 Kafka ConsumeResult 的映射
func TestMapRecordHandlerResultToKafka(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   RecordHandleResult
		want yomikafka.ConsumeResult
	}{
		{name: "ok", in: HandleOK(), want: yomikafka.AckOK()},
		{name: "retry", in: HandleRetry(), want: yomikafka.Retry()},
		{name: "drop", in: HandleDrop(), want: yomikafka.AckDrop()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mapRecordHandlerResultToKafka(tc.in)
			if got != tc.want {
				t.Fatalf("mapRecordHandlerResultToKafka() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestMapRecordHandlerResultToKafka_UnknownStatus(t *testing.T) {
	unknown := RecordHandleResult{Status: RecordHandleStatus(99)}
	if got := mapRecordHandlerResultToKafka(unknown); got != yomikafka.AckDrop() {
		t.Fatalf("未知状态应丢弃 Kafka 消息: %v", got)
	}
}
