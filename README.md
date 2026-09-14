# Waymark Go SDK

> Waymark 注册中心 / 配置中心的 Go 客户端。一个客户端即可完成配置管理、服务注册与发现、SSE 变更订阅。

- 模块路径：`github.com/wueasy/waymark-sdk-golang`
- 要求：Go 1.26+
- 依赖：`go.uber.org/zap`（可选使用的调试日志）

## 特性

- **配置中心**：发布、查询、删除、历史版本、回滚、导出、导入。
- **注册中心**：实例注册、更新、心跳、注销、实例列表与服务列表查询。
- **变更订阅**：基于 SSE 长连接，一条连接可同时订阅多个配置与实例变更，断线自动重连。
- **快速初始化**：`ConfigWatcher`（配置监听 + 内存快照）、`RegistryResolver`（实例缓存 + 平滑加权轮询 + 自动注册心跳）。
- **自动登录**：令牌缺失或失效时，使用配置的账号密码自动登录并重试一次。
- **故障转移**：`Endpoint` 支持英文逗号分隔多地址，仅网络层错误触发切换，业务错误不切换。
- **缓存降级**：配置中心不可达时 `GetConfig` 自动回退本地缓存。

## 安装

```bash
go get github.com/wueasy/waymark-sdk-golang
```

## 快速开始

```go
package main

import (
	"context"
	"fmt"
	"time"

	waymark "github.com/wueasy/waymark-sdk-golang"
)

func main() {
	ctx := context.Background()

	client, err := waymark.NewClient(waymark.Config{
		Endpoint: "http://127.0.0.1:9868",
		Username: "admin",
		Password: "123456",
		Timeout:  10 * time.Second,
	})
	if err != nil {
		panic(err)
	}

	// 发布配置
	err = client.PublishConfig(ctx, waymark.PublishConfigRequest{
		Namespace: waymark.DefaultNamespace,
		GroupName: waymark.DefaultGroup,
		DataId:    "app.yaml",
		Content:   "server:\n  port: 8080\n",
		Type:      waymark.ConfigTypeYAML,
	})
	if err != nil {
		panic(err)
	}

	// 读取配置
	item, err := client.GetConfig(ctx, waymark.DefaultNamespace, waymark.DefaultGroup, "app.yaml")
	if err != nil {
		panic(err)
	}
	fmt.Println(item.Md5, item.Content)

	// 注册实例并发送一次心跳
	_, err = client.RegisterInstance(ctx, waymark.InstanceRequest{
		Namespace:   waymark.DefaultNamespace,
		GroupName:   waymark.DefaultGroup,
		ServiceName: "order-service",
		ClusterName: waymark.DefaultCluster,
		Ip:          "127.0.0.1",
		Port:        8080,
		Metadata:    map[string]string{"version": "1.0.0"},
	})
	if err != nil {
		panic(err)
	}

	// 订阅变更（阻塞，直到 ctx 结束）
	_ = client.Watch(ctx, waymark.SubscribeOptions{
		Namespace:   waymark.DefaultNamespace,
		GroupName:   waymark.DefaultGroup,
		DataIds:     []string{"app.yaml"},
		ServiceName: "order-service",
		Handler: func(ev waymark.Event) {
			fmt.Printf("变更: type=%s key=%s md5=%s\n", ev.EventType, ev.WatchKey, ev.Md5)
		},
	})
}
```

## 客户端配置（Config）

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Endpoint` | `string` | 必填 | 服务端地址，如 `http://127.0.0.1:9868`；多个地址用英文逗号分隔，按顺序故障转移 |
| `Username` | `string` | 空 | 登录用户名，配置后令牌缺失/过期时自动登录 |
| `Password` | `string` | 空 | 登录密码 |
| `Token` | `string` | 空 | 已有的访问令牌，非空时直接使用 |
| `Timeout` | `time.Duration` | `10s` | 单次请求超时；订阅长连接不受此限制 |
| `HTTPClient` | `*http.Client` | `nil` | 自定义 HTTP 客户端，为空时按 `Timeout` 创建 |
| `CacheDir` | `string` | 系统用户缓存目录下的 `waymark` | 本地配置缓存目录 |
| `DisableCache` | `bool` | `false` | 是否关闭本地配置缓存 |
| `Logger` | `*zap.SugaredLogger` | `zap.NewNop().Sugar()` | 调试日志，始终非 nil |

