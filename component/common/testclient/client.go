package testclient

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTimeout = 15 * time.Second

var defaultClient = New()

// Client 集成测试 HTTP 客户端
type Client struct {
	httpClient *http.Client
}

// New 创建集成测试 HTTP 客户端
func New() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
}

// Get 发起 GET 请求
func Get(url, token string) (body string, ok bool) {
	return defaultClient.Get(url, token)
}

// Post 发起 POST 请求，reqBody 非 nil 时序列化为 JSON
func Post(url, token string, reqBody any) (body string, ok bool) {
	return defaultClient.Post(url, token, reqBody)
}

// Put 发起 PUT 请求，reqBody 非 nil 时序列化为 JSON
func Put(url, token string, reqBody any) (body string, ok bool) {
	return defaultClient.Put(url, token, reqBody)
}

// Delete 发起 DELETE 请求
func Delete(url, token string) (body string, ok bool) {
	return defaultClient.Delete(url, token)
}

// Patch 发起 PATCH 请求，reqBody 非 nil 时序列化为 JSON
func Patch(url, token string, reqBody any) (body string, ok bool) {
	return defaultClient.Patch(url, token, reqBody)
}

// Get 发起 GET 请求
func (c *Client) Get(url, token string) (body string, ok bool) {
	return c.do(http.MethodGet, url, token, nil)
}

// Post 发起 POST 请求，reqBody 非 nil 时序列化为 JSON
func (c *Client) Post(url, token string, reqBody any) (body string, ok bool) {
	return c.do(http.MethodPost, url, token, reqBody)
}

// Put 发起 PUT 请求，reqBody 非 nil 时序列化为 JSON
func (c *Client) Put(url, token string, reqBody any) (body string, ok bool) {
	return c.do(http.MethodPut, url, token, reqBody)
}

// Delete 发起 DELETE 请求
func (c *Client) Delete(url, token string) (body string, ok bool) {
	return c.do(http.MethodDelete, url, token, nil)
}

// Patch 发起 PATCH 请求，reqBody 非 nil 时序列化为 JSON
func (c *Client) Patch(url, token string, reqBody any) (body string, ok bool) {
	return c.do(http.MethodPatch, url, token, reqBody)
}

// do 执行 HTTP 请求并返回响应体字符串，HTTP 状态码为 200 时第二个返回值为 true
func (c *Client) do(method, url, token string, reqBody any) (string, bool) {
	if c == nil || c.httpClient == nil {
		return "testclient: HTTP 客户端未初始化", false
	}
	if strings.TrimSpace(url) == "" {
		return "testclient: 请求地址不能为空", false
	}

	var bodyReader io.Reader
	if reqBody != nil {
		payload, err := json.Marshal(reqBody)
		if err != nil {
			return "testclient: 序列化请求体失败: " + err.Error(), false
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return "testclient: 创建请求失败: " + err.Error(), false
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	setAuthorization(req, token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "testclient: 请求失败: " + err.Error(), false
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "testclient: 读取响应失败: " + err.Error(), false
	}
	return string(respBody), resp.StatusCode == http.StatusOK
}

// setAuthorization 根据 token 设置 Authorization 请求头，支持裸 token 与 Bearer 前缀两种形式
func setAuthorization(req *http.Request, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	if len(token) >= 7 && strings.EqualFold(token[:7], "Bearer ") {
		req.Header.Set("Authorization", token)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
}
