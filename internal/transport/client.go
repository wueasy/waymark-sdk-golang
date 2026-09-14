// Package transport 实现 Waymark SDK 的底层 HTTP 传输与会话管理，
// 供 internal 下各功能子包复用；根包通过类型别名与转发方法对外暴露，保持调用方式不变。
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/wueasy/waymark-sdk-golang/internal/apierr"
	"github.com/wueasy/waymark-sdk-golang/internal/cache"
)

// DefaultTimeout 单次请求默认超时时间。
const DefaultTimeout = 10 * time.Second

// DefaultCallTimeout 后台任务单次调用（拉取配置、刷新实例等）默认超时时间。
const DefaultCallTimeout = 5 * time.Second

// Config 客户端配置。
type Config struct {
	// Endpoint 服务端地址，例如 http://127.0.0.1:9868，必填。
	// 多个地址使用英文逗号分隔，例如 http://a:9868,http://b:9868；
	// 客户端按顺序进行故障转移：默认使用第一个，遇到网络层错误时自动切换到下一个。
	Endpoint string
	// Username 登录用户名，配置后可在令牌缺失或过期时自动登录。
	Username string
	// Password 登录密码。
	Password string
	// Token 已有的访问令牌，非空时直接使用。
	Token string
	// Timeout 单次请求超时时间，默认 10s（订阅长连接不受此限制）。
	Timeout time.Duration
	// HTTPClient 自定义 HTTP 客户端，为空时使用默认客户端。
	HTTPClient *http.Client
	// CacheDir 本地配置缓存目录，为空时使用系统用户缓存目录下的 waymark 目录。
	CacheDir string
	// DisableCache 关闭本地配置缓存；默认开启，配置中心不可用时回退到本地缓存。
	DisableCache bool
	// Logger 可选的日志记录器，用于输出调试日志；为空时使用 no-op，不产生任何输出。
	Logger *zap.SugaredLogger
}

// Client Waymark 客户端核心，并发安全。
type Client struct {
	// endpoints 服务端地址列表，至少一个，按配置顺序排列。
	endpoints []string
	username  string
	password  string

	// cacheDir 本地配置缓存目录，为空表示未启用缓存。
	cacheDir string

	httpClient   *http.Client
	streamClient *http.Client

	// logger 输出调试日志，永不为 nil。
	logger *zap.SugaredLogger

	mu    sync.RWMutex
	token string
	// active 当前使用的服务端地址下标，网络层错误时递增以故障转移。
	active int
}

// New 创建客户端。
func New(cfg Config) (*Client, error) {
	endpoints, err := parseEndpoints(cfg.Endpoint)
	if err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}

	return &Client{
		endpoints: endpoints,
		username:  strings.TrimSpace(cfg.Username),
		password:  cfg.Password,
		// 订阅长连接不设置整体超时，由 context 控制生命周期。
		streamClient: &http.Client{},
		httpClient:   httpClient,
		cacheDir:     cache.ResolveDir(cfg.DisableCache, cfg.CacheDir),
		token:        strings.TrimSpace(cfg.Token),
		logger:       logger,
	}, nil
}

// parseEndpoints 解析逗号分隔的服务端地址：去除空白与尾部斜杠、去重并校验协议与主机。
func parseEndpoints(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("waymark: endpoint 不能为空")
	}
	endpoints := make([]string, 0, 2)
	seen := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		ep := strings.TrimSpace(part)
		if ep == "" {
			continue
		}
		u, err := url.Parse(ep)
		if err != nil {
			return nil, fmt.Errorf("waymark: endpoint 非法 %q: %w", ep, err)
		}
		if u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("waymark: endpoint 需包含协议与主机，例如 http://127.0.0.1:9868（错误值：%s）", ep)
		}
		ep = strings.TrimRight(ep, "/")
		if _, ok := seen[ep]; ok {
			continue
		}
		seen[ep] = struct{}{}
		endpoints = append(endpoints, ep)
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("waymark: endpoint 不能为空")
	}
	return endpoints, nil
}

// Endpoint 返回当前使用的服务端地址（已去除尾部斜杠）。
func (c *Client) Endpoint() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.endpoints[c.active]
}

// Endpoints 返回全部已配置的服务端地址（副本）。
func (c *Client) Endpoints() []string {
	out := make([]string, len(c.endpoints))
	copy(out, c.endpoints)
	return out
}

