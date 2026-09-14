package quickstart

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wueasy/waymark-sdk-golang/internal/cache"
	"github.com/wueasy/waymark-sdk-golang/internal/model"
	"github.com/wueasy/waymark-sdk-golang/internal/registry"
	"github.com/wueasy/waymark-sdk-golang/internal/transport"
)

// 服务发现默认参数。
const (
	// DefaultHeartbeatInterval 自身实例默认心跳间隔，需小于服务端心跳超时时间。
	DefaultHeartbeatInterval = 5 * time.Second
	// DefaultRefreshInterval 内存实例缓存默认全量刷新间隔。
	DefaultRefreshInterval = 30 * time.Second
)

// RegistryResolverOptions 服务发现解析器初始化参数。
type RegistryResolverOptions struct {
	// Namespace 命名空间，为空时使用默认命名空间。
	Namespace string
	// GroupName 分组，为空时使用默认分组。
	GroupName string
	// Self 自身实例信息；非空时自动注册并保持心跳，Close 时自动注销。
	Self *model.InstanceRequest
	// HeartbeatInterval 自身实例心跳间隔，默认 5s。
	HeartbeatInterval time.Duration
	// RefreshInterval 内存实例缓存的全量刷新间隔，默认 30s。
	RefreshInterval time.Duration
	// OnError 错误回调，可选。
	OnError func(error)
	// ReconnectDelay 订阅断线重连间隔，默认 3s。
	ReconnectDelay time.Duration
}

// SelectedInstance 负载均衡选中的实例。
type SelectedInstance struct {
	// Instance 实例信息。
	Instance model.Instance
	// URL 实例访问链接，形如 http://127.0.0.1:8080。
	URL string
	// CurrentWeight 命中实例的生效权重，即调度实际使用的权重（权重非正时按 1 处理）。
	CurrentWeight float64
}

// RegistryResolver 服务发现解析器：将在线实例缓存到内存，提供按服务名的
// 平滑加权轮询（SWRR）选择，并可选自动注册、保持自身实例在线。使用 Close 释放。
type RegistryResolver struct {
	client    *transport.Client
	namespace string
	group     string
	self      *model.InstanceRequest
	heartbeat time.Duration
	refresh   time.Duration
	onError   func(error)

	mu         sync.Mutex
	services   map[string][]*instanceState
	registered bool

	cancel      context.CancelFunc
	unsubscribe func()
	closeOnce   sync.Once
	wg          sync.WaitGroup
}

// instanceState 缓存中的实例及其平滑加权轮询状态。
type instanceState struct {
	instance        model.Instance
	effectiveWeight float64
	currentWeight   float64
}

// NewRegistryResolver 创建服务发现解析器：拉取一次在线实例并缓存到内存，
// 随后订阅实例变更并定时全量刷新；Self 非空时自动注册并保持心跳。
func NewRegistryResolver(c *transport.Client, ctx context.Context, opts RegistryResolverOptions) (*RegistryResolver, error) {
	heartbeat := opts.HeartbeatInterval
	if heartbeat <= 0 {
		heartbeat = DefaultHeartbeatInterval
	}
	refresh := opts.RefreshInterval
	if refresh <= 0 {
		refresh = DefaultRefreshInterval
	}

	r := &RegistryResolver{
		client:    c,
		namespace: cache.NormalizeKey(opts.Namespace, model.DefaultNamespace),
		group:     cache.NormalizeKey(opts.GroupName, model.DefaultGroup),
		heartbeat: heartbeat,
		refresh:   refresh,
		onError:   opts.OnError,
		services:  make(map[string][]*instanceState),
	}
	if opts.Self != nil {
		self, err := r.normalizeSelf(*opts.Self)
		if err != nil {
			return nil, err
		}
		r.self = self
	}

	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	if r.self != nil {
		c.Logger().Infof("waymark: 启动服务发现解析器 namespace=%s group=%s self=%s:%d", r.namespace, r.group, r.self.Ip, r.self.Port)
	} else {
		c.Logger().Infof("waymark: 启动服务发现解析器 namespace=%s group=%s", r.namespace, r.group)
	}

	// 初始化：拉取一次在线实例；失败不阻断启动，交由后台刷新兜底。
	if err := r.refreshAll(runCtx); err != nil {
		c.Logger().Warnf("waymark: 加载在线实例失败: %v", err)
		reportError(r.onError, fmt.Errorf("waymark: 加载在线实例失败: %w", err))
	}

	// 订阅该分组下全部服务的实例变更；不订阅任何配置，与同分组的配置监听复用一条 SSE 连接。
	r.unsubscribe = subscribeShared(c, r.namespace, r.group, opts.ReconnectDelay, nil,
		func(ev model.Event) { r.handleEvent(runCtx, ev) },
		func(err error) {
			c.Logger().Errorf("waymark: 实例订阅异常退出: %v", err)
			reportError(r.onError, err)
		})

	// 定时全量刷新，兜底补偿丢失的变更事件。
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.refreshLoop(runCtx)
	}()

	// 自身实例注册与心跳。
	if r.self != nil {
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.keepAliveLoop(runCtx)
		}()
	}

	return r, nil
}

