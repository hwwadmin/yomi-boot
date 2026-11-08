package captcha

import (
	"github.com/hwwadmin/yomi-boot/component/core/errors"
	"testing"
)

func TestNewDriver_UnknownDriver(t *testing.T) {
	driver, err := newDriver(Config{Driver: "nope", Width: defaultWidth, Height: defaultHeight, Length: defaultLength})
	if err == nil || driver != nil {
		t.Fatal("未知驱动应返回错误")
	}
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("未知驱动错误 = %v", err)
	}
}
