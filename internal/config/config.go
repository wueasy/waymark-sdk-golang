// Package config 实现配置中心的读写、历史、导入导出等接口。
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"

	"github.com/wueasy/waymark-sdk-golang/internal/cache"
	"github.com/wueasy/waymark-sdk-golang/internal/model"
	"github.com/wueasy/waymark-sdk-golang/internal/transport"
)

type configRequest struct {
	Namespace string `json:"namespace"`
	GroupName string `json:"groupName"`
	DataId    string `json:"dataId"`
	Content   string `json:"content"`
	Type      string `json:"type"`
}

type configRestoreRequest struct {
	Namespace string `json:"namespace"`
	GroupName string `json:"groupName"`
	DataId    string `json:"dataId"`
	HistoryId int64  `json:"historyId"`
}

type configExportRequest struct {
	Namespace string             `json:"namespace"`
	GroupName string             `json:"groupName"`
	DataId    string             `json:"dataId"`
	Items     []model.ExportItem `json:"items"`
}

// ListConfigs 分页查询配置列表。
func ListConfigs(c *transport.Client, ctx context.Context, opts model.ListConfigsOptions) (*model.ConfigPage, error) {
	q := url.Values{}
	transport.SetIfNotEmpty(q, "namespace", opts.Namespace)
	transport.SetIfNotEmpty(q, "groupName", opts.GroupName)
	transport.SetIfNotEmpty(q, "dataId", opts.DataId)
	if opts.PageNum > 0 {
		q.Set("pageNum", strconv.Itoa(opts.PageNum))
	}
	if opts.PageSize > 0 {
		q.Set("pageSize", strconv.Itoa(opts.PageSize))
	}

	var out model.ConfigPage
	if err := c.SendJSON(ctx, http.MethodGet, "/api/configs", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetConfig 查询配置详情。读取成功时会写入本地缓存；配置中心不可用（网络层错误）
// 且本地存在该配置的缓存时，返回缓存内容并忽略错误。
func GetConfig(c *transport.Client, ctx context.Context, namespace, group, dataId string) (*model.ConfigItem, error) {
	q := url.Values{}
	transport.SetIfNotEmpty(q, "namespace", namespace)
	transport.SetIfNotEmpty(q, "groupName", group)
	q.Set("dataId", dataId)

	var out model.ConfigItem
	if err := c.SendJSON(ctx, http.MethodGet, "/api/configs/detail", q, nil, &out); err != nil {
		if c.CacheDir() != "" && cache.IsUnreachable(err) {
			if item, cacheErr := cache.Read(c.CacheDir(), namespace, group, dataId); cacheErr == nil {
				c.Logger().Warnf("waymark: 配置中心不可用，回退本地缓存 namespace=%s group=%s dataId=%s", namespace, group, dataId)
				return item, nil
			}
		}
		return nil, err
	}
	cache.Write(c.CacheDir(), &out)
	c.Logger().Debugf("waymark: 拉取配置成功 namespace=%s group=%s dataId=%s", namespace, group, dataId)
	return &out, nil
}

// PublishConfig 发布或更新配置。
func PublishConfig(c *transport.Client, ctx context.Context, req model.PublishConfigRequest) error {
	body := configRequest{
		Namespace: req.Namespace,
		GroupName: req.GroupName,
		DataId:    req.DataId,
		Content:   req.Content,
		Type:      req.Type,
	}
	if err := c.SendJSON(ctx, http.MethodPost, "/api/configs", nil, body, nil); err != nil {
		return err
	}
	c.Logger().Infof("waymark: 发布配置成功 namespace=%s group=%s dataId=%s", req.Namespace, req.GroupName, req.DataId)
	return nil
}

// DeleteConfig 删除配置。
func DeleteConfig(c *transport.Client, ctx context.Context, namespace, group, dataId string) error {
	q := url.Values{}
	transport.SetIfNotEmpty(q, "namespace", namespace)
	transport.SetIfNotEmpty(q, "groupName", group)
	q.Set("dataId", dataId)

	if err := c.SendJSON(ctx, http.MethodDelete, "/api/configs", q, nil, nil); err != nil {
		return err
	}
	c.Logger().Infof("waymark: 删除配置成功 namespace=%s group=%s dataId=%s", namespace, group, dataId)
	return nil
}

// configHistoryPage 配置历史分页响应。
type configHistoryPage struct {
	List     []model.ConfigHistory `json:"list"`
	Total    int64                 `json:"total"`
	PageNum  int                   `json:"pageNum"`
	PageSize int                   `json:"pageSize"`
}

// ConfigHistory 查询配置历史版本。服务端按页返回且单页有上限，这里循环拉取所有分页后合并为完整列表。
func ConfigHistory(c *transport.Client, ctx context.Context, namespace, group, dataId string) ([]model.ConfigHistory, error) {
	const pageSize = 200
	out := make([]model.ConfigHistory, 0)
	for pageNum := 1; ; pageNum++ {
		q := url.Values{}
		transport.SetIfNotEmpty(q, "namespace", namespace)
		transport.SetIfNotEmpty(q, "groupName", group)
		q.Set("dataId", dataId)
		q.Set("pageNum", strconv.Itoa(pageNum))
		q.Set("pageSize", strconv.Itoa(pageSize))

		var page configHistoryPage
		if err := c.SendJSON(ctx, http.MethodGet, "/api/configs/history", q, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.List...)
		if len(page.List) == 0 || int64(len(out)) >= page.Total {
			break
		}
	}
	return out, nil
}

// RestoreConfig 将指定历史版本还原为当前配置。
func RestoreConfig(c *transport.Client, ctx context.Context, namespace, group, dataId string, historyId int64) error {
	body := configRestoreRequest{
		Namespace: namespace,
		GroupName: group,
		DataId:    dataId,
		HistoryId: historyId,
	}
	return c.SendJSON(ctx, http.MethodPost, "/api/configs/restore", nil, body, nil)
}

// ExportConfigs 导出配置为 zip 字节流。
func ExportConfigs(c *transport.Client, ctx context.Context, opts model.ExportOptions) ([]byte, error) {
	body := configExportRequest{
		Namespace: opts.Namespace,
		GroupName: opts.GroupName,
		DataId:    opts.DataId,
		Items:     opts.Items,
	}

	data, header, status, err := c.DoRaw(ctx, http.MethodPost, "/api/configs/export", nil, body, "application/json")
	if err != nil {
		return nil, err
	}
	// 导出失败时服务端返回 JSON 错误体，成功时返回 zip 二进制流。
	if transport.IsJSONResponse(header) {
		var rv transport.ResultVo
		if err := json.Unmarshal(data, &rv); err == nil {
			if apiErr := rv.ToError(status); apiErr != nil {
				return nil, apiErr
			}
		}
		return nil, fmt.Errorf("waymark: 导出配置失败")
	}
	return data, nil
}

// ImportConfigs 从 zip 字节流导入配置。
// group 非空时全部导入到该分组，否则沿用压缩包中记录的原分组。
func ImportConfigs(c *transport.Client, ctx context.Context, namespace, group string, zipData []byte) (*model.ImportResult, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if namespace != "" {
		if err := writer.WriteField("namespace", namespace); err != nil {
			return nil, fmt.Errorf("waymark: 构建导入请求失败: %w", err)
		}
	}
	if group != "" {
		if err := writer.WriteField("groupName", group); err != nil {
			return nil, fmt.Errorf("waymark: 构建导入请求失败: %w", err)
		}
	}
	fileWriter, err := writer.CreateFormFile("file", "configs.zip")
	if err != nil {
		return nil, fmt.Errorf("waymark: 构建导入请求失败: %w", err)
	}
	if _, err := fileWriter.Write(zipData); err != nil {
		return nil, fmt.Errorf("waymark: 构建导入请求失败: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("waymark: 构建导入请求失败: %w", err)
	}

	req, err := c.NewRequest(ctx, http.MethodPost, "/api/configs/import", nil, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
	req.ContentLength = int64(buf.Len())

	data, err := c.Execute(req, c.HTTPClient())
	if err != nil {
		return nil, err
	}
	var out model.ImportResult
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("waymark: 解析导入结果失败: %w", err)
	}
	return &out, nil
}
