package quickstart

import (
	"context"
	"sync"
	"time"

	"github.com/wueasy/waymark-sdk-golang/internal/model"
	"github.com/wueasy/waymark-sdk-golang/internal/subscribe"
	"github.com/wueasy/waymark-sdk-golang/internal/transport"
)

// sharedWatchers 按客户端缓存共享订阅，键为命名空间+分组。
// 服务端一条连接同时订阅配置与实例变更，因此配置监听与实例订阅复用同一条 SSE 连接。
var (
	sharedMu       sync.Mutex
	sharedWatchers = make(map[*transport.Client]map[string]*sharedWatcher)
)

// sharedWatcher 一条按命名空间与分组复用的订阅连接，把事件分派给已注册的处理函数。
// 连接实际订阅的 dataId 为当前全部处理函数关注 dataId 的并集，并集变化时重建连接。
type sharedWatcher struct {
	client    *transport.Client
	namespace string
	group     string
	reconnect time.Duration

	mu       sync.Mutex
	handlers map[uint64]sharedHandler
	nextID   uint64
	// active 当前连接实际订阅的 dataId 并集；conn 为当前连接，未建立时为 nil。
	active []string
	conn   *sharedConn
}

// sharedHandler 共享订阅上的一个处理函数及其关注的 dataId。
type sharedHandler struct {
	// dataIds 关注的配置文件名，为空表示只关注实例变更。
	dataIds []string
	onEvent func(model.Event)
	onError func(error)
}

// sharedConn 一次订阅连接的生命周期句柄。
type sharedConn struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// subscribeShared 注册处理函数：复用（或按需创建）同命名空间与分组的订阅连接。
// dataIds 为该处理函数关注的配置文件名，为空表示只关注实例变更；连接实际订阅的 dataId
// 为当前全部处理函数 dataId 的并集。返回的注销函数用于释放；同组内最后一个处理函数注销时连接才会关闭。
func subscribeShared(c *transport.Client, namespace, group string, reconnectDelay time.Duration,
	dataIds []string, onEvent func(model.Event), onError func(error)) func() {
	key := sharedKey(namespace, group)

	sharedMu.Lock()
	byGroup := sharedWatchers[c]
	if byGroup == nil {
		byGroup = make(map[string]*sharedWatcher)
		sharedWatchers[c] = byGroup
	}
	sw := byGroup[key]
	if sw == nil {
		sw = &sharedWatcher{
			client:    c,
			namespace: namespace,
			group:     group,
			reconnect: reconnectDelay,
			handlers:  make(map[uint64]sharedHandler),
		}
		byGroup[key] = sw
	}
	id := sw.add(sharedHandler{dataIds: normalizeValues(dataIds), onEvent: onEvent, onError: onError})
	sharedMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() { unsubscribeShared(c, key, sw, id) })
	}
}

// add 注册处理函数；首个处理函数注册或 dataId 并集变化时建立（重建）连接。调用方需持有 sharedMu。
func (sw *sharedWatcher) add(handler sharedHandler) uint64 {
	sw.mu.Lock()
	sw.nextID++
	id := sw.nextID
	sw.handlers[id] = handler
	old := sw.rebuildLocked()
	sw.mu.Unlock()

	stopConn(old)
	return id
}

// rebuildLocked 按当前并集建立或重建连接；返回需要关闭等待的旧连接（并集未变化时为 nil）。
// 调用方需持有 sw.mu，且应在释放锁之后再关闭旧连接，避免与本连接的事件分派互等。
func (sw *sharedWatcher) rebuildLocked() *sharedConn {
	merged := sw.mergedDataIdsLocked()
	if sw.conn == nil {
		sw.active = merged
		sw.startLocked()
		return nil
	}
	if equalStrings(merged, sw.active) {
		return nil
	}
	old := sw.conn
	sw.active = merged
	sw.startLocked()
	return old
}

// mergedDataIdsLocked 当前全部处理函数关注 dataId 的并集，保持注册顺序。调用方需持有 sw.mu。
func (sw *sharedWatcher) mergedDataIdsLocked() []string {
	merged := make([]string, 0, len(sw.handlers))
	seen := make(map[string]struct{})
	for _, handler := range sw.handlers {
		for _, dataId := range handler.dataIds {
			if _, ok := seen[dataId]; ok {
				continue
			}
			seen[dataId] = struct{}{}
			merged = append(merged, dataId)
		}
	}
	return merged
}

// startLocked 按当前并集建立订阅连接。调用方需持有 sw.mu。
func (sw *sharedWatcher) startLocked() {
	ctx, cancel := context.WithCancel(context.Background())
	conn := &sharedConn{cancel: cancel, done: make(chan struct{})}
	sw.conn = conn
	active := append([]string(nil), sw.active...)
	go func() {
		defer close(conn.done)
		err := subscribe.Watch(sw.client, ctx, model.SubscribeOptions{
			Namespace:      sw.namespace,
			GroupName:      sw.group,
			DataIds:        active,
			ReconnectDelay: sw.reconnect,
			Handler:        sw.dispatch,
		})
		if err != nil && ctx.Err() == nil {
			sw.client.Logger().Errorf("waymark: 订阅异常退出: %v", err)
			sw.reportError(err)
		}
	}()
}

// stopConn 取消并等待连接退出；conn 为 nil 时不做处理。
func stopConn(conn *sharedConn) {
	if conn == nil {
		return
	}
	conn.cancel()
	<-conn.done
}

// dispatch 把事件分派给当前全部处理函数。
func (sw *sharedWatcher) dispatch(ev model.Event) {
	sw.mu.Lock()
	handlers := make([]func(model.Event), 0, len(sw.handlers))
	for _, handler := range sw.handlers {
		if handler.onEvent != nil {
			handlers = append(handlers, handler.onEvent)
		}
	}
	sw.mu.Unlock()
	for _, onEvent := range handlers {
		onEvent(ev)
	}
}

// reportError 把不可恢复的订阅错误上报给全部处理函数。
func (sw *sharedWatcher) reportError(err error) {
	sw.mu.Lock()
	handlers := make([]func(error), 0, len(sw.handlers))
	for _, handler := range sw.handlers {
		if handler.onError != nil {
			handlers = append(handlers, handler.onError)
		}
	}
	sw.mu.Unlock()
	for _, onError := range handlers {
		onError(err)
	}
}

// unsubscribeShared 注销处理函数；同组最后一个注销时停止连接并从注册表移除。
func unsubscribeShared(c *transport.Client, key string, sw *sharedWatcher, id uint64) {
	sharedMu.Lock()
	sw.mu.Lock()
	delete(sw.handlers, id)
	var old *sharedConn
	if len(sw.handlers) > 0 {
		old = sw.rebuildLocked()
	} else {
		old = sw.conn
		sw.conn = nil
		sw.active = nil
		if byGroup := sharedWatchers[c]; byGroup != nil {
			if byGroup[key] == sw {
				delete(byGroup, key)
			}
			if len(byGroup) == 0 {
				delete(sharedWatchers, c)
			}
		}
	}
	sw.mu.Unlock()
	sharedMu.Unlock()

	stopConn(old)
}

// sharedKey 共享订阅的注册键。
func sharedKey(namespace, group string) string {
	return namespace + "\x00" + group
}

// equalStrings 判断两个字符串切片是否元素完全一致且顺序相同。
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
