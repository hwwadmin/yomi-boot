package response

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// TestMustRegister_RejectDuplicateAndUnregistered 验证未注册时 Enc 应 panic
func TestMustRegister_RejectDuplicateAndUnregistered(t *testing.T) {
	resetRegisterForTest()
	t.Cleanup(resetRegisterForTest)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("未注册时 Enc() 应 panic")
		}
	}()
	_ = Enc()
}

// TestMustRegister_DuplicatePanics 验证重复 MustRegister 应 panic
func TestMustRegister_DuplicatePanics(t *testing.T) {
	resetRegisterForTest()
	t.Cleanup(resetRegisterForTest)

	MustRegister(nopEncoder{})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("重复 MustRegister 应 panic")
		}
	}()
	MustRegister(nopEncoder{})
}

// nopEncoder 空实现 Encoder（测试桩）
type nopEncoder struct{}

// EncodeSuccess 实现 EncodeSuccess 接口（测试桩）
func (nopEncoder) EncodeSuccess(_ *gin.Context, _ any) {}

// EncodePage 实现 EncodePage 接口（测试桩）
func (nopEncoder) EncodePage(_ *gin.Context, _ any, _ int64) {}

// EncodeFailure 实现 EncodeFailure 接口（测试桩）
func (nopEncoder) EncodeFailure(_ *gin.Context, _ error) {}

// Parse 实现 Parse 接口（测试桩）
func (nopEncoder) Parse(_ []byte) (Response, bool) { return Response{}, false }

// IsSuccess 实现 IsSuccess 接口（测试桩）
func (nopEncoder) IsSuccess(_ string) bool { return false }
