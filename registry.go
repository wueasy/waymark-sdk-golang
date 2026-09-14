package waymark

import (
	"context"

	"github.com/wueasy/waymark-sdk-golang/internal/registry"
)

// RegisterInstance 注册实例，实例已存在时更新并返回 created=false。
func (c *Client) RegisterInstance(ctx context.Context, req InstanceRequest) (*RegisterInstanceResult, error) {
	return registry.RegisterInstance(c.core, ctx, req)
}

// UpdateInstance 更新实例属性。
func (c *Client) UpdateInstance(ctx context.Context, req InstanceRequest) error {
	return registry.UpdateInstance(c.core, ctx, req)
}

// DeregisterInstance 注销实例。
func (c *Client) DeregisterInstance(ctx context.Context, namespace, group, service, ip string, port int) error {
	return registry.DeregisterInstance(c.core, ctx, namespace, group, service, ip, port)
}

// Beat 发送实例心跳。
func (c *Client) Beat(ctx context.Context, namespace, group, service, ip string, port int) error {
	return registry.Beat(c.core, ctx, namespace, group, service, ip, port)
}

// ListInstances 查询实例列表，group/service 为空表示不过滤。
func (c *Client) ListInstances(ctx context.Context, namespace, group, service string) ([]Instance, error) {
	return registry.ListInstances(c.core, ctx, namespace, group, service)
}

// ListServices 查询服务概览列表，group 为空表示不过滤，返回该命名空间下的全部服务。
func (c *Client) ListServices(ctx context.Context, namespace, group string) ([]ServiceSummary, error) {
	return registry.ListServices(c.core, ctx, namespace, group)
}
