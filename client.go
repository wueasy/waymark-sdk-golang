// Package waymark 是 Waymark 配置中心与注册中心的 Go 客户端 SDK。
//
// 基本用法：
//
//	client, err := waymark.NewClient(waymark.Config{
//		Endpoint: "http://127.0.0.1:9868",
//		Username: "admin",
//		Password: "admin",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	item, err := client.GetConfig(ctx, "public", "DEFAULT_GROUP", "app.yaml")
//
// 客户端在令牌缺失或过期时会自动使用配置的账号密码登录。
// GetConfig 会在本地缓存一份配置快照，配置中心不可用时自动回退到本地缓存。
//
// Endpoint 支持配置多个服务端地址（英文逗号分隔），例如
//
//	Endpoint: "http://a:9868,http://b:9868"
//
// 默认使用第一个地址；当请求遇到网络层错误（连接失败、读取响应失败等）时，
// 会自动切换到下一个地址重试（故障转移）。业务错误不会触发切换。
//
// 本包对外暴露全部公开 API；具体实现按功能拆分在 internal 子包中，调用方式保持不变。
package waymark

import (
	"github.com/wueasy/waymark-sdk-golang/internal/transport"
)

// Config 客户端配置。
type Config = transport.Config

// Client Waymark 客户端，并发安全。
type Client struct {
	core *transport.Client
}

// NewClient 创建客户端。
func NewClient(cfg Config) (*Client, error) {
	core, err := transport.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{core: core}, nil
}

// SetToken 设置访问令牌。
func (c *Client) SetToken(token string) {
	c.core.SetToken(token)
}

// Token 返回当前访问令牌。
func (c *Client) Token() string {
	return c.core.Token()
}