```go
client, err := waymark.NewClient(waymark.Config{
	Endpoint: "http://10.0.0.1:9868,http://10.0.0.2:9868", // 多地址故障转移
	Username: "admin",
	Password: "123456",
	Logger:   zap.NewDevelopment().Sugar(),
})
```

`Client` 并发安全，可长期复用。也可通过 `SetToken` / `Token` 读写当前令牌。

## 认证

| 方法 | 说明 |
|------|------|
| `InitStatus(ctx) (bool, error)` | 查询系统是否已初始化（`user` 表是否为空） |
| `Init(ctx, username, password, nickname string) error` | 初始化首个管理员账号，仅在未初始化时可用 |
| `Login(ctx) (*LoginResult, error)` | 使用配置中的账号密码登录并缓存令牌 |
| `LoginWith(ctx, username, password string) (*LoginResult, error)` | 使用指定账号密码登录并缓存令牌 |
| `Profile(ctx) (*UserProfile, error)` | 查询当前登录用户资料（含角色与命名空间权限） |

正常使用时**无需手动登录**：首次请求会自动登录，收到 401 时自动重新登录并重试一次。若未配置账号密码且无令牌，返回 `ErrNoCredentials`。

## 配置中心

| 方法 | 说明 |
|------|------|
| `ListConfigs(ctx, ListConfigsOptions) (*ConfigPage, error)` | 分页查询配置列表，`PageNum` 从 1 开始 |
| `GetConfig(ctx, namespace, group, dataId string) (*ConfigItem, error)` | 查询配置详情，成功写入本地缓存；网络不可达时回退缓存 |
| `PublishConfig(ctx, PublishConfigRequest) error` | 发布或更新配置，`Type` 为空按 `text` 处理 |
| `DeleteConfig(ctx, namespace, group, dataId string) error` | 删除配置 |
| `ConfigHistory(ctx, namespace, group, dataId string) ([]ConfigHistory, error)` | 查询全部历史版本（内部按 200/页循环拉取合并） |
| `RestoreConfig(ctx, namespace, group, dataId string, historyId int64) error` | 将指定历史版本还原为当前配置 |
| `ExportConfigs(ctx, ExportOptions) ([]byte, error)` | 导出配置为 zip 字节流 |
| `ImportConfigs(ctx, namespace, group string, zipData []byte) (*ImportResult, error)` | 从 zip 字节流导入配置 |

```go
// 精确导出指定的配置项；Items 为空则按 namespace/group/dataId 过滤导出
zipData, err := client.ExportConfigs(ctx, waymark.ExportOptions{
	Namespace: waymark.DefaultNamespace,
	Items: []waymark.ExportItem{
		{GroupName: waymark.DefaultGroup, DataId: "app.yaml"},
	},
})

// 导入时 group 非空表示全部导入到该分组，为空则沿用压缩包内原分组
result, err := client.ImportConfigs(ctx, waymark.DefaultNamespace, "", zipData)
fmt.Println(result.Imported, result.Failed)
```

## 注册中心

| 方法 | 说明 |
|------|------|
| `RegisterInstance(ctx, InstanceRequest) (*RegisterInstanceResult, error)` | 注册实例，已存在则更新并返回 `Created=false` |
| `UpdateInstance(ctx, InstanceRequest) error` | 更新实例属性 |
| `DeregisterInstance(ctx, namespace, group, service, ip string, port int) error` | 注销实例 |
| `Beat(ctx, namespace, group, service, ip string, port int) error` | 发送实例心跳 |
| `ListInstances(ctx, namespace, group, service string) ([]Instance, error)` | 查询实例列表，`group`/`service` 为空表示不过滤 |
| `ListServices(ctx, namespace, group string) ([]ServiceSummary, error)` | 查询服务概览列表（内部按 200/页循环拉取合并） |