// Pick 按服务名返回一个在线实例（平滑加权轮询），无可用实例时返回错误。
func (r *RegistryResolver) Pick(service string) (*SelectedInstance, error) {
	service = strings.TrimSpace(service)
	if service == "" {
		return nil, fmt.Errorf("waymark: 服务名不能为空")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	states := r.services[service]
	if len(states) == 0 {
		return nil, fmt.Errorf("waymark: 服务 %s 暂无在线实例", service)
	}

	// 平滑加权轮询：每个实例先累加自身权重，取当前权重最大者，选中后再减去总权重。
	var totalWeight float64
	var best *instanceState
	for _, state := range states {
		state.currentWeight += state.effectiveWeight
		totalWeight += state.effectiveWeight
		if best == nil || state.currentWeight > best.currentWeight {
			best = state
		}
	}
	best.currentWeight -= totalWeight

	picked := &SelectedInstance{
		Instance:      best.instance,
		URL:           instanceURL(best.instance),
		CurrentWeight: best.effectiveWeight,
	}
	r.client.Logger().Debugf("waymark: 选中实例 service=%s url=%s weight=%.2f", service, picked.URL, picked.CurrentWeight)
	return picked, nil
}

// PickURL 按服务名返回一个在线实例的访问链接，形如 http://127.0.0.1:8080。
func (r *RegistryResolver) PickURL(service string) (string, error) {
	picked, err := r.Pick(service)
	if err != nil {
		return "", err
	}
	return picked.URL, nil
}

// Instances 返回指定服务的在线实例快照，服务不存在时返回空列表。
func (r *RegistryResolver) Instances(service string) []model.Instance {
	service = strings.TrimSpace(service)
	r.mu.Lock()
	defer r.mu.Unlock()
	states := r.services[service]
	out := make([]model.Instance, 0, len(states))
	for _, state := range states {
		out = append(out, state.instance)
	}
	return out
}

// Close 停止后台订阅与刷新；若注册过自身实例，会先注销。
func (r *RegistryResolver) Close() error {
	r.closeOnce.Do(func() {
		r.client.Logger().Infof("waymark: 关闭服务发现解析器 namespace=%s group=%s", r.namespace, r.group)
		r.cancel()
		r.unsubscribe()
		r.wg.Wait()
	})
	if r.self == nil || !r.isRegistered() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), transport.DefaultCallTimeout)
	defer cancel()
	if err := registry.DeregisterInstance(r.client, ctx, r.self.Namespace, r.self.GroupName, r.self.ServiceName, r.self.Ip, r.self.Port); err != nil {
		return fmt.Errorf("waymark: 注销自身实例失败: %w", err)
	}
	r.setRegistered(false)
	return nil
}