// Failover 切换到下一个服务端地址；仅配置了多个地址时生效。
func (c *Client) Failover() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.endpoints) > 1 {
		c.active = (c.active + 1) % len(c.endpoints)
	}
}

// CacheDir 返回本地配置缓存目录，为空表示未启用缓存。
func (c *Client) CacheDir() string {
	return c.cacheDir
}

// HTTPClient 返回普通请求使用的 HTTP 客户端。
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

// StreamClient 返回订阅长连接使用的 HTTP 客户端。
func (c *Client) StreamClient() *http.Client {
	return c.streamClient
}

// SetToken 设置访问令牌。
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = strings.TrimSpace(token)
}

// Token 返回当前访问令牌。
func (c *Client) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// Logger 返回日志记录器，未配置时返回 no-op logger（永不为 nil）。
func (c *Client) Logger() *zap.SugaredLogger {
	return c.logger
}

// ResultVo 服务端统一响应结构。
type ResultVo struct {
	Code       int             `json:"code"`
	Successful bool            `json:"successful"`
	Msg        *string         `json:"msg"`
	Data       json.RawMessage `json:"data"`
	Encrypt    bool            `json:"encrypt"`
}

// ToError 将失败响应转换为 APIError。
func (r *ResultVo) ToError(status int) error {
	if r.Successful {
		return nil
	}
	msg := ""
	if r.Msg != nil {
		msg = *r.Msg
	}
	return &apierr.APIError{Code: r.Code, Msg: msg, HTTPStatus: status}
}