`InstanceRequest` 中 `Weight <= 0` 按 1 处理，`Healthy` / `Ephemeral` 为 `*int`，传 `nil` 时由服务端按默认值（1）处理。

> 临时实例（`Ephemeral = 1`）需持续心跳，超过服务端心跳超时（默认 15s）会被淘汰；心跳间隔需明显小于该超时（示例取 5s）。生产环境建议直接使用 `RegistryResolver` 的 `Self` 自动保活。

## 变更订阅（SSE）

```go
func (c *Client) Watch(ctx context.Context, opts SubscribeOptions) error
```

`Watch` **阻塞**直到 `ctx` 结束；连接异常断开时按 `ReconnectDelay`（默认 3s）自动重连。

`SubscribeOptions`：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Namespace` | `string` | 空表示默认命名空间 `public` |
| `GroupName` | `string` | 空表示默认分组 `DEFAULT_GROUP` |
| `DataIds` | `[]string` | 要订阅的配置；`"*"` 表示该分组全部配置；为空表示不订阅配置 |
| `ServiceName` | `string` | 要订阅的服务名；为空表示该分组下全部服务 |
| `Handler` | `func(Event)` | 事件回调，在接收协程中**同步**执行，勿做长阻塞 |
| `ReconnectDelay` | `time.Duration` | 重连间隔，默认 3s |

事件只携带定位信息，不含内容，收到后需自行拉取最新数据：

```
event: change
data: {"eventType":"CONFIG","namespace":"public","group":"DEFAULT_GROUP","watchKey":"app.yaml","md5":"a1b2c3..."}
```

- `EventType`：`CONFIG` 或 `INSTANCE`；`WatchKey` 在配置变更时为 `dataId`，实例变更时为服务名。
- 认证使用 `Authorization: Bearer <token>` 请求头（非 URL 传参）。
- 认证失败时清理令牌以便重连时重新登录，随后重试。

## 快速初始化

### ConfigWatcher：配置监听

创建时先为每个 `dataId` 同步加载一次并回调，随后后台订阅变更；配置更新后自动拉取最新内容并回调。

```go
watcher, err := client.NewConfigWatcher(ctx, waymark.ConfigWatcherOptions{
	Namespace: waymark.DefaultNamespace,
	GroupName: waymark.DefaultGroup,
	DataIds:   []string{"app.yaml", "common.yaml"},
	OnChange: func(item *waymark.ConfigItem) {
		fmt.Println("配置更新:", item.DataId, item.Content)
	},
	OnError: func(err error) { fmt.Println("监听异常:", err) },
})
if err != nil {
	panic(err)
}
defer watcher.Close()

// 读取内存中的最新快照（未命中返回 false）
if item, ok := watcher.Get("app.yaml"); ok {
	fmt.Println(item.Content)
}
```

`ConfigWatcherOptions`：`Namespace` / `GroupName`（空取默认）、`DataIds`（至少一个）、`OnChange`（必填）、`OnError`（可选）、`ReconnectDelay`（默认 3s）。

### RegistryResolver：服务发现

将在线实例缓存到内存，提供按服务名的**平滑加权轮询（SWRR）**选择；可选自动注册自身实例并保持心跳。

```go
resolver, err := client.NewRegistryResolver(ctx, waymark.RegistryResolverOptions{
	Namespace: waymark.DefaultNamespace,
	GroupName: waymark.DefaultGroup,
	Self: &waymark.InstanceRequest{ // 非空则自动注册 + 心跳，Close 时自动注销
		ServiceName: "order-service",
		Ip:          "127.0.0.1",
		Port:        8080,
		Weight:      1,
		Metadata:    map[string]string{"version": "1.0.0"},
	},
	OnError: func(err error) { fmt.Println("发现异常:", err) },
})
if err != nil {
	panic(err)
}
defer resolver.Close()

picked, err := resolver.Pick("order-service") // 平滑加权轮询选出一个在线实例
if err == nil {
	fmt.Println(picked.URL, picked.CurrentWeight)
}

