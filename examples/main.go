// Command example 演示 Waymark Go 客户端 SDK 的常见用法。
//
// 运行前请先启动 Waymark 服务端，然后执行：
//
//	go run . -endpoint http://127.0.0.1:9868 -username admin -password admin
//
// 也可以通过环境变量 WAYMARK_ENDPOINT、WAYMARK_USERNAME、WAYMARK_PASSWORD、
// WAYMARK_NAMESPACE 进行配置。
//
// 示例执行完配置中心演示后，会注册实例并保持在线（后台定时心跳），
// 同时持续订阅配置与实例变更，收到事件后打印最新配置信息与在线实例列表，
// 并常驻运行，便于调试观察；按 Ctrl+C 退出，退出时会自动注销已注册的实例。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	waymark "github.com/wueasy/waymark-sdk-golang"
)

const (
	demoGroup     = waymark.DefaultGroup
	demoDataId    = "demo.yaml"
	demoAppDataId = "demo-app.yaml"
	demoService   = "waymark-demo-service"
	demoIp        = "127.0.0.1"
	demoPort      = 18080
	// demoHeartbeatInterval 实例心跳间隔，需小于服务端心跳超时时间（默认 15s）。
	demoHeartbeatInterval = 5 * time.Second
	// demoQuickService、demoQuickPort 快速初始化示例使用的自身实例，与上面的手动示例区分。
	demoQuickService = "waymark-demo-quick-service"
	demoQuickPort    = 18081
)

func main() {
	endpoint := flag.String("endpoint", env("WAYMARK_ENDPOINT", "http://127.0.0.1:9868"), "Waymark 服务端地址，多个用英文逗号分隔（故障转移）")
	username := flag.String("username", env("WAYMARK_USERNAME", "admin"), "登录用户名")
	password := flag.String("password", env("WAYMARK_PASSWORD", "123456"), "登录密码")
	namespace := flag.String("namespace", env("WAYMARK_NAMESPACE", waymark.DefaultNamespace), "命名空间")
	flag.Parse()

	client, err := waymark.NewClient(waymark.Config{
		Endpoint: *endpoint,
		Username: *username,
		Password: *password,
		Timeout:  10 * time.Second,
	})
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}
	// 无需手动登录：SDK 会在首次请求时使用配置的账号密码自动登录，
	// 并在令牌过期（401）时自动重新登录后重试。
	fmt.Printf("== 创建客户端成功: endpoint=%s namespace=%s ==\n", *endpoint, *namespace)

	// 常驻上下文：收到 Ctrl+C/SIGTERM 时取消。
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(runCtx, 30*time.Second)
	defer cancel()

	if err := demoConfig(ctx, client, *namespace); err != nil {
		log.Fatalf("配置中心示例失败: %v", err)
	}

	// 注册实例并保持在线（后台定时心跳），返回退出时使用的注销函数。
	deregister, err := demoRegistry(runCtx, client, *namespace)
	if err != nil {
		log.Fatalf("注册中心示例失败: %v", err)
	}

	if err := demoWatch(runCtx, client, *namespace); err != nil {
		log.Fatalf("订阅示例失败: %v", err)
	}

	// 快速初始化：配置监听 + 服务发现两个开箱即用组件（内置自身注册与心跳）。
	closeQuickStart, err := demoQuickStart(runCtx, client, *namespace)
	if err != nil {
		log.Fatalf("快速初始化示例失败: %v", err)
	}
	defer closeQuickStart()

	fmt.Println("\n示例持续运行中（实例定时心跳、配置变更订阅）。按 Ctrl+C 退出。")
	<-runCtx.Done()

	deregister()

	fmt.Println("\n== 示例退出 ==")
}

