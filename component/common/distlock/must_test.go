package distlock

import "testing"

func TestMustNewFactory_NilClientPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil client 应 panic")
		}
	}()
	MustNewFactory(nil)
}