// normalizeSelf 补全自身实例的默认字段并校验必填项。
func (r *RegistryResolver) normalizeSelf(self model.InstanceRequest) (*model.InstanceRequest, error) {
	if strings.TrimSpace(self.ServiceName) == "" {
		return nil, fmt.Errorf("waymark: 自身实例的 serviceName 不能为空")
	}
	if strings.TrimSpace(self.Ip) == "" {
		return nil, fmt.Errorf("waymark: 自身实例的 ip 不能为空")
	}
	if self.Port <= 0 {
		return nil, fmt.Errorf("waymark: 自身实例的 port 必须大于 0")
	}
	if strings.TrimSpace(self.Namespace) == "" {
		self.Namespace = r.namespace
	}
	if strings.TrimSpace(self.GroupName) == "" {
		self.GroupName = r.group
	}
	if strings.TrimSpace(self.ClusterName) == "" {
		self.ClusterName = model.DefaultCluster
	}
	return &self, nil
}

// handleEvent 处理实例变更事件，刷新受影响服务的在线实例缓存。
func (r *RegistryResolver) handleEvent(ctx context.Context, ev model.Event) {
	if ev.EventType != model.EventTypeInstance {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, transport.DefaultCallTimeout)
	defer cancel()

	service := strings.TrimSpace(ev.WatchKey)
	var err error
	if service == "" {
		err = r.refreshAll(callCtx)
	} else {
		err = r.refreshService(callCtx, service)
	}
	if err != nil && ctx.Err() == nil {
		r.client.Logger().Errorf("waymark: 刷新实例缓存失败: %v", err)
		reportError(r.onError, fmt.Errorf("waymark: 刷新实例缓存失败: %w", err))
	}
}

// refreshAll 拉取该命名空间与分组下的全部实例，重建内存缓存。
func (r *RegistryResolver) refreshAll(ctx context.Context) error {
	instances, err := registry.ListInstances(r.client, ctx, r.namespace, r.group, "")
	if err != nil {
		return err
	}
	grouped := make(map[string][]model.Instance)
	for _, inst := range instances {
		if inst.Healthy != 1 {
			continue
		}
		grouped[inst.ServiceName] = append(grouped[inst.ServiceName], inst)
	}
	r.replace(grouped)
	r.client.Logger().Debugf("waymark: 全量刷新实例缓存完成，在线服务数=%d", len(grouped))
	return nil
}

// refreshService 拉取指定服务的在线实例，更新对应缓存。
func (r *RegistryResolver) refreshService(ctx context.Context, service string) error {
	instances, err := registry.ListInstances(r.client, ctx, r.namespace, r.group, service)
	if err != nil {
		return err
	}
	online := make([]model.Instance, 0, len(instances))
	for _, inst := range instances {
		if inst.Healthy == 1 {
			online = append(online, inst)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(online) == 0 {
		delete(r.services, service)
		r.client.Logger().Debugf("waymark: 服务 %s 已无在线实例", service)
		return nil
	}
	r.services[service] = buildStates(r.services[service], online)
	r.client.Logger().Debugf("waymark: 刷新服务 %s 实例完成，在线实例数=%d", service, len(online))
	return nil
}

// replace 用最新的全量实例重建缓存，并保留仍在线实例的调度权重。
func (r *RegistryResolver) replace(grouped map[string][]model.Instance) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for service, instances := range grouped {
		r.services[service] = buildStates(r.services[service], instances)
	}
	// 已无在线实例的服务从缓存中移除。
	for service := range r.services {
		if _, ok := grouped[service]; !ok {
			delete(r.services, service)
		}
	}
}

// buildStates 构建实例调度状态；同实例沿用原有当前权重，避免刷新后调度突变。
func buildStates(prev []*instanceState, instances []model.Instance) []*instanceState {
	next := make([]*instanceState, 0, len(instances))
	for _, inst := range instances {
		weight := inst.Weight
		if weight <= 0 {
			weight = 1
		}
		state := &instanceState{instance: inst, effectiveWeight: weight}
		for _, old := range prev {
			if old.instance.Ip == inst.Ip && old.instance.Port == inst.Port {
				state.currentWeight = old.currentWeight
				break
			}
		}
		next = append(next, state)
	}
	return next
}

// refreshLoop 按固定间隔全量刷新实例缓存，直到 ctx 结束。
func (r *RegistryResolver) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(r.refresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			callCtx, cancel := context.WithTimeout(ctx, transport.DefaultCallTimeout)
			err := r.refreshAll(callCtx)
			cancel()
			if err != nil && ctx.Err() == nil {
				r.client.Logger().Warnf("waymark: 定时刷新实例缓存失败: %v", err)
				reportError(r.onError, fmt.Errorf("waymark: 刷新实例缓存失败: %w", err))
			}
		}
	}
}