// demoQuickStart 演示快速初始化组件：
// ConfigWatcher 一次初始化即完成配置加载与变更订阅，只需提供回调；
// RegistryResolver 将在线实例缓存到内存，内置自身注册与心跳，并可按服务名返回存活链接。
func demoQuickStart(runCtx context.Context, client *waymark.Client, namespace string) (func(), error) {
	fmt.Println("\n-- 快速初始化 --")

	// 配置监听：初始化时回调一次当前配置，之后变更自动回调最新配置。
	watcher, err := client.NewConfigWatcher(runCtx, waymark.ConfigWatcherOptions{
		Namespace: namespace,
		GroupName: demoGroup,
		DataIds:   []string{demoDataId, demoAppDataId},
		OnChange: func(item *waymark.ConfigItem) {
			fmt.Printf("[快速初始化] 配置回调: dataId=%s type=%s md5=%s\n", item.DataId, item.Type, item.Md5)
		},
		OnError: func(err error) { log.Printf("[快速初始化] 配置监听错误: %v", err) },
	})
	if err != nil {
		return nil, fmt.Errorf("创建配置监听器: %w", err)
	}

	// 服务发现：在线实例缓存到内存，自动订阅变更，并内置自身实例注册与心跳。
	resolver, err := client.NewRegistryResolver(runCtx, waymark.RegistryResolverOptions{
		Namespace: namespace,
		GroupName: demoGroup,
		Self: &waymark.InstanceRequest{
			Namespace:   namespace,
			GroupName:   demoGroup,
			ServiceName: demoQuickService,
			Ip:          demoIp,
			Port:        demoQuickPort,
			Weight:      1,
			Metadata:    map[string]string{"version": "1.0.0"},
		},
		OnError: func(err error) { log.Printf("[快速初始化] 服务发现错误: %v", err) },
	})
	if err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("创建服务发现解析器: %w", err)
	}

	// 等待自身实例注册生效。
	time.Sleep(time.Second)

	fmt.Printf("[快速初始化] 服务 %s 在线实例 %d 个\n", demoQuickService, len(resolver.Instances(demoQuickService)))
	// 按服务名选择在线实例（平滑加权轮询），返回访问链接与计算后的权重。
	for i := 0; i < 3; i++ {
		picked, err := resolver.Pick(demoQuickService)
		if err != nil {
			log.Printf("[快速初始化] 选择实例失败: %v", err)
			break
		}
		fmt.Printf("[快速初始化] 选择实例: url=%s currentWeight=%.2f\n", picked.URL, picked.CurrentWeight)
	}

	return func() {
		if err := resolver.Close(); err != nil {
			log.Printf("关闭服务发现解析器失败: %v", err)
		}
		if err := watcher.Close(); err != nil {
			log.Printf("关闭配置监听器失败: %v", err)
		}
	}, nil
}

// demoConfig 演示配置中心：读取、列表、历史与导出（只读取，不发布配置）。
func demoConfig(ctx context.Context, client *waymark.Client, namespace string) error {
	fmt.Println("\n-- 配置中心 --")

	item, err := client.GetConfig(ctx, namespace, demoGroup, demoDataId)
	if err != nil {
		log.Printf("读取配置失败（可先在配置中心创建 %s/%s）: %v", demoGroup, demoDataId, err)
	} else {
		fmt.Printf("读取配置成功: type=%s md5=%s\n%s", item.Type, item.Md5, item.Content)
	}

	page, err := client.ListConfigs(ctx, waymark.ListConfigsOptions{
		Namespace: namespace,
		GroupName: demoGroup,
		PageNum:   1,
		PageSize:  10,
	})
	if err != nil {
		return fmt.Errorf("查询配置列表: %w", err)
	}
	fmt.Printf("查询配置列表成功: 共 %d 条，本页 %d 条\n", page.Total, len(page.List))

	histories, err := client.ConfigHistory(ctx, namespace, demoGroup, demoDataId)
	if err != nil {
		log.Printf("查询配置历史失败: %v", err)
	} else {
		fmt.Printf("查询配置历史成功: 共 %d 个版本\n", len(histories))
	}

	data, err := client.ExportConfigs(ctx, waymark.ExportOptions{
		Namespace: namespace,
		Items:     []waymark.ExportItem{{GroupName: demoGroup, DataId: demoDataId}},
	})
	if err != nil {
		log.Printf("导出配置失败: %v", err)
	} else {
		fmt.Printf("导出配置成功: %d 字节\n", len(data))
	}

	return nil
}

// demoRegistry 演示注册中心：注册实例并保持在线（后台定时心跳），
// 返回的注销函数用于退出时清理实例。
func demoRegistry(runCtx context.Context, client *waymark.Client, namespace string) (func(), error) {
	fmt.Println("\n-- 注册中心 --")

	ctx, cancel := context.WithTimeout(runCtx, 10*time.Second)
	defer cancel()

	result, err := client.RegisterInstance(ctx, waymark.InstanceRequest{
		Namespace:   namespace,
		GroupName:   demoGroup,
		ServiceName: demoService,
		ClusterName: waymark.DefaultCluster,
		Ip:          demoIp,
		Port:        demoPort,
		Weight:      1,
		Metadata:    map[string]string{"version": "1.0.0"},
	})
	if err != nil {
		return nil, fmt.Errorf("注册实例: %w", err)
	}
	fmt.Printf("注册实例成功: created=%v id=%d（临时实例，服务端默认 15s 心跳超时）\n", result.Created, result.Instance.Id)

	if err := client.Beat(ctx, namespace, demoGroup, demoService, demoIp, demoPort); err != nil {
		return nil, fmt.Errorf("发送心跳: %w", err)
	}
	fmt.Println("发送心跳成功")

	instances, err := client.ListInstances(ctx, namespace, demoGroup, demoService)
	if err != nil {
		return nil, fmt.Errorf("查询实例列表: %w", err)
	}
	fmt.Printf("查询实例列表成功: 共 %d 个实例\n", len(instances))

	services, err := client.ListServices(ctx, namespace, demoGroup)
	if err != nil {
		return nil, fmt.Errorf("查询服务列表: %w", err)
	}
	fmt.Printf("查询服务列表成功: 共 %d 个服务\n", len(services))

	// 后台定时心跳，保持实例在线（实例未注销前一直续约）。
	go heartbeatLoop(runCtx, client, namespace, demoService, demoIp, demoPort)

	deregister := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.DeregisterInstance(ctx, namespace, demoGroup, demoService, demoIp, demoPort); err != nil {
			log.Printf("注销实例失败: %v", err)
			return
		}
		fmt.Println("注销实例成功")
	}
	return deregister, nil
}

