package waymark

import (
	"context"

	"github.com/wueasy/waymark-sdk-golang/internal/config"
)

// ListConfigs 分页查询配置列表。
func (c *Client) ListConfigs(ctx context.Context, opts ListConfigsOptions) (*ConfigPage, error) {
	return config.ListConfigs(c.core, ctx, opts)
}

// GetConfig 查询配置详情。读取成功时会写入本地缓存；配置中心不可用（网络层错误）
// 且本地存在该配置的缓存时，返回缓存内容并忽略错误。
func (c *Client) GetConfig(ctx context.Context, namespace, group, dataId string) (*ConfigItem, error) {
	return config.GetConfig(c.core, ctx, namespace, group, dataId)
}

// PublishConfig 发布或更新配置。
func (c *Client) PublishConfig(ctx context.Context, req PublishConfigRequest) error {
	return config.PublishConfig(c.core, ctx, req)
}

// DeleteConfig 删除配置。
func (c *Client) DeleteConfig(ctx context.Context, namespace, group, dataId string) error {
	return config.DeleteConfig(c.core, ctx, namespace, group, dataId)
}

// ConfigHistory 查询配置历史版本。
func (c *Client) ConfigHistory(ctx context.Context, namespace, group, dataId string) ([]ConfigHistory, error) {
	return config.ConfigHistory(c.core, ctx, namespace, group, dataId)
}

// RestoreConfig 将指定历史版本还原为当前配置。
func (c *Client) RestoreConfig(ctx context.Context, namespace, group, dataId string, historyId int64) error {
	return config.RestoreConfig(c.core, ctx, namespace, group, dataId, historyId)
}

// ExportConfigs 导出配置为 zip 字节流。
func (c *Client) ExportConfigs(ctx context.Context, opts ExportOptions) ([]byte, error) {
	return config.ExportConfigs(c.core, ctx, opts)
}

// ImportConfigs 从 zip 字节流导入配置。
// group 非空时全部导入到该分组，否则沿用压缩包中记录的原分组。
func (c *Client) ImportConfigs(ctx context.Context, namespace, group string, zipData []byte) (*ImportResult, error) {
	return config.ImportConfigs(c.core, ctx, namespace, group, zipData)
}
