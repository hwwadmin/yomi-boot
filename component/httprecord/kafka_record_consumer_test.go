package httprecord

import (
	"testing"

	yomikafka "gitee.com/hq_project/yomi-boot/component/kafka"
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