// keepAliveLoop 注册自身实例并按固定间隔发送心跳，直到 ctx 结束。
func (r *RegistryResolver) keepAliveLoop(ctx context.Context) {
	r.registerSelf(ctx)

	ticker := time.NewTicker(r.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.beatSelf(ctx)
		}
	}
}

// registerSelf 注册自身实例，成功后立即刷新该服务，保证自身实例尽快进入内存缓存。
func (r *RegistryResolver) registerSelf(ctx context.Context) {
	regCtx, cancel := context.WithTimeout(ctx, transport.DefaultCallTimeout)
	_, err := registry.RegisterInstance(r.client, regCtx, *r.self)
	cancel()
	if err != nil {
		r.setRegistered(false)
		r.client.Logger().Errorf("waymark: 注册自身实例失败 service=%s address=%s:%d: %v", r.self.ServiceName, r.self.Ip, r.self.Port, err)
		reportError(r.onError, fmt.Errorf("waymark: 注册自身实例失败: %w", err))
		return
	}
	r.setRegistered(true)
	r.client.Logger().Infof("waymark: 自身实例注册成功 service=%s address=%s:%d", r.self.ServiceName, r.self.Ip, r.self.Port)

	refreshCtx, cancelRefresh := context.WithTimeout(ctx, transport.DefaultCallTimeout)
	err = r.refreshService(refreshCtx, r.self.ServiceName)
	cancelRefresh()
	if err != nil && ctx.Err() == nil {
		r.client.Logger().Errorf("waymark: 刷新服务 %s 实例失败: %v", r.self.ServiceName, err)
		reportError(r.onError, fmt.Errorf("waymark: 刷新服务 %s 实例失败: %w", r.self.ServiceName, err))
	}
}

// beatSelf 发送自身实例心跳；失败通常意味着实例已被剔除，此时重新注册。
func (r *RegistryResolver) beatSelf(ctx context.Context) {
	beatCtx, cancel := context.WithTimeout(ctx, transport.DefaultCallTimeout)
	err := registry.Beat(r.client, beatCtx, r.self.Namespace, r.self.GroupName, r.self.ServiceName, r.self.Ip, r.self.Port)
	cancel()
	if err == nil || ctx.Err() != nil {
		return
	}
	r.client.Logger().Warnf("waymark: 实例心跳失败，尝试重新注册: %v", err)
	reportError(r.onError, fmt.Errorf("waymark: 实例心跳失败: %w", err))
	r.registerSelf(ctx)
}

// isRegistered 返回自身实例当前是否处于已注册状态。
func (r *RegistryResolver) isRegistered() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registered
}

// setRegistered 更新自身实例的注册状态。
func (r *RegistryResolver) setRegistered(registered bool) {
	r.mu.Lock()
	r.registered = registered
	r.mu.Unlock()
}

// instanceURL 拼接实例访问链接。
func instanceURL(inst model.Instance) string {
	return "http://" + net.JoinHostPort(inst.Ip, strconv.Itoa(inst.Port))
}