// heartbeatLoop 按固定间隔发送实例心跳，直到 ctx 结束。
func heartbeatLoop(ctx context.Context, client *waymark.Client, namespace, service, ip string, port int) {
	ticker := time.NewTicker(demoHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			beatCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := client.Beat(beatCtx, namespace, demoGroup, service, ip, port)
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("发送心跳失败: %v", err)
			}
		}
	}
}

// demoWatch 演示变更订阅：持续订阅配置与实例变更，
// 收到事件后拉取并打印订阅的配置信息与在线实例列表，直到 ctx 结束（Ctrl+C 退出）。
// 只订阅、不发布配置；在配置中心修改配置或上下线实例即可看到回调。
func demoWatch(ctx context.Context, client *waymark.Client, namespace string) error {
	fmt.Println("\n-- 变更订阅 --")

	// 单条订阅连接同时订阅多个配置文件与实例变更，按事件类型区分处理。
	go func() {
		err := client.Watch(ctx, waymark.SubscribeOptions{
			Namespace:   namespace,
			GroupName:   demoGroup,
			DataIds:     []string{demoDataId, demoAppDataId},
			ServiceName: demoService,
			Handler: func(ev waymark.Event) {
				switch ev.EventType {
				case waymark.EventTypeConfig:
					fmt.Printf("收到配置变更事件: key=%s md5=%s\n", ev.WatchKey, ev.Md5)
					logConfigs(ctx, client, namespace)
				case waymark.EventTypeInstance:
					fmt.Printf("收到实例变更事件: key=%s\n", ev.WatchKey)
					logInstances(ctx, client, namespace)
				}
			},
		})
		if err != nil && ctx.Err() == nil {
			log.Printf("订阅变更异常退出: %v", err)
		}
	}()

	// 等待订阅连接建立。
	time.Sleep(500 * time.Millisecond)

	// 先打印一次订阅范围内的当前数据，便于确认订阅内容。
	logConfigs(ctx, client, namespace)
	logInstances(ctx, client, namespace)

	return nil
}

// logConfigs 拉取并打印订阅分组下的配置信息。
func logConfigs(ctx context.Context, client *waymark.Client, namespace string) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	page, err := client.ListConfigs(callCtx, waymark.ListConfigsOptions{
		Namespace: namespace,
		GroupName: demoGroup,
		PageNum:   1,
		PageSize:  50,
	})
	if err != nil {
		log.Printf("拉取配置列表失败: %v", err)
		return
	}
	fmt.Printf("订阅配置 [%s/%s] 共 %d 条:\n", namespace, demoGroup, page.Total)
	for _, item := range page.List {
		fmt.Printf("  - dataId=%s type=%s md5=%s\n", item.DataId, item.Type, item.Md5)
		if item.DataId == demoDataId {
			fmt.Printf("    内容:\n%s", indent(item.Content, "    "))
		}
	}
}

// logInstances 拉取并打印订阅服务下的在线实例列表。
func logInstances(ctx context.Context, client *waymark.Client, namespace string) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	instances, err := client.ListInstances(callCtx, namespace, demoGroup, demoService)
	if err != nil {
		log.Printf("拉取实例列表失败: %v", err)
		return
	}
	online := 0
	for _, inst := range instances {
		if inst.Healthy == 1 {
			online++
		}
	}
	fmt.Printf("订阅服务 [%s/%s/%s] 实例 %d 个，在线 %d 个:\n", namespace, demoGroup, demoService, len(instances), online)
	for _, inst := range instances {
		status := "不健康"
		if inst.Healthy == 1 {
			status = "在线"
		}
		fmt.Printf("  - %s:%d %s weight=%.1f cluster=%s meta=%v\n", inst.Ip, inst.Port, status, inst.Weight, inst.ClusterName, inst.Metadata)
	}
}

// indent 为多行文本的每一行添加前缀，便于对齐输出。
func indent(text, prefix string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n") + "\n"
}

// env 读取环境变量，为空时返回默认值。
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
