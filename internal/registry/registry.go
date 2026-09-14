// Package registry 实现注册中心的实例注册、心跳与查询接口。
package registry

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/wueasy/waymark-sdk-golang/internal/model"
	"github.com/wueasy/waymark-sdk-golang/internal/transport"
)

type instanceRequest struct {
	Namespace   string            `json:"namespace"`
	GroupName   string            `json:"groupName"`
	ServiceName string            `json:"serviceName"`
	ClusterName string            `json:"clusterName"`
	Ip          string            `json:"ip"`
	Port        int               `json:"port"`
	Weight      float64           `json:"weight"`
	Healthy     *int              `json:"healthy"`
	Ephemeral   *int              `json:"ephemeral"`
	Metadata    map[string]string `json:"metadata"`
}

func toBody(r model.InstanceRequest) instanceRequest {
	return instanceRequest{
		Namespace:   r.Namespace,
		GroupName:   r.GroupName,
		ServiceName: r.ServiceName,
		ClusterName: r.ClusterName,
		Ip:          r.Ip,
		Port:        r.Port,
		Weight:      r.Weight,
		Healthy:     r.Healthy,
		Ephemeral:   r.Ephemeral,
		Metadata:    r.Metadata,
	}
}

// RegisterInstance 注册实例，实例已存在时更新并返回 created=false。
func RegisterInstance(c *transport.Client, ctx context.Context, req model.InstanceRequest) (*model.RegisterInstanceResult, error) {
	var out model.RegisterInstanceResult
	if err := c.SendJSON(ctx, http.MethodPost, "/api/registry/instance", nil, toBody(req), &out); err != nil {
		return nil, err
	}
	c.Logger().Infof("waymark: 注册实例成功 service=%s address=%s:%d created=%t", req.ServiceName, req.Ip, req.Port, out.Created)
	return &out, nil
}

// UpdateInstance 更新实例属性。
func UpdateInstance(c *transport.Client, ctx context.Context, req model.InstanceRequest) error {
	if err := c.SendJSON(ctx, http.MethodPut, "/api/registry/instance", nil, toBody(req), nil); err != nil {
		return err
	}
	c.Logger().Debugf("waymark: 更新实例成功 service=%s address=%s:%d", req.ServiceName, req.Ip, req.Port)
	return nil
}

// DeregisterInstance 注销实例。
func DeregisterInstance(c *transport.Client, ctx context.Context, namespace, group, service, ip string, port int) error {
	q := url.Values{}
	transport.SetIfNotEmpty(q, "namespace", namespace)
	transport.SetIfNotEmpty(q, "groupName", group)
	q.Set("serviceName", service)
	q.Set("ip", ip)
	q.Set("port", strconv.Itoa(port))

	if err := c.SendJSON(ctx, http.MethodDelete, "/api/registry/instance", q, nil, nil); err != nil {
		return err
	}
	c.Logger().Infof("waymark: 注销实例成功 service=%s address=%s:%d", service, ip, port)
	return nil
}

// Beat 发送实例心跳。
func Beat(c *transport.Client, ctx context.Context, namespace, group, service, ip string, port int) error {
	q := url.Values{}
	transport.SetIfNotEmpty(q, "namespace", namespace)
	transport.SetIfNotEmpty(q, "groupName", group)
	q.Set("serviceName", service)
	q.Set("ip", ip)
	q.Set("port", strconv.Itoa(port))

	if err := c.SendJSON(ctx, http.MethodPut, "/api/registry/beat", q, nil, nil); err != nil {
		return err
	}
	c.Logger().Debugf("waymark: 实例心跳成功 service=%s address=%s:%d", service, ip, port)
	return nil
}

// ListInstances 查询实例列表，group/service 为空表示不过滤。
func ListInstances(c *transport.Client, ctx context.Context, namespace, group, service string) ([]model.Instance, error) {
	q := url.Values{}
	transport.SetIfNotEmpty(q, "namespace", namespace)
	transport.SetIfNotEmpty(q, "groupName", group)
	transport.SetIfNotEmpty(q, "serviceName", service)

	var out []model.Instance
	if err := c.SendJSON(ctx, http.MethodGet, "/api/registry/instances", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListServices 查询服务概览列表，group 为空表示不过滤，返回该命名空间下的全部服务。
func ListServices(c *transport.Client, ctx context.Context, namespace, group string) ([]model.ServiceSummary, error) {
	// 服务端按页返回且单页有上限，这里循环拉取所有分页后合并为完整列表。
	const pageSize = 200
	out := make([]model.ServiceSummary, 0)
	for pageNum := 1; ; pageNum++ {
		q := url.Values{}
		transport.SetIfNotEmpty(q, "namespace", namespace)
		transport.SetIfNotEmpty(q, "groupName", group)
		q.Set("pageNum", strconv.Itoa(pageNum))
		q.Set("pageSize", strconv.Itoa(pageSize))

		var page model.ServicePage
		if err := c.SendJSON(ctx, http.MethodGet, "/api/registry/services", q, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.List...)
		if len(page.List) == 0 || int64(len(out)) >= page.Total {
			break
		}
	}
	return out, nil
}
