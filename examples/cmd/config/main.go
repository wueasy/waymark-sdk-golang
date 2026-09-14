// Command config 演示用「快速初始化」方式监听配置：
// 一次初始化即完成「加载当前配置 + 订阅变更 + 回调」，
// 只需提供 OnChange 回调，无需手动调用 GetConfig 与 Watch。
//
// 运行：
//
//	go run ./cmd/config
//
// 只订阅配置变更，不写入/发布配置；在配置中心修改配置即可看到回调。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"waymark-sdk-examples/internal/demo"

	waymark "github.com/wueasy/waymark-sdk-golang"
)

func main() {
	opts := demo.Bind()
	group := flag.String("group", demo.Env("WAYMARK_GROUP", waymark.DefaultGroup), "配置分组")
	dataIds := flag.String("dataIds", "demo.yaml,demo-app.yaml", "监听的配置文件名，逗号分隔")
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

	// 快速初始化：一次创建即完成加载 + 订阅，变更时自动回调最新配置。
	watcher, err := client.NewConfigWatcher(runCtx, waymark.ConfigWatcherOptions{
		Namespace: opts.Namespace,
		GroupName: *group,
		DataIds:   dataIdList,
		OnChange: func(item *waymark.ConfigItem) {
			fmt.Printf("\n[回调] 配置已就绪 dataId=%s type=%s md5=%s\n%s", item.DataId, item.Type, item.Md5, demo.Indent(item.Content, "    "))
		},
		OnError: func(err error) { log.Printf("[错误] 配置监听: %v", err) },
	})
	if err != nil {
		log.Fatalf("创建配置监听器失败: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	// 读取监听器内存中缓存的最新配置。
	for _, dataId := range dataIdList {
		if item, ok := watcher.Get(dataId); ok {
			fmt.Printf("[缓存] dataId=%s md5=%s\n", item.DataId, item.Md5)
		}
	}

	fmt.Println("\n配置监听运行中（修改配置即可看到回调）。按 Ctrl+C 退出。")
	<-runCtx.Done()
	fmt.Println("\n== 退出 ==")
}
