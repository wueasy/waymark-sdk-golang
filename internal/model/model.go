// Package model 定义 Waymark SDK 的公共数据模型与常量，
// 由根包以类型别名方式对外暴露，保证调用方引用路径不变。
package model

import "time"

// 默认命名空间、分组与集群。
const (
	DefaultNamespace = "public"
	DefaultGroup     = "DEFAULT_GROUP"
	DefaultCluster   = "DEFAULT"
)

// 配置类型。
const (
	ConfigTypeText       = "text"
	ConfigTypeJSON       = "json"
	ConfigTypeYAML       = "yaml"
	ConfigTypeProperties = "properties"
	ConfigTypeXML        = "xml"
)

// 订阅事件类型。
const (
	// EventTypeConfig 配置变更事件。
	EventTypeConfig = "CONFIG"
	// EventTypeInstance 实例变更事件。
	EventTypeInstance = "INSTANCE"
)

// RoleBrief 角色精简信息。
type RoleBrief struct {
	Id   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// UserProfile 用户资料、角色与命名空间权限。
type UserProfile struct {
	Id          int64             `json:"id"`
	Username    string            `json:"username"`
	Nickname    string            `json:"nickname"`
	Status      int               `json:"status"`
	IsAdmin     bool              `json:"isAdmin"`
	Roles       []RoleBrief       `json:"roles"`
	Permissions map[string]string `json:"permissions"`
}

// ConfigItem 配置项。
type ConfigItem struct {
	Id         int64  `json:"id"`
	Namespace  string `json:"namespace"`
	GroupName  string `json:"groupName"`
	DataId     string `json:"dataId"`
	Content    string `json:"content"`
	Md5        string `json:"md5"`
	Type       string `json:"type"`
	CreateTime int64  `json:"createTime"`
	UpdateTime int64  `json:"updateTime"`
}

// ConfigHistory 配置历史版本。
type ConfigHistory struct {
	Id         int64  `json:"id"`
	Namespace  string `json:"namespace"`
	GroupName  string `json:"groupName"`
	DataId     string `json:"dataId"`
	Content    string `json:"content"`
	Md5        string `json:"md5"`
	Type       string `json:"type"`
	CreateTime int64  `json:"createTime"`
}

// ConfigPage 分页配置。
type ConfigPage struct {
	List     []ConfigItem `json:"list"`
	Total    int64        `json:"total"`
	PageNum  int          `json:"pageNum"`
	PageSize int          `json:"pageSize"`
}

// ImportResult 配置导入结果统计。
type ImportResult struct {
	Imported int      `json:"imported"`
	Failed   []string `json:"failed"`
}

// ListConfigsOptions 配置列表查询条件。
type ListConfigsOptions struct {
	// Namespace 命名空间，为空时使用默认命名空间。
	Namespace string
	// GroupName 分组，为空表示不过滤。
	GroupName string
	// DataId 配置标识，为空表示不过滤。
	DataId string
	// PageNum 页码，从 1 开始。
	PageNum int
	// PageSize 每页条数。
	PageSize int
}

// PublishConfigRequest 发布配置参数。
type PublishConfigRequest struct {
	Namespace string
	GroupName string
	DataId    string
	Content   string
	// Type 配置类型：text/json/yaml/properties/xml，为空时服务端按 text 处理。
	Type string
}

// ExportItem 待导出配置的定位信息。
type ExportItem struct {
	GroupName string `json:"groupName"`
	DataId    string `json:"dataId"`
}

// ExportOptions 导出配置参数。
// Items 非空时按指定配置导出；否则按 Namespace/GroupName/DataId 过滤导出全部匹配配置。
type ExportOptions struct {
	Namespace string
	GroupName string
	DataId    string
	Items     []ExportItem
}

// Instance 服务实例。
type Instance struct {
	Id            int64             `json:"id"`
	Namespace     string            `json:"namespace"`
	GroupName     string            `json:"groupName"`
	ServiceName   string            `json:"serviceName"`
	ClusterName   string            `json:"clusterName"`
	Ip            string            `json:"ip"`
	Port          int               `json:"port"`
	Weight        float64           `json:"weight"`
	Healthy       int               `json:"healthy"`
	Ephemeral     int               `json:"ephemeral"`
	Metadata      map[string]string `json:"metadata"`
	LastHeartbeat int64             `json:"lastHeartbeat"`
	CreateTime    int64             `json:"createTime"`
	UpdateTime    int64             `json:"updateTime"`
}

// ServiceSummary 服务概览。
type ServiceSummary struct {
	Namespace     string `json:"namespace"`
	GroupName     string `json:"groupName"`
	ServiceName   string `json:"serviceName"`
	InstanceCount int    `json:"instanceCount"`
	HealthyCount  int    `json:"healthyCount"`
}

// ServicePage 分页服务概览。
type ServicePage struct {
	List     []ServiceSummary `json:"list"`
	Total    int64            `json:"total"`
	PageNum  int              `json:"pageNum"`
	PageSize int              `json:"pageSize"`
}

// InstanceRequest 注册或更新实例参数。
type InstanceRequest struct {
	Namespace   string
	GroupName   string
	ServiceName string
	ClusterName string
	Ip          string
	Port        int
	// Weight 权重，小于等于 0 时服务端按 1 处理。
	Weight float64
	// Healthy 健康状态，nil 表示由服务端默认（1）。
	Healthy *int
	// Ephemeral 是否为临时实例，nil 表示由服务端默认（1）。
	Ephemeral *int
	Metadata  map[string]string
}

// RegisterInstanceResult 实例注册结果。
type RegisterInstanceResult struct {
	// Created 是否为新建（false 表示更新了已有实例）。
	Created  bool     `json:"created"`
	Instance Instance `json:"instance"`
}

// SubscribeOptions 订阅参数。
type SubscribeOptions struct {
	// Namespace 命名空间，为空时使用默认命名空间。
	Namespace string
	// GroupName 分组，为空时使用默认分组。
	GroupName string
	// DataIds 订阅的配置文件名（dataId）列表，支持一次订阅多个文件；传 "*" 表示订阅该分组下全部配置变更，
	// 为空表示不订阅配置（仅订阅实例变更）。
	DataIds []string
	// ServiceName 订阅实例变更时使用；为空表示订阅该分组下全部服务变更。
	ServiceName string
	// Handler 事件回调，收到变更事件时同步调用。
	Handler func(Event)
	// ReconnectDelay 断线重连间隔，默认 3s。
	ReconnectDelay time.Duration
}

// Event 变更事件（订阅推送），仅包含定位信息，客户端收到后需自行拉取最新数据。
type Event struct {
	EventType string `json:"eventType"`
	Namespace string `json:"namespace"`
	Group     string `json:"group"`
	WatchKey  string `json:"watchKey"`
	Md5       string `json:"md5"`
}

// LoginResult 登录结果。
type LoginResult struct {
	// Token 访问令牌，同时会被缓存到客户端。
	Token string `json:"token"`
	// ExpiresIn 令牌有效期（秒）。
	ExpiresIn int64 `json:"expiresIn"`
	// User 登录用户资料。
	User UserProfile `json:"user"`
}
