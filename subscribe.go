package waymark

import (
	"context"

	"github.com/wueasy/waymark-sdk-golang/internal/subscribe"
)

// Watch 建立一条订阅连接，同时订阅配置与实例变更，阻塞直到 ctx 结束；连接断开后会自动重连。
// DataIds 为要订阅的 dataId 列表：传 "*" 表示订阅该分组下全部配置变更，为空表示不订阅配置；
// ServiceName 为空表示订阅该分组下全部服务变更。
func (c *Client) Watch(ctx context.Context, opts SubscribeOptions) error {
	return subscribe.Watch(c.core, ctx, opts)
}
