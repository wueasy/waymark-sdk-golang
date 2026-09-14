// Command discovery 演示用「快速初始化」方式做服务发现：
// RegistryResolver 会把在线实例缓存到内存、自动订阅实例变更，
// 并内置自身实例的注册与心跳（默认每 5s 一次，退出时自动注销）。
// 调用方只需按服务名取值，即可拿到存活链接与计算好的权重。
//
// 运行：
//
//	go run ./cmd/discovery
//
// 程序常驻运行并每 2 秒按服务名选一次实例（平滑加权轮询），按 Ctrl+C 退出。
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
	group := flag.String("group", demo.Env("WAYMARK_GROUP", waymark.DefaultGroup), "服务分组")
	service := flag.String("service", "waymark-demo-service", "要发现/注册的服务名")
	ip := flag.String("ip", demo.Env("WAYMARK_SELF_IP", "127.0.0.1"), "自身实例 IP")
	port := flag.Int("port", 18081, "自身实例端口")
	weight := flag.Float64("weight", 1, "自身实例权重")
	register := flag.Bool("register", true, "是否注册自身实例并保持心跳")
	flag.Parse()

	client, err := opts.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}
	fmt.Printf("== 创建客户端成功: endpoint=%s namespace=%s ==\n", opts.Endpoint, opts.Namespace)

	// 常驻上下文：收到 Ctrl+C/SIGTERM 时取消。
	runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	resolverOpts := waymark.RegistryResolverOptions{
		Namespace: opts.Namespace,
		GroupName: *group,
		OnError:   func(err error) { log.Printf("[错误] 服务发现: %v", err) },
	}
	if *register {
		// 内置自身注册与心跳，无需手动调用 RegisterInstance/Beat。
		resolverOpts.Self = &waymark.InstanceRequest{
			Namespace:   opts.Namespace,
			GroupName:   *group,
			ServiceName: *service,
			ClusterName: waymark.DefaultCluster,
			Ip:          *ip,
			Port:        *port,
			Weight:      *weight,
			Metadata:    map[string]string{"version": "1.0.0"},
		}
	}

	resolver, err := client.NewRegistryResolver(runCtx, resolverOpts)
	if err != nil {
		log.Fatalf("创建服务发现解析器失败: %v", err)
	}
	// Close 会注销自身实例（若注册过）。
	defer func() {
		if err := resolver.Close(); err != nil {
			log.Printf("关闭服务发现解析器失败: %v", err)
		}
	}()

	// 等待自身实例注册生效。
	time.Sleep(time.Second)
	logInstances(resolver, *service)

	// 每 2 秒按服务名选一次实例，观察平滑加权轮询的权重变化。
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	fmt.Println("\n服务发现运行中，每 2 秒选择一次实例。按 Ctrl+C 退出。")
	for {
		select {
		case <-runCtx.Done():
			fmt.Println("\n== 退出 ==")
			return
		case <-ticker.C:
			logInstances(resolver, *service)
			picked, err := resolver.Pick(*service)
			if err != nil {
				log.Printf("[错误] 选择实例: %v", err)
				continue
			}
			fmt.Printf("[选择] service=%s url=%s currentWeight=%.2f\n", *service, picked.URL, picked.CurrentWeight)
		}
	}
}

// logInstances 打印解析器内存中缓存的在线实例。
func logInstances(resolver *waymark.RegistryResolver, service string) {
	instances := resolver.Instances(service)
	fmt.Printf("[缓存] 服务 %s 在线实例 %d 个:\n", service, len(instances))
	for _, inst := range instances {
		fmt.Printf("  - %s:%d weight=%.1f cluster=%s meta=%v\n", inst.Ip, inst.Port, inst.Weight, inst.ClusterName, inst.Metadata)
	}
}
