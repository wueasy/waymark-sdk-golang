// Package quickstart 提供开箱即用的快速初始化组件：
// 配置监听（ConfigWatcher）与服务发现解析（RegistryResolver）。
package quickstart

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wueasy/waymark-sdk-golang/internal/apierr"
	"github.com/wueasy/waymark-sdk-golang/internal/cache"
	"github.com/wueasy/waymark-sdk-golang/internal/config"
	"github.com/wueasy/waymark-sdk-golang/internal/model"
	"github.com/wueasy/waymark-sdk-golang/internal/transport"
)

// ConfigWatcherOptions 配置监听初始化参数。
type ConfigWatcherOptions struct {
	// Namespace 命名空间，为空时使用默认命名空间。
	Namespace string
	// GroupName 分组，为空时使用默认分组。
	GroupName string
	// DataIds 监听的配置文件名列表，至少指定一个。
	DataIds []string
	// OnChange 配置回调：初始化时每个 dataId 回调一次当前配置，之后配置变更时回调最新配置。
	OnChange func(*model.ConfigItem)
	// OnError 错误回调，可选；用于上报初始化之后的监听与拉取错误。
	OnError func(error)
	// ReconnectDelay 订阅断线重连间隔，默认 3s。
	ReconnectDelay time.Duration
}

// ConfigWatcher 配置监听器：创建时加载一次当前配置并回调，随后在后台订阅变更，
// 配置更新时自动拉取最新内容并回调。使用 Close 释放。
type ConfigWatcher struct {
	client    *transport.Client
	namespace string
	group     string
	dataIds   []string
	onChange  func(*model.ConfigItem)
	onError   func(error)

	mu      sync.RWMutex
	configs map[string]*model.ConfigItem

	cancel      context.CancelFunc
	unsubscribe func()
	closeOnce   sync.Once
}

// NewConfigWatcher 创建配置监听器：先为每个 dataId 加载一次当前配置并回调，
// 随后在后台订阅配置变更，变更时自动拉取最新配置并回调。
func NewConfigWatcher(c *transport.Client, ctx context.Context, opts ConfigWatcherOptions) (*ConfigWatcher, error) {
	if opts.OnChange == nil {
		return nil, fmt.Errorf("waymark: OnChange 回调不能为空")
	}
	dataIds := normalizeValues(opts.DataIds)
	if len(dataIds) == 0 {
		return nil, fmt.Errorf("waymark: 至少需要指定一个 dataId")
	}

	w := &ConfigWatcher{
		client:    c,
		namespace: cache.NormalizeKey(opts.Namespace, model.DefaultNamespace),
		group:     cache.NormalizeKey(opts.GroupName, model.DefaultGroup),
		dataIds:   dataIds,
		onChange:  opts.OnChange,
		onError:   opts.OnError,
		configs:   make(map[string]*model.ConfigItem, len(dataIds)),
	}

	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	c.Logger().Infof("waymark: 启动配置监听 namespace=%s group=%s dataIds=%v", w.namespace, w.group, dataIds)

	// 初始化：加载一次当前配置并回调，保证调用方在启动阶段即可拿到配置。
	for _, dataId := range dataIds {
		item, err := config.GetConfig(c, runCtx, w.namespace, w.group, dataId)
		if err != nil {
			// 配置尚未发布属正常情况：不报错、不回调，保持订阅等待后续发布通知。
			if apierr.IsNotFound(err) {
				c.Logger().Infof("waymark: 配置 %s 尚未发布，等待变更通知", dataId)
				continue
			}
			cancel()
			c.Logger().Errorf("waymark: 加载配置 %s 失败: %v", dataId, err)
			return nil, fmt.Errorf("waymark: 加载配置 %s 失败: %w", dataId, err)
		}
		w.store(dataId, item)
		c.Logger().Debugf("waymark: 配置 %s 已加载", dataId)
		w.onChange(item)
	}

	// 与同分组的实例订阅复用一条 SSE 连接，仅订阅这些 dataId 的配置变更。
	w.unsubscribe = subscribeShared(c, w.namespace, w.group, opts.ReconnectDelay, w.dataIds,
		func(ev model.Event) { w.handleEvent(runCtx, ev) },
		func(err error) {
			c.Logger().Errorf("waymark: 配置订阅异常退出: %v", err)
			reportError(w.onError, err)
		})

	return w, nil
}

// Get 返回最近一次加载到的配置，未命中时 ok 为 false。
func (w *ConfigWatcher) Get(dataId string) (model.ConfigItem, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	item, ok := w.configs[strings.TrimSpace(dataId)]
	if !ok {
		return model.ConfigItem{}, false
	}
	return *item, true
}

// Close 停止监听并释放底层订阅连接。
func (w *ConfigWatcher) Close() error {
	w.closeOnce.Do(func() {
		w.client.Logger().Infof("waymark: 关闭配置监听 namespace=%s group=%s", w.namespace, w.group)
		w.cancel()
		w.unsubscribe()
	})
	return nil
}

// handleEvent 处理配置变更事件：拉取最新配置并回调。
func (w *ConfigWatcher) handleEvent(ctx context.Context, ev model.Event) {
	if ev.EventType != model.EventTypeConfig {
		return
	}
	dataId := strings.TrimSpace(ev.WatchKey)
	if dataId == "" || !containsValue(w.dataIds, dataId) {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, transport.DefaultCallTimeout)
	defer cancel()
	item, err := config.GetConfig(w.client, callCtx, w.namespace, w.group, dataId)
	if err != nil {
		// 配置在事件到达后被删除时视为未发布：不报错，保持订阅。
		if apierr.IsNotFound(err) {
			w.client.Logger().Infof("waymark: 配置 %s 尚未发布，等待变更通知", dataId)
			return
		}
		if ctx.Err() == nil {
			w.client.Logger().Errorf("waymark: 拉取变更配置 %s 失败: %v", dataId, err)
			reportError(w.onError, fmt.Errorf("waymark: 拉取变更配置 %s 失败: %w", dataId, err))
		}
		return
	}
	w.client.Logger().Infof("waymark: 配置 %s 发生变更", dataId)
	w.store(dataId, item)
	w.onChange(item)
}

// store 缓存最新配置，并发安全。
func (w *ConfigWatcher) store(dataId string, item *model.ConfigItem) {
	if item == nil || dataId == "" {
		return
	}
	w.mu.Lock()
	w.configs[dataId] = item
	w.mu.Unlock()
}

// normalizeValues 去除空白、忽略空值并去重，保持原有顺序。
func normalizeValues(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

// containsValue 判断切片中是否存在指定值。
func containsValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// reportError 将错误上报到可选回调。
func reportError(onError func(error), err error) {
	if onError != nil && err != nil {
		onError(err)
	}
}
