// Package subscribe 实现配置与实例变更的 SSE 订阅。
package subscribe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wueasy/waymark-sdk-golang/internal/apierr"
	"github.com/wueasy/waymark-sdk-golang/internal/model"
	"github.com/wueasy/waymark-sdk-golang/internal/transport"
)

// DefaultReconnectDelay 订阅重连默认间隔。
const DefaultReconnectDelay = 3 * time.Second

// Watch 建立一条订阅连接，同时订阅配置与实例变更，阻塞直到 ctx 结束；连接断开后会自动重连。
// DataIds 为要订阅的 dataId 列表：传 "*" 表示订阅该分组下全部配置变更，为空表示不订阅配置；
// ServiceName 为空表示订阅该分组下全部服务变更。
func Watch(c *transport.Client, ctx context.Context, opts model.SubscribeOptions) error {
	q := url.Values{}
	transport.SetIfNotEmpty(q, "namespace", opts.Namespace)
	transport.SetIfNotEmpty(q, "groupName", opts.GroupName)
	for _, dataId := range opts.DataIds {
		if v := strings.TrimSpace(dataId); v != "" {
			q.Add("dataId", v)
		}
	}
	transport.SetIfNotEmpty(q, "serviceName", opts.ServiceName)
	return watch(c, ctx, "/api/subscribe", q, opts)
}

// watch 建立订阅并在断开后重连，直到 ctx 结束。
func watch(c *transport.Client, ctx context.Context, path string, query url.Values, opts model.SubscribeOptions) error {
	delay := opts.ReconnectDelay
	if delay <= 0 {
		delay = DefaultReconnectDelay
	}
	for {
		err := stream(c, ctx, path, query, opts.Handler)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// 认证缺失或权限不足等不可恢复错误直接返回，避免无效重连。
		if errors.Is(err, apierr.ErrNoCredentials) || apierr.IsForbidden(err) {
			c.Logger().Errorf("waymark: 订阅终止（不可恢复错误）: %v", err)
			return err
		}
		// 网络层错误时切换到下一个服务端地址（配置了多个地址才生效）。
		if transport.IsTransportFailure(err) {
			c.Failover()
			c.Logger().Warnf("waymark: 订阅连接失败，切换到地址 %s", c.Endpoint())
		}
		c.Logger().Warnf("waymark: 订阅断开，将在 %s 后重连: %v", delay, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// stream 建立一次 SSE 连接并持续解析事件，直到连接结束或 ctx 结束。
func stream(c *transport.Client, ctx context.Context, path string, query url.Values, handler func(model.Event)) error {
	if err := c.EnsureToken(ctx); err != nil {
		return err
	}

	target := c.Endpoint() + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("waymark: 构建订阅请求失败: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if token := c.Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.StreamClient().Do(req)
	if err != nil {
		return &transport.TransportError{Err: fmt.Errorf("waymark: 订阅失败: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		var rv transport.ResultVo
		if json.Unmarshal(data, &rv) == nil {
			if apiErr := rv.ToError(resp.StatusCode); apiErr != nil {
				// 令牌失效时清除，重连时会重新登录。
				if apierr.IsAuthError(apiErr) && c.CanRelogin() {
					c.SetToken("")
				}
				return apiErr
			}
		}
		return fmt.Errorf("waymark: 订阅失败(status=%d)", resp.StatusCode)
	}
	c.Logger().Infof("waymark: 订阅已建立 %s", target)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	eventName := ""
	var dataBuf strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if dataBuf.Len() > 0 {
				c.Logger().Debugf("waymark: 收到订阅事件 event=%s", eventName)
				dispatchEvent(eventName, dataBuf.String(), handler)
			}
			eventName = ""
			dataBuf.Reset()
		case strings.HasPrefix(line, ":"):
			// 注释行（心跳），忽略。
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return &transport.TransportError{Err: fmt.Errorf("waymark: 订阅连接中断: %w", err)}
	}
	return nil
}

// dispatchEvent 解析并派发单个 SSE 事件。
func dispatchEvent(eventName, payload string, handler func(model.Event)) {
	if handler == nil {
		return
	}
	// 仅处理变更事件，忽略其他自定义事件。
	if eventName != "" && eventName != "change" {
		return
	}
	var ev model.Event
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return
	}
	handler(ev)
}
