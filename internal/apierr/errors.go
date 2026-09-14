// Package apierr 定义 Waymark SDK 的错误类型与判定方法。
package apierr

import (
	"errors"
	"fmt"
	"net/http"
)

// 服务端统一返回码。
const (
	// CodeFail 业务失败。
	CodeFail = 1001
	// CodeUnauthorized 未认证。
	CodeUnauthorized = 401
	// CodeForbidden 无权限。
	CodeForbidden = 403
)

// ErrNoCredentials 未配置用户名密码且未提供令牌。
var ErrNoCredentials = errors.New("waymark: 未配置用户名密码，无法登录")

// APIError 服务端返回的业务错误。
type APIError struct {
	Code       int    // 业务返回码
	Msg        string // 错误信息
	HTTPStatus int    // HTTP 状态码
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("waymark: 请求失败(code=%d, status=%d)", e.Code, e.HTTPStatus)
	}
	return fmt.Sprintf("waymark: %s (code=%d)", e.Msg, e.Code)
}

// IsUnauthorized 判断错误是否为未认证。
func IsUnauthorized(err error) bool {
	var e *APIError
	if errors.As(err, &e) {
		return e.Code == CodeUnauthorized || e.HTTPStatus == http.StatusUnauthorized
	}
	return false
}

// IsForbidden 判断错误是否为无权限。
func IsForbidden(err error) bool {
	var e *APIError
	if errors.As(err, &e) {
		return e.Code == CodeForbidden || e.HTTPStatus == http.StatusForbidden
	}
	return false
}

// IsAuthError 判断错误是否需要重新登录。
func IsAuthError(err error) bool {
	return IsUnauthorized(err)
}
