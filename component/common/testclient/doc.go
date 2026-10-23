// Package testclient 提供接口集成测试用的 HTTP 请求封装。
//
// 测试用例只需传入完整 URL、Token 与请求参数对象，即可发起 HTTP 调用并获取响应。
// 返回值 body 为响应体字符串，ok 在 HTTP 状态码为 200 时为 true，否则为 false。
//
// 业务响应解析请直接调用已注册 Encoder（response.Enc().Parse / IsSuccess）；经 boot 启动时已默认注册，
// 直接构造测试进程时须先 MustRegister 或使用 RegisterDefaultForTest。
//
// 使用示例：
//
//	body, ok := testclient.Post("http://127.0.0.1:8080/api/login", "", loginReq)
//	if !ok {
//	    t.Fatalf("login failed: %s", body)
//	}
//	parsed, ok := response.Enc().Parse([]byte(body))
//	if !ok || !response.Enc().IsSuccess(parsed.Code) {
//	    t.Fatalf("biz failed: %s", body)
//	}
//
// @author zwei
package testclient
