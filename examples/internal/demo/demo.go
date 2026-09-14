// Package demo 提供示例程序共用的命令行参数解析、客户端创建与输出辅助，
// 让各启动入口（cmd/*）专注于演示快速初始化组件的用法。
package demo

import (
	"flag"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	waymark "github.com/wueasy/waymark-sdk-golang"
)

// Options 示例共用的服务端连接参数。
type Options struct {
	// Endpoint 服务端地址。
	Endpoint string
	// Username 登录用户名。
	Username string
	// Password 登录密码。
	Password string
	// Namespace 命名空间。
	Namespace string
	// Log 是否输出 SDK 调试日志。
	Log bool
}

// Bind 注册共用命令行参数（-endpoint/-username/-password/-namespace/-log），
// 未显式指定时回退到环境变量与默认值。调用后需由入口执行 flag.Parse。
func Bind() *Options {
	o := &Options{}
	flag.StringVar(&o.Endpoint, "endpoint", Env("WAYMARK_ENDPOINT", "http://127.0.0.1:9868"), "Waymark 服务端地址，多个用英文逗号分隔（故障转移）")
	flag.StringVar(&o.Username, "username", Env("WAYMARK_USERNAME", "admin"), "登录用户名")
	flag.StringVar(&o.Password, "password", Env("WAYMARK_PASSWORD", "123456"), "登录密码")
	flag.StringVar(&o.Namespace, "namespace", Env("WAYMARK_NAMESPACE", waymark.DefaultNamespace), "命名空间")
	flag.BoolVar(&o.Log, "log", true, "是否输出 SDK 调试日志")
	return o
}

// NewClient 按参数创建客户端。SDK 会在首次请求时自动登录，并在令牌过期时自动重新登录。
// 开启 -log 时传入 zap 开发模式 logger，SDK 内部会输出请求、订阅、调度等调试日志。
func (o *Options) NewClient() (*waymark.Client, error) {
	var logger *zap.SugaredLogger
	if o.Log {
		l, err := zap.NewDevelopment()
		if err != nil {
			return nil, err
		}
		logger = l.Sugar()
	}
	return waymark.NewClient(waymark.Config{
		Endpoint: o.Endpoint,
		Username: o.Username,
		Password: o.Password,
		Timeout:  10 * time.Second,
		Logger:   logger,
	})
}

// Env 读取环境变量，为空时返回默认值。
func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// SplitList 按逗号拆分字符串，去除每项的空白并忽略空项。
func SplitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Indent 为多行文本的每一行添加前缀，便于对齐输出。
func Indent(text, prefix string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n") + "\n"
}