url, err := resolver.PickURL("order-service")   // 等价于 picked.URL
instances := resolver.Instances("order-service") // 在线实例快照
```

`RegistryResolverOptions`：`Namespace` / `GroupName`（空取默认）、`Self`（可选，自动注册并心跳）、`HeartbeatInterval`（默认 5s）、`RefreshInterval`（默认 30s，内存实例全量刷新兜底）、`OnError`（可选）、`ReconnectDelay`（默认 3s）。

行为要点：

- 仅缓存 `Healthy == 1` 的实例；权重 `<= 0` 按 1 计算。
- 心跳失败时自动重新注册，无需自行处理实例过期。
- 同 namespace + group 的 `ConfigWatcher` 与 `RegistryResolver` **复用同一条 SSE 连接**，不会产生多余连接。
- `Close()` 幂等：停止订阅与刷新、等待后台协程退出，并在注册过自身实例时执行注销。

## 错误处理

```go
item, err := client.GetConfig(ctx, ns, group, dataId)
switch {
case err == nil:
	// ok
case waymark.IsUnauthorized(err):
	// 未认证
case waymark.IsForbidden(err):
	// 无权限
default:
	// 其它错误
}
```

| 类型 / 函数 | 说明 |
|-------------|------|
| `*APIError` | 服务端业务错误，包含 `Code`、`Msg`、`HTTPStatus` |
| `ErrNoCredentials` | 未配置账号密码且无令牌，无法登录 |
| `IsUnauthorized(err) bool` | 是否未认证（`Code == 401` 或 HTTP 401） |
| `IsForbidden(err) bool` | 是否无权限（`Code == 403` 或 HTTP 403） |

错误码常量：`CodeFail = 1001`、`CodeUnauthorized = 401`、`CodeForbidden = 403`。

## 常量与默认值

```go
waymark.DefaultNamespace // "public"
waymark.DefaultGroup     // "DEFAULT_GROUP"
waymark.DefaultCluster   // "DEFAULT"

waymark.ConfigTypeText / ConfigTypeJSON / ConfigTypeYAML / ConfigTypeProperties / ConfigTypeXML
waymark.EventTypeConfig   // "CONFIG"
waymark.EventTypeInstance // "INSTANCE"
```

| 项 | 默认值 |
|----|--------|
| 请求超时 | 10s |
| 后台任务单次调用超时 | 5s |
| 订阅重连间隔 | 3s |
| 自身实例心跳间隔 | 5s |
| 实例全量刷新间隔 | 30s |
| 历史/服务分页大小 | 200 |

## 注意事项

1. **资源释放**：`ConfigWatcher`、`RegistryResolver` 必须调用 `Close()`（示例均 `defer`）；`Watch` 依赖 `ctx` 取消退出，需自行管理协程。
2. **事件需二次拉取**：`Event` 仅含定位信息，回调中需自行 `GetConfig` / `ListInstances` 获取最新数据（`ConfigWatcher` 已内置该逻辑）。
3. **回调勿阻塞**：`Handler` 在接收协程中同步执行，长时间阻塞会影响后续事件处理。
4. **缓存降级范围**：仅 `GetConfig` 会回退本地缓存，且仅在网络不可达时触发；业务错误不回退。缓存写入为尽力而为，失败静默忽略。
5. **多地址故障转移**：仅网络层错误触发地址切换，业务错误不切换。
6. **心跳约束**：自身心跳间隔需明显小于服务端 `registry.heartbeat-timeout`（默认 15s）。

## 示例

`examples/` 为独立 module（通过 `replace` 引用本地 SDK），在 `examples/` 目录下运行：

```bash
cd examples
go run ./cmd/config      # 配置监听（ConfigWatcher）
go run ./cmd/discovery   # 服务发现（RegistryResolver）
go run ./cmd/all         # 配置监听 + 服务发现的组合骨架
go run .                 # 完整流程演示
```

参数与环境变量：`-endpoint` / `WAYMARK_ENDPOINT`（默认 `http://127.0.0.1:9868`）、`-username` / `WAYMARK_USERNAME`（默认 `admin`）、`-password` / `WAYMARK_PASSWORD`（默认 `123456`）、`-namespace` / `WAYMARK_NAMESPACE`（默认 `public`）。
