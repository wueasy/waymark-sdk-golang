// Command all 演示把「快速初始化」两个组件组合使用：
// ConfigWatcher 负责配置加载与变更回调，RegistryResolver 负责服务发现与自身注册，
// 是业务服务接入 Waymark 的最小骨架。
//
// 运行：
//
//	go run ./cmd/all
//
// 程序常驻运行，按 Ctrl+C 退出并自动注销已注册实例。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"waymark-sdk-examples/internal/demo"

	waymark "github.com/wueasy/waymark-sdk-golang"
)

func main() {
	opts := demo.Bind()
	group := flag.String("group", demo.Env("WAYMARK_GROUP", waymark.DefaultGroup), "分组")
	dataIds := flag.String("dataIds", "demo.yaml,demo-app.yaml", "监听的配置文件名，逗号分隔")
	service := flag.String("service", "waymark-demo-quick-service", "自身服务名")
	ip := flag.String("ip", demo.Env("WAYMARK_SELF_IP", "127.0.0.1"), "自身实例 IP")
	port := flag.Int("port", 18081, "自身实例端口")
	weight := flag.Float64("weight", 1, "自身实例权重")
	flag.Parse()

	client, err := opts.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}
	fmt.Printf("== 创建客户端成功: endpoint=%s namespace=%s ==\n", opts.Endpoint, opts.Namespace)

	dataIdList := demo.SplitList(*dataIds)
	if len(dataIdList) == 0 {
		log.Fatal("dataIds 不能为空")
	}

	// 常驻上下文：收到 Ctrl+C/SIGTERM 时取消。
	runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 配置监听：一次初始化即完成加载 + 订阅 + 回调。
	watcher, err := client.NewConfigWatcher(runCtx, waymark.ConfigWatcherOptions{
		Namespace: opts.Namespace,
		GroupName: *group,
		DataIds:   dataIdList,
		OnChange: func(item *waymark.ConfigItem) {
			fmt.Printf("[配置回调] dataId=%s type=%s md5=%s\n", item.DataId, item.Type, item.Md5)
		},
		OnError: func(err error) { log.Printf("[错误] 配置监听: %v", err) },
	})
	if err != nil {
		log.Fatalf("创建配置监听器失败: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	// 服务发现：在线实例缓存到内存，内置自身注册与心跳。
	resolver, err := client.NewRegistryResolver(runCtx, waymark.RegistryResolverOptions{
		Namespace: opts.Namespace,
		GroupName: *group,
		Self: &waymark.InstanceRequest{
			Namespace:   opts.Namespace,
			GroupName:   *group,
			ServiceName: *service,
			ClusterName: waymark.DefaultCluster,
			Ip:          *ip,
			Port:        *port,
			Weight:      *weight,
			Metadata:    map[string]string{"version": "1.0.0"},
		},
		OnError: func(err error) { log.Printf("[错误] 服务发现: %v", err) },
	})
	if err != nil {
		log.Fatalf("创建服务发现解析器失败: %v", err)
	}
	defer func() {
		if err := resolver.Close(); err != nil {
			log.Printf("关闭服务发现解析器失败: %v", err)
		}
	}()

	// 等待自身实例注册生效。
	time.Sleep(time.Second)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	fmt.Println("\n快速初始化运行中（配置监听 + 服务发现）。按 Ctrl+C 退出。")
	for {
		select {
		case <-runCtx.Done():
			fmt.Println("\n== 退出 ==")
			return
		case <-ticker.C:
			instances := resolver.Instances(*service)
			fmt.Printf("[发现] 服务 %s 在线实例 %d 个\n", *service, len(instances))
			picked, err := resolver.Pick(*service)
			if err != nil {
				log.Printf("[错误] 选择实例: %v", err)
				continue
			}
			fmt.Printf("[选择] url=%s currentWeight=%.2f\n", picked.URL, picked.CurrentWeight)
		}
	}
}