// NewRequest 构建带认证信息与请求体的 HTTP 请求。
func (c *Client) NewRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Request, error) {
	target := c.Endpoint() + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("waymark: 序列化请求体失败: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, fmt.Errorf("waymark: 构建请求失败: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if token := c.Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

// TransportError 表示网络层错误（连接失败、读取响应失败等），
// 与业务错误区分开，用于触发服务端地址的故障转移。
type TransportError struct {
	Err error
}

// Error 实现 error 接口。
func (e *TransportError) Error() string { return e.Err.Error() }

// Unwrap 支持 errors.Is / errors.As。
func (e *TransportError) Unwrap() error { return e.Err }

// IsTransportFailure 判断错误是否为网络层错误。
func IsTransportFailure(err error) bool {
	var te *TransportError
	return errors.As(err, &te)
}

// Execute 发送请求并解析统一响应，返回 data 字段原始内容。
// 配置了多个地址时，遇到网络层错误会依次切换到下一个地址重试（故障转移）；
// 业务错误不做转移，直接返回。
func (c *Client) Execute(req *http.Request, client *http.Client) (json.RawMessage, error) {
	return retryWithFailover(c, req, func() (json.RawMessage, error) {
		return c.executeOnce(req, client)
	})
}

// retryWithFailover 在配置了多个地址时依次重试：网络层错误触发故障转移，业务错误直接返回。
// req 在每次尝试前会被重定向到当前使用的地址；once 负责执行单次请求。
func retryWithFailover[T any](c *Client, req *http.Request, once func() (T, error)) (T, error) {
	var zero T
	total := len(c.endpoints)
	var lastErr error
	for attempt := 0; attempt < total; attempt++ {
		if err := c.redirect(req); err != nil {
			return zero, err
		}
		// 重试前重置请求体，保证请求可被再次发送。
		if attempt > 0 && req.GetBody != nil {
			if body, err := req.GetBody(); err == nil {
				req.Body = body
			}
		}
		result, err := once()
		if err == nil {
			return result, nil
		}
		if !IsTransportFailure(err) {
			return zero, err
		}
		lastErr = err
		if total > 1 && attempt < total-1 {
			c.Failover()
			c.logger.Warnf("waymark: 请求失败，切换到地址 %s: %v", c.Endpoint(), err)
		}
	}
	return zero, lastErr
}

// redirect 将请求指向当前使用的服务端地址（保留路径与查询串）。
func (c *Client) redirect(req *http.Request) error {
	base, err := url.Parse(c.Endpoint())
	if err != nil {
		return fmt.Errorf("waymark: endpoint 非法: %w", err)
	}
	req.URL.Scheme = base.Scheme
	req.URL.Host = base.Host
	req.Host = base.Host
	return nil
}

// executeOnce 使用当前地址发送一次请求并解析响应。
func (c *Client) executeOnce(req *http.Request, client *http.Client) (json.RawMessage, error) {
	start := time.Now()
	c.logger.Debugf("waymark: 请求 %s %s", req.Method, req.URL.String())
	resp, err := client.Do(req)
	if err != nil {
		c.logger.Errorf("waymark: 请求失败 %s %s: %v", req.Method, req.URL.String(), err)
		return nil, &TransportError{Err: fmt.Errorf("waymark: 请求失败: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Errorf("waymark: 读取响应失败 %s %s: %v", req.Method, req.URL.String(), err)
		return nil, &TransportError{Err: fmt.Errorf("waymark: 读取响应失败: %w", err)}
	}

	var rv ResultVo
	if err := json.Unmarshal(data, &rv); err != nil {
		c.logger.Errorf("waymark: 解析响应失败 %s %s (status=%d): %v", req.Method, req.URL.String(), resp.StatusCode, err)
		return nil, fmt.Errorf("waymark: 解析响应失败(status=%d): %w", resp.StatusCode, err)
	}
	if err := rv.ToError(resp.StatusCode); err != nil {
		c.logger.Warnf("waymark: 接口返回失败 %s %s: %v", req.Method, req.URL.String(), err)
		return nil, err
	}
	c.logger.Debugf("waymark: 响应 %s %s status=%d 耗时=%s", req.Method, req.URL.String(), resp.StatusCode, time.Since(start))
	return rv.Data, nil
}

// Send 发送已完成认证的请求，返回 data 原始内容。
// 令牌过期时会自动重新登录并重试一次。
func (c *Client) Send(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, error) {
	if err := c.EnsureToken(ctx); err != nil {
		return nil, err
	}
	req, err := c.NewRequest(ctx, method, path, query, body)
	if err != nil {
		return nil, err
	}
	data, err := c.Execute(req, c.httpClient)
	if err == nil {
		return data, nil
	}
	if !apierr.IsAuthError(err) || !c.CanRelogin() {
		return nil, err
	}
	// 令牌失效，重新登录后重试一次。
	c.logger.Debugf("waymark: 令牌失效，尝试重新登录后重试 %s %s", method, path)
	if loginErr := c.login(ctx); loginErr != nil {
		c.logger.Errorf("waymark: 重新登录失败: %v", loginErr)
		return nil, err
	}
	retryReq, err := c.NewRequest(ctx, method, path, query, body)
	if err != nil {
		return nil, err
	}
	return c.Execute(retryReq, c.httpClient)
}

// SendJSON 发送请求并将 data 解析到 out；out 为 nil 时忽略响应数据。
func (c *Client) SendJSON(ctx context.Context, method, path string, query url.Values, body, out any) error {
	data, err := c.Send(ctx, method, path, query, body)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("waymark: 解析响应数据失败: %w", err)
	}
	return nil
}

// rawResult DoRaw 单次请求的原始响应。
type rawResult struct {
	data   []byte
	header http.Header
	status int
}

// DoRaw 发送请求并返回原始响应体、响应头与状态码，用于处理非统一响应的接口（如 zip 导出）。
// 配置了多个地址时同样支持故障转移。
func (c *Client) DoRaw(ctx context.Context, method, path string, query url.Values, body any, contentType string) ([]byte, http.Header, int, error) {
	if err := c.EnsureToken(ctx); err != nil {
		return nil, nil, 0, err
	}
	req, err := c.NewRequest(ctx, method, path, query, body)
	if err != nil {
		return nil, nil, 0, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := retryWithFailover(c, req, func() (rawResult, error) {
		return c.doRawOnce(req)
	})
	if err != nil {
		return nil, nil, 0, err
	}
	return res.data, res.header, res.status, nil
}

// doRawOnce 使用当前地址发送一次请求并读取原始响应。
func (c *Client) doRawOnce(req *http.Request) (rawResult, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return rawResult{}, &TransportError{Err: fmt.Errorf("waymark: 请求失败: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return rawResult{}, &TransportError{Err: fmt.Errorf("waymark: 读取响应失败: %w", err)}
	}
	return rawResult{data: data, header: resp.Header, status: resp.StatusCode}, nil
}

// SetIfNotEmpty 仅在值非空时写入查询参数。
func SetIfNotEmpty(q url.Values, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		q.Set(key, v)
	}
}

// IsJSONResponse 判断响应内容类型是否为 JSON。
func IsJSONResponse(header http.Header) bool {
	return strings.Contains(header.Get("Content-Type"), "application/json")
}
