package distlock

import (
	"github.com/hwwadmin/yomi-boot/component/common/redis/redistest"
	"testing"
)

func TestMustNewFactory_NilClientPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil client 应 panic")
		}
	}()
	MustNewFactory(nil)
}

func TestMustNewFactory_Success(t *testing.T) {
	client, cleanup := redistest.NewClient(t)
	defer cleanup()

	factory := MustNewFactory(client)
	if factory == nil || factory.GetLock("k") == nil {
		t.Fatal("合法客户端应返回工厂")
	}
}
