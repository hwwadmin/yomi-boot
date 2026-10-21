package boot

import "testing"

func TestRunWithOptions_RequiresName(t *testing.T) {
	mustPanic(t, "boot: Name 不能为空", func() {
		RunWithOptions(Options{}, func(*Infra) []Registrar { return nil })
	})
}

func TestRunWithOptions_RequiresWireFunc(t *testing.T) {
	mustPanic(t, "boot: WireFunc 不能为空", func() {
		RunWithOptions(Options{Name: "test-service"}, nil)
	})
}

func mustPanic(t *testing.T, want string, fn func()) {
	t.Helper()

	defer func() {
		got := recover()
		if got == nil {
			t.Fatalf("期望 panic %q", want)
		}
		if message, ok := got.(string); !ok || message != want {
			t.Fatalf("panic = %v, 期望 %q", got, want)
		}
	}()

	fn()
}
