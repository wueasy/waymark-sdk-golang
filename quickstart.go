package waymark

import (
	"context"

	"github.com/wueasy/waymark-sdk-golang/internal/quickstart"
)

// 快速初始化组件（配置监听、服务发现解析），由 internal/quickstart 实现。
type (
	// ConfigWatcher 配置监听器。
	ConfigWatcher = quickstart.ConfigWatcher
	// ConfigWatcherOptions 配置监听初始化参数。
	ConfigWatcherOptions = quickstart.ConfigWatcherOptions
	// RegistryResolver 服务发现解析器。
	RegistryResolver = quickstart.RegistryResolver
	// RegistryResolverOptions 服务发现解析器初始化参数。
	RegistryResolverOptions = quickstart.RegistryResolverOptions
	// SelectedInstance 负载均衡选中的实例。
	SelectedInstance = quickstart.SelectedInstance
)

// NewConfigWatcher 创建配置监听器：先为每个 dataId 加载一次当前配置并回调，
// 随后在后台订阅配置变更，变更时自动拉取最新配置并回调。
func (c *Client) NewConfigWatcher(ctx context.Context, opts ConfigWatcherOptions) (*ConfigWatcher, error) {
	return quickstart.NewConfigWatcher(c.core, ctx, opts)
}

// NewRegistryResolver 创建服务发现解析器：拉取一次在线实例并缓存到内存，
// 随后订阅实例变更并定时全量刷新；Self 非空时自动注册并保持心跳。
func (c *Client) NewRegistryResolver(ctx context.Context, opts RegistryResolverOptions) (*RegistryResolver, error) {
	return quickstart.NewRegistryResolver(c.core, ctx, opts)
}
