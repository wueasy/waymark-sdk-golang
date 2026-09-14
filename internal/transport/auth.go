package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wueasy/waymark-sdk-golang/internal/apierr"
	"github.com/wueasy/waymark-sdk-golang/internal/model"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type initRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

// EnsureToken 确保存在访问令牌，缺失时尝试自动登录。
func (c *Client) EnsureToken(ctx context.Context) error {
	if c.Token() != "" {
		return nil
	}
	return c.login(ctx)
}

// CanRelogin 判断是否具备自动登录所需的账号信息。
func (c *Client) CanRelogin() bool {
	return c.username != "" && c.password != ""
}

// login 使用客户端配置的账号密码登录并缓存令牌。
func (c *Client) login(ctx context.Context) error {
	if !c.CanRelogin() {
		return apierr.ErrNoCredentials
	}
	c.logger.Debugf("waymark: 使用账号 %s 自动登录", c.username)
	req, err := c.NewRequest(ctx, http.MethodPost, "/api/auth/login", nil, loginRequest{
		Username: c.username,
		Password: c.password,
	})
	if err != nil {
		return err
	}
	req.Header.Del("Authorization")

	data, err := c.Execute(req, c.httpClient)
	if err != nil {
		return err
	}
	var out model.LoginResult
	if err := json.Unmarshal(data, &out); err != nil {
		return fmt.Errorf("waymark: 解析登录结果失败: %w", err)
	}
	c.SetToken(out.Token)
	c.logger.Debugf("waymark: 自动登录成功，用户 %s", c.username)
	return nil
}

// InitStatus 查询系统是否已初始化。
func (c *Client) InitStatus(ctx context.Context) (bool, error) {
	req, err := c.NewRequest(ctx, http.MethodGet, "/api/auth/init-status", nil, nil)
	if err != nil {
		return false, err
	}
	req.Header.Del("Authorization")

	data, err := c.Execute(req, c.httpClient)
	if err != nil {
		return false, err
	}
	var out struct {
		Initialized bool `json:"initialized"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return false, fmt.Errorf("waymark: 解析初始化状态失败: %w", err)
	}
	return out.Initialized, nil
}

// Init 初始化首个管理员账号。
func (c *Client) Init(ctx context.Context, username, password, nickname string) error {
	req, err := c.NewRequest(ctx, http.MethodPost, "/api/auth/init", nil, initRequest{
		Username: username,
		Password: password,
		Nickname: nickname,
	})
	if err != nil {
		return err
	}
	req.Header.Del("Authorization")

	_, err = c.Execute(req, c.httpClient)
	return err
}

// Login 使用客户端配置的账号密码登录并缓存令牌。
func (c *Client) Login(ctx context.Context) (*model.LoginResult, error) {
	if !c.CanRelogin() {
		return nil, apierr.ErrNoCredentials
	}
	return c.LoginWith(ctx, c.username, c.password)
}

// LoginWith 使用指定账号密码登录并缓存令牌。
func (c *Client) LoginWith(ctx context.Context, username, password string) (*model.LoginResult, error) {
	req, err := c.NewRequest(ctx, http.MethodPost, "/api/auth/login", nil, loginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, err
	}
	req.Header.Del("Authorization")

	data, err := c.Execute(req, c.httpClient)
	if err != nil {
		return nil, err
	}
	var out model.LoginResult
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("waymark: 解析登录结果失败: %w", err)
	}
	c.SetToken(out.Token)
	c.logger.Debugf("waymark: 登录成功，用户 %s", username)
	return &out, nil
}

// Profile 查询当前登录用户资料。
func (c *Client) Profile(ctx context.Context) (*model.UserProfile, error) {
	var out model.UserProfile
	if err := c.SendJSON(ctx, http.MethodGet, "/api/auth/profile", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
