package waymark

import "context"

// InitStatus 查询系统是否已初始化。
func (c *Client) InitStatus(ctx context.Context) (bool, error) {
	return c.core.InitStatus(ctx)
}

// Init 初始化首个管理员账号。
func (c *Client) Init(ctx context.Context, username, password, nickname string) error {
	return c.core.Init(ctx, username, password, nickname)
}

// Login 使用客户端配置的账号密码登录并缓存令牌。
func (c *Client) Login(ctx context.Context) (*LoginResult, error) {
	return c.core.Login(ctx)
}

// LoginWith 使用指定账号密码登录并缓存令牌。
func (c *Client) LoginWith(ctx context.Context, username, password string) (*LoginResult, error) {
	return c.core.LoginWith(ctx, username, password)
}

// Profile 查询当前登录用户资料。
func (c *Client) Profile(ctx context.Context) (*UserProfile, error) {
	return c.core.Profile(ctx)
}
