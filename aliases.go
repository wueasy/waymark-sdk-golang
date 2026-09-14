package waymark

import (
	"github.com/wueasy/waymark-sdk-golang/internal/apierr"
	"github.com/wueasy/waymark-sdk-golang/internal/model"
)

// 默认命名空间、分组与集群。
const (
	DefaultNamespace = model.DefaultNamespace
	DefaultGroup     = model.DefaultGroup
	DefaultCluster   = model.DefaultCluster
)

// 配置类型。
const (
	ConfigTypeText       = model.ConfigTypeText
	ConfigTypeJSON       = model.ConfigTypeJSON
	ConfigTypeYAML       = model.ConfigTypeYAML
	ConfigTypeProperties = model.ConfigTypeProperties
	ConfigTypeXML        = model.ConfigTypeXML
)

// 订阅事件类型。
const (
	// EventTypeConfig 配置变更事件。
	EventTypeConfig = model.EventTypeConfig
	// EventTypeInstance 实例变更事件。
	EventTypeInstance = model.EventTypeInstance
)

// 服务端统一返回码。
const (
	// CodeFail 业务失败。
	CodeFail = apierr.CodeFail
	// CodeUnauthorized 未认证。
	CodeUnauthorized = apierr.CodeUnauthorized
	// CodeForbidden 无权限。
	CodeForbidden = apierr.CodeForbidden
)

// ErrNoCredentials 未配置用户名密码且未提供令牌。
var ErrNoCredentials = apierr.ErrNoCredentials

// APIError 服务端返回的业务错误。
type APIError = apierr.APIError

// IsUnauthorized 判断错误是否为未认证。
func IsUnauthorized(err error) bool {
	return apierr.IsUnauthorized(err)
}

// IsForbidden 判断错误是否为无权限。
func IsForbidden(err error) bool {
	return apierr.IsForbidden(err)
}

// 公共数据模型。
type (
	// RoleBrief 角色精简信息。
	RoleBrief = model.RoleBrief
	// UserProfile 用户资料、角色与命名空间权限。
	UserProfile = model.UserProfile
	// ConfigItem 配置项。
	ConfigItem = model.ConfigItem
	// ConfigHistory 配置历史版本。
	ConfigHistory = model.ConfigHistory
	// ConfigPage 分页配置。
	ConfigPage = model.ConfigPage
	// ImportResult 配置导入结果统计。
	ImportResult = model.ImportResult
	// ListConfigsOptions 配置列表查询条件。
	ListConfigsOptions = model.ListConfigsOptions
	// PublishConfigRequest 发布配置参数。
	PublishConfigRequest = model.PublishConfigRequest
	// ExportItem 待导出配置的定位信息。
	ExportItem = model.ExportItem
	// ExportOptions 导出配置参数。
	ExportOptions = model.ExportOptions
	// Instance 服务实例。
	Instance = model.Instance
	// ServiceSummary 服务概览。
	ServiceSummary = model.ServiceSummary
	// ServicePage 分页服务概览。
	ServicePage = model.ServicePage
	// InstanceRequest 注册或更新实例参数。
	InstanceRequest = model.InstanceRequest
	// RegisterInstanceResult 实例注册结果。
	RegisterInstanceResult = model.RegisterInstanceResult
	// SubscribeOptions 订阅参数。
	SubscribeOptions = model.SubscribeOptions
	// Event 变更事件（订阅推送）。
	Event = model.Event
	// LoginResult 登录结果。
	LoginResult = model.LoginResult
)
